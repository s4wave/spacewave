package s3test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// CheckMover checks the move contract of an ObjectMover over bucket: a move
// into a directory that does not exist, a read of each key after each move, a
// repeated move, a move back, and a move of nothing.
func CheckMover(t *testing.T, m block_store_s3.ObjectMover, bucket string) {
	// Write a pack and read a range, which may leave a file open.
	t.Helper()
	ctx := context.Background()
	const pack, trash = "world/packs/p.pack", "world/trash/packs/p.pack"
	data := bytes.Repeat([]byte("0123456789"), 100)
	if err := m.PutObject(ctx, bucket, pack, data, ""); err != nil {
		t.Fatal(err)
	}
	checkAt(t, m, bucket, pack, trash, data)

	// Move it into the trash, whose directory does not exist, then again.
	for range 2 {
		if err := m.MoveObject(ctx, bucket, pack, trash); err != nil {
			t.Fatal(err)
		}
		checkAt(t, m, bucket, trash, pack, data)
	}

	// Move it back to the key it left.
	if err := m.MoveObject(ctx, bucket, trash, pack); err != nil {
		t.Fatal(err)
	}
	checkAt(t, m, bucket, pack, trash, data)

	// A move with neither key there is not found.
	if err := m.MoveObject(ctx, bucket, "world/packs/none.pack", "world/trash/packs/none.pack"); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("move of nothing = %v, want not found", err)
	}
}

// checkAt checks the object reads as data at key, in whole, in a range and in
// the listing, and is not found at gone.
func checkAt(t *testing.T, m block_store_s3.ObjectMover, bucket, key, gone string, data []byte) {
	// The object reads at its key in a range.
	t.Helper()
	ctx := context.Background()
	got, err := m.GetObjectRange(ctx, bucket, key, 3, 4)
	if err != nil || string(got) != "3456" {
		t.Fatalf("range of %s = %q, %v", key, got, err)
	}

	// It reads whole.
	rc, err := m.GetObject(ctx, bucket, key)
	if err != nil {
		t.Fatal(err)
	}
	var whole bytes.Buffer
	_, err = whole.ReadFrom(rc)
	_ = rc.Close()
	if err != nil || !bytes.Equal(whole.Bytes(), data) {
		t.Fatalf("read of %s = %d bytes, %v", key, whole.Len(), err)
	}

	// It is gone from the other key, in a read and in the listing.
	if _, err := m.GetObjectRange(ctx, bucket, gone, 3, 4); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("range of %s after the move = %v, want not found", gone, err)
	}
	var keys []string
	err = m.ListObjects(ctx, bucket, "world/", func(k string, _ int64) error {
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(keys, key) || slices.Contains(keys, gone) {
		t.Fatalf("listing = %v, want %s and not %s", keys, key, gone)
	}
}
