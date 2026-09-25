//go:build !js

package engine

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// rootReadBackend pauses a retained root read at the browser byte-copy boundary.
// Disk reads alone cannot reproduce invalidation of a retained browser File.
type rootReadBackend struct {
	// Backend supplies durable files and origin-wide locks.
	Backend
	// reading records whether the retained root still needs its bytes.
	reading atomic.Bool
	// started reports that the reader has retained the root.
	started chan struct{}
	// resume permits copying the retained root bytes.
	resume chan struct{}
}

// Read holds the first root read until the test permits its byte copy.
func (b *rootReadBackend) Read(ctx context.Context, name string, offset int64, length int) ([]byte, error) {
	if name == "root-0" {
		b.reading.Store(true)
		close(b.started)
		select {
		case <-b.resume:
		case <-ctx.Done():
			b.reading.Store(false)
			return nil, ctx.Err()
		}
		defer b.reading.Store(false)
	}
	return b.Backend.Read(ctx, name, offset, length)
}

// rootWriteBackend rejects replacement while the browser still needs root bytes.
type rootWriteBackend struct {
	// Backend supplies the same durable files and locks as the reader.
	Backend
	// reader exposes the retained browser-file lifetime under test.
	reader *rootReadBackend
	// attempted reports root lock acquisition or an unprotected root write.
	attempted chan struct{}
}

// Lock reports publication reaching the root critical section.
func (b *rootWriteBackend) Lock(ctx context.Context, name string, exclusive bool) (func(), error) {
	if name == "root" && exclusive {
		b.attempted <- struct{}{}
	}
	return b.Backend.Lock(ctx, name, exclusive)
}

// Write models browser invalidation when a root is replaced during its read.
func (b *rootWriteBackend) Write(ctx context.Context, name string, data []byte) error {
	if strings.HasPrefix(name, "root-") && b.reader.reading.Load() {
		b.attempted <- struct{}{}
		return errors.New("root replacement invalidated the retained browser File")
	}
	return b.Backend.Write(ctx, name, data)
}

// TestRootReadExcludesReplacement preserves bytes without pinning later publication.
func TestRootReadExcludesReplacement(t *testing.T) {
	// Seed a durable generation before installing the browser read boundary.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	disk := newDiskBackend(t)
	writer, err := Open(ctx, disk)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})

	// Leave generation three in root-1 so the next publication replaces root-0.
	for _, value := range []string{"initial", "before"} {
		if err := writer.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte(value)}}); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := Open(ctx, disk)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	reads := &rootReadBackend{Backend: disk, started: make(chan struct{}), resume: make(chan struct{})}
	writes := &rootWriteBackend{Backend: disk, reader: reads, attempted: make(chan struct{}, 2)}
	reader.backend = reads
	writer.backend = writes

	// Pause the root byte copy and start a publication from another engine.
	readDone := make(chan *snapshot, 1)
	readErr := make(chan error, 1)
	go func() {
		s, err := reader.snapshot(ctx)
		readDone <- s
		readErr <- err
	}()
	select {
	case <-reads.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- writer.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("after")}})
	}()
	select {
	case <-writes.attempted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	close(reads.resume)

	// Publication must finish while the reader still pins its old generation.
	if err := <-readErr; err != nil {
		t.Fatal(err)
	}
	s := <-readDone
	defer s.release()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	value, found, err := s.get(ctx, []byte("key"))
	if err != nil || !found || string(value) != "before" {
		t.Fatalf("pinned generation changed: %q %t %v", value, found, err)
	}
}

// _ is a type assertion.
var (
	_ Backend = (*rootReadBackend)(nil)
	_ Backend = (*rootWriteBackend)(nil)
)
