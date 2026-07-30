package main

import (
	"sync"
	"testing"
)

// TestGetClientConcurrent runs GetClient — the locked config read now used when
// building S3 media metadata — against concurrent writers. The previous unlocked
// read of m.configs raced with these updates; under go test -race the unlocked
// version is flagged and the locked GetClient passes.
func TestGetClientConcurrent(t *testing.T) {
	m := &S3Manager{configs: map[string]*S3Config{}}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, _ = m.GetClient("u1")
		}()
		go func() {
			defer wg.Done()
			m.mu.Lock()
			m.configs["u1"] = &S3Config{Bucket: "b"}
			m.mu.Unlock()
		}()
	}
	wg.Wait()
}
