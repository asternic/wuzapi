package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"
)

// mediaFile owns one private file. Consumers must finish before Close; each Open
// returns an independent cursor. Delivery goroutines own their prepared files.
type mediaFile struct {
	Path, Name, MIME string
	Size             int64
	store            *mediaStore
	once             sync.Once
}

// Fail closed if a future caller bypasses the streaming serializer. Internal
// filesystem paths must never become part of a delivery payload.
func (*mediaFile) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("media files require the streaming serializer")
}

func (f *mediaFile) Open() (*os.File, error) { return os.Open(f.Path) }
func (f *mediaFile) Close() {
	if f != nil {
		f.once.Do(func() {
			if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
				log.Error().Err(err).Msg("Failed to remove temporary media file")
			}
			f.store.files.Done()
		})
	}
}
func (f *mediaFile) refresh() error {
	st, err := os.Stat(f.Path)
	if err != nil {
		return err
	}
	f.Size = st.Size()
	return nil
}
func (f *mediaFile) sniff() (string, error) {
	r, err := f.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	var prefix [512]byte
	n, err := io.ReadFull(r, prefix[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	return http.DetectContentType(prefix[:n]), nil
}

type mediaStore struct {
	mu       sync.Mutex
	dir      string
	lock     *os.File
	closed   bool
	files    sync.WaitGroup
	capacity chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// The root lock serializes directory creation/reclamation. Each live process
// keeps its own lock open, including when it currently has no media files.
func newMediaStore(base string, concurrency int) (*mediaStore, error) {
	if concurrency < 1 {
		return nil, fmt.Errorf("WUZAPI_MEDIA_CONCURRENCY must be positive")
	}
	if base == "" {
		base = os.TempDir()
	}
	root := filepath.Join(base, fmt.Sprintf("wuzapi-media-%d", os.Getuid()))
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	guard, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer guard.Close()
	if err = lockMediaFile(guard, false); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) < 8 || e.Name()[:8] != "process-" {
			continue
		}
		dir := filepath.Join(root, e.Name())
		owner, openErr := os.OpenFile(filepath.Join(dir, ".owner"), os.O_CREATE|os.O_RDWR, 0600)
		if openErr == nil {
			if lockMediaFile(owner, true) == nil {
				owner.Close() // Root lock prevents reclamation/creation races while removing.
				os.RemoveAll(dir)
			}
			owner.Close()
		}
	}
	dir, err := os.MkdirTemp(root, "process-")
	if err != nil {
		return nil, err
	}
	owner, err := os.OpenFile(filepath.Join(dir, ".owner"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if err = lockMediaFile(owner, true); err != nil {
		owner.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &mediaStore{dir: dir, lock: owner, capacity: make(chan struct{}, concurrency), ctx: ctx, cancel: cancel}, nil
}
func (s *mediaStore) create(name, mime string) (*mediaFile, *os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return nil, nil, context.Canceled
	}
	f, err := os.CreateTemp(s.dir, "media-*")
	if err != nil {
		return nil, nil, err
	}
	s.files.Add(1)
	return &mediaFile{Path: f.Name(), Name: name, MIME: mime, store: s}, f, nil
}
func (s *mediaStore) acquire(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case s.capacity <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		<-s.capacity
		return nil, err
	}
	if err := s.ctx.Err(); err != nil {
		<-s.capacity
		return nil, err
	}
	return func() { <-s.capacity }, nil
}
func (s *mediaStore) shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.files.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	// Serialize removal with reclamation. Closing the owner before removal is
	// also necessary on Windows, where an open lock file cannot be unlinked.
	guard, err := os.OpenFile(filepath.Join(filepath.Dir(s.dir), ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err = lockMediaFile(guard, false); err != nil {
		return err
	}
	s.lock.Close()
	return os.RemoveAll(s.dir)
}

var mediaStoreOnce sync.Once
var defaultMediaStore *mediaStore
var mediaStoreError error

func getMediaStore() (*mediaStore, error) {
	mediaStoreOnce.Do(func() {
		n := 2
		if value := os.Getenv("WUZAPI_MEDIA_CONCURRENCY"); value != "" {
			n, mediaStoreError = strconv.Atoi(value)
			if mediaStoreError != nil {
				return
			}
		}
		defaultMediaStore, mediaStoreError = newMediaStore(os.Getenv("WUZAPI_MEDIA_TMPDIR"), n)
	})
	return defaultMediaStore, mediaStoreError
}
func mediaContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	s, err := getMediaStore()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(s.ctx, cancel)
	if s.ctx.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }, nil
}
func shutdownMedia(ctx context.Context) error {
	store, err := getMediaStore()
	if err != nil {
		return err
	}
	return store.shutdown(ctx)
}
