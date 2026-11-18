package main

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
)

// countingStore counts a store's reads and the bytes they return.
type countingStore struct {
	block_store_s3.ObjectStore
	// gets and bytes are the reads and their bytes.
	gets, bytes atomic.Int64
}

// GetObject counts the read and its body's bytes.
func (c *countingStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	// Read the whole body to count it.
	c.gets.Add(1)
	body, err := c.ObjectStore.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	c.bytes.Add(int64(len(data)))
	return io.NopCloser(bytes.NewReader(data)), err
}

// GetObjectRange counts the read and its bytes.
func (c *countingStore) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	c.gets.Add(1)
	data, err := c.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
	c.bytes.Add(int64(len(data)))
	return data, err
}

// TestPacedStore checks the pacing of a step's reads: the rate cap holds a
// read past its burst to the rate, and a closed gate holds every read until
// it opens, asking it at most once while an admission covers the reads.
func TestPacedStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, client := s3test.New(t)
	data := make([]byte, 96<<10)
	if err := client.PutObject(ctx, s3test.BucketName, "obj", data, ""); err != nil {
		t.Fatal(err)
	}

	// At 32 KiB a second the first 32 KiB pass at once and the rest take two
	// seconds, in a whole read and in ranges alike.
	a := &Args{RateMiB: 1.0 / 32}
	store, closePace, err := a.pace(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer closePace()
	start := time.Now()
	body, err := store.GetObject(ctx, s3test.BucketName, "obj")
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, body)
	_ = body.Close()
	if took := time.Since(start); err != nil || n != int64(len(data)) || took < 1900*time.Millisecond || took > 3*time.Second {
		t.Fatalf("capped read of %d bytes took %v, %v; want about 2s", n, took, err)
	}
	start = time.Now()
	if _, err := store.GetObjectRange(ctx, s3test.BucketName, "obj", 0, 64<<10); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 1900*time.Millisecond || took > 3*time.Second {
		t.Fatalf("capped 64 KiB range took %v, want about 2s", took)
	}

	// A closed gate holds the reads; once open, one admission covers the
	// reads after it.
	open := make(chan struct{})
	var asks atomic.Int64
	a = &Args{dialGate: func(context.Context) (func(context.Context) error, func(), error) {
		return func(ctx context.Context) error {
			asks.Add(1)
			select {
			case <-open:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, func() {}, nil
	}}
	store, closeGate, err := a.pace(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer closeGate()
	done := make(chan error, 4)
	for range 4 {
		go func() {
			_, err := store.GetObjectRange(ctx, s3test.BucketName, "obj", 0, 16)
			done <- err
		}()
	}
	select {
	case err := <-done:
		t.Fatalf("a read passed a closed gate: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(open)
	for range 4 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if n := asks.Load(); n != 1 {
		t.Fatalf("gate asked %d times for reads one admission covers, want 1", n)
	}

	// A gate that fails fails the read.
	a = &Args{dialGate: func(context.Context) (func(context.Context) error, func(), error) {
		return func(context.Context) error { return context.DeadlineExceeded }, func() {}, nil
	}}
	store, closeFail, err := a.pace(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFail()
	if _, err := store.GetObjectRange(ctx, s3test.BucketName, "obj", 0, 16); err == nil {
		t.Fatal("a read passed a failed gate")
	}
}
