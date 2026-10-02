package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"sort"
)

// fileJSON is either embedded as JSON or escaped as a JSON string. This lets
// error queues retain the legacy form's jsonData without loading it into RAM.
type fileJSON struct {
	file   *mediaFile
	quoted bool
}

func writeMediaJSON(w io.Writer, value interface{}) error {
	switch v := value.(type) {
	case *mediaFile:
		r, err := v.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		if _, err = io.WriteString(w, `"`); err != nil {
			return err
		}
		enc := base64.NewEncoder(base64.StdEncoding, w)
		_, err = io.Copy(enc, r)
		endErr := enc.Close()
		if err != nil {
			return err
		}
		if endErr != nil {
			return endErr
		}
		_, err = io.WriteString(w, `"`)
		return err
	case fileJSON:
		r, err := v.file.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		if !v.quoted {
			_, err = io.Copy(w, r)
			return err
		}
		if _, err = io.WriteString(w, `"`); err != nil {
			return err
		}
		_, err = io.Copy(jsonStringWriter{w}, r)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, `"`)
		return err
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := io.WriteString(w, "{"); err != nil {
			return err
		}
		for i, k := range keys {
			if i > 0 {
				if _, err := io.WriteString(w, ","); err != nil {
					return err
				}
			}
			key, _ := json.Marshal(k)
			if _, err := w.Write(key); err != nil {
				return err
			}
			if _, err := io.WriteString(w, ":"); err != nil {
				return err
			}
			if err := writeMediaJSON(w, v[k]); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "}")
		return err
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
}

// Input is already valid, compact JSON. Its control characters are escaped, so
// only quotes and backslashes need an additional escape when nesting it.
type jsonStringWriter struct{ io.Writer }

func (w jsonStringWriter) Write(p []byte) (int, error) {
	start := 0
	for i, b := range p {
		if b == '"' || b == '\\' {
			if _, err := w.Writer.Write(p[start:i]); err != nil {
				return 0, err
			}
			if _, err := w.Writer.Write([]byte{'\\', b}); err != nil {
				return 0, err
			}
			start = i + 1
		}
	}
	if _, err := w.Writer.Write(p[start:]); err != nil {
		return 0, err
	}
	return len(p), nil
}

type formEscapeWriter struct {
	io.Writer
	buffer [12 * 1024]byte
}

func (w *formEscapeWriter) Write(p []byte) (int, error) {
	const hex = "0123456789ABCDEF"
	written := 0
	for len(p) > 0 {
		count := len(p)
		if count > 4096 {
			count = 4096
		}
		n := 0
		for _, c := range p[:count] {
			switch {
			case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~':
				w.buffer[n] = c
				n++
			case c == ' ':
				w.buffer[n] = '+'
				n++
			default:
				w.buffer[n] = '%'
				w.buffer[n+1] = hex[c>>4]
				w.buffer[n+2] = hex[c&15]
				n += 3
			}
		}
		if _, err := w.Writer.Write(w.buffer[:n]); err != nil {
			return written, err
		}
		written += count
		p = p[count:]
	}
	return written, nil
}
func prepareMediaBody(write func(io.Writer) error) (*mediaFile, error) {
	s, err := getMediaStore()
	if err != nil {
		return nil, err
	}
	release, err := s.acquire(s.ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	body, out, err := s.create("", "application/json")
	if err != nil {
		return nil, err
	}
	buffer := bufio.NewWriterSize(out, 32*1024)
	err = write(contextWriter{ctx: s.ctx, w: buffer})
	if err == nil {
		err = buffer.Flush()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = body.refresh()
	}
	if err != nil {
		body.Close()
		return nil, err
	}
	return body, nil
}
func prepareMediaJSON(value interface{}) (*mediaFile, error) {
	return prepareMediaBody(func(w io.Writer) error { return writeMediaJSON(w, value) })
}
func prepareMediaForm(event *mediaFile, instance, userID string) (*mediaFile, error) {
	return prepareMediaBody(func(w io.Writer) error {
		if _, err := io.WriteString(w, "instanceName="+url.QueryEscape(instance)+"&jsonData="); err != nil {
			return err
		}
		r, err := event.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		if _, err = io.Copy(&formEscapeWriter{Writer: w}, r); err != nil {
			return err
		}
		_, err = io.WriteString(w, "&userID="+url.QueryEscape(userID))
		return err
	})
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}
