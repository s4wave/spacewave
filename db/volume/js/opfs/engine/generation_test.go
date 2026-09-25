//go:build !js

package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// observedRootBackend reports the end of each two-slot root read.
type observedRootBackend struct {
	// Backend provides durable files and deliberately drops notification hints.
	Backend
	// read wakes the test after the second root slot has been copied.
	read chan struct{}
}

// Read exposes the root-read boundary without changing the copied file bytes.
func (b *observedRootBackend) Read(ctx context.Context, name string, offset int64, length int) ([]byte, error) {
	data, err := b.Backend.Read(ctx, name, offset, length)
	if name == "root-1" {
		select {
		case b.read <- struct{}{}:
		default:
		}
	}
	return data, err
}

// TestGenerationWaitRecoversLostHintsAndCloses checks durable polling and shutdown.
func TestGenerationWaitRecoversLostHintsAndCloses(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	disk := newDiskBackend(t)
	writer, err := Open(ctx, disk)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	observed := &observedRootBackend{Backend: disk, read: make(chan struct{}, 1)}
	reader, err := Open(ctx, observed)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	<-observed.read

	// Start waiting before publishing; the fixture never delivers hints.
	result := make(chan error, 1)
	go func() {
		generation, err := reader.WaitGeneration(ctx, 0)
		if err == nil && generation != 1 {
			err = errors.New("wait returned the wrong durable revision")
		}
		result <- err
	}()
	select {
	case <-observed.read:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := writer.Apply(ctx, []*Record{{Key: []byte("changed"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// A waiter for another revision must stop when its owning engine closes.
	go func() {
		_, err := reader.WaitGeneration(ctx, 1)
		result <- err
	}()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("closed generation wait: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
