//go:build !tinygo

package block_store_s3_overlay

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
)

// TestOverlayMover checks the move contract over an empty base, and that the
// base receives nothing.
func TestOverlayMover(t *testing.T) {
	bucket, client := s3test.New(t)
	s, err := New(client, s3test.BucketName, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s3test.CheckMover(t, s, s3test.BucketName)
	if keys := bucket.Keys(""); len(keys) != 0 || bucket.Puts() != 0 || bucket.Copies() != 0 {
		t.Fatalf("base holds %v after %d puts and %d copies", keys, bucket.Puts(), bucket.Copies())
	}
}

// TestOverlayBase moves a base pack to the trash and back, deletes and puts
// over base keys, resumes killed moves, and reopens the directory, reading each
// key, in whole, in a range and in the listings, after each step, while the
// base stays as it was.
func TestOverlayBase(t *testing.T) {
	// Seed the base with a pack and its entry.
	ctx := context.Background()
	bucket, client := s3test.New(t)
	const pack, entry, trash = "world/packs/p", "world/entries/p", "world/trash/packs/p"
	data := bytes.Repeat([]byte("0123456789"), 100)
	for key, value := range map[string][]byte{pack: data, entry: []byte("entry")} {
		if err := client.PutObject(ctx, s3test.BucketName, key, value, ""); err != nil {
			t.Fatal(err)
		}
	}
	before, puts := readAll(t, client), bucket.Puts()
	dir := t.TempDir()
	s, err := New(client, s3test.BucketName, dir)
	if err != nil {
		t.Fatal(err)
	}

	// The overlay reads the base, and refuses another bucket and unclean keys.
	check(t, s, map[string][]byte{pack: data, entry: []byte("entry")})
	if _, err := s.GetObjectRange(ctx, "other", pack, 0, 1); err == nil {
		t.Fatal("read another bucket")
	}
	for _, key := range []string{"../x", "/x", "world//x", "world/" + tempPrefix + "x"} {
		if err := s.PutObject(ctx, s3test.BucketName, key, nil, ""); err == nil {
			t.Fatalf("put key %q", key)
		}
	}

	// A move to the trash reads at the trash key, at the base's size, and
	// again after a reopen.
	if err := s.MoveObject(ctx, s3test.BucketName, pack, trash); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{trash: data, entry: []byte("entry")})
	if s, err = New(client, s3test.BucketName, dir); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{trash: data, entry: []byte("entry")})

	// A move back drops the trash key's alias, and the base reads again.
	if err := s.MoveObject(ctx, s3test.BucketName, trash, pack); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{pack: data, entry: []byte("entry")})
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(pack))); !os.IsNotExist(err) {
		t.Fatalf("record of the pack after the move back: %v", err)
	}

	// A move killed after its alias leaves the object at both keys, and a
	// repeat finishes it.
	if err := writeRecord(filepath.Join(dir, filepath.FromSlash(trash)), kindAlias, []byte(pack)); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{pack: data, trash: data, entry: []byte("entry")})
	if err := s.MoveObject(ctx, s3test.BucketName, pack, trash); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{trash: data, entry: []byte("entry")})

	// A move back killed after it unhid the base key leaves the object at
	// both keys, and a repeat finishes it.
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(pack))); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{pack: data, trash: data, entry: []byte("entry")})
	if err := s.MoveObject(ctx, s3test.BucketName, trash, pack); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{pack: data, entry: []byte("entry")})
	if err := s.MoveObject(ctx, s3test.BucketName, pack, trash); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{trash: data, entry: []byte("entry")})

	// A delete hides the base's entry, a put over it reads the local bytes,
	// and a delete of the trash hides it.
	if err := s.DeleteObject(ctx, s3test.BucketName, entry); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{trash: data})
	if err := s.PutObject(ctx, s3test.BucketName, entry, []byte("new entry"), ""); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{trash: data, entry: []byte("new entry")})
	if err := s.DeleteObject(ctx, s3test.BucketName, trash); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{entry: []byte("new entry")})

	// A move of local bytes killed after its link leaves them at both keys,
	// and a repeat finishes it; a move of nothing is not found.
	const moved = "world/trash/entries/p"
	if err := linkRecord(filepath.Join(dir, filepath.FromSlash(entry)), filepath.Join(dir, filepath.FromSlash(moved))); err != nil {
		t.Fatal(err)
	}
	check(t, s, map[string][]byte{entry: []byte("new entry"), moved: []byte("new entry")})
	for range 2 {
		if err := s.MoveObject(ctx, s3test.BucketName, entry, moved); err != nil {
			t.Fatal(err)
		}
		check(t, s, map[string][]byte{moved: []byte("new entry")})
	}
	if err := s.MoveObject(ctx, s3test.BucketName, pack, "world/x"); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("move of a deleted key = %v, want not found", err)
	}

	// The base is as it was.
	if after := readAll(t, client); !maps.EqualFunc(before, after, bytes.Equal) || bucket.Puts() != puts || bucket.Copies() != 0 {
		t.Fatalf("base changed: %d keys, %d puts, %d copies", len(after), bucket.Puts()-puts, bucket.Copies())
	}
}

// check checks that s holds exactly want under "world/": each key in whole,
// in a range, and in the listings of "world/" and of its directory.
func check(t *testing.T, s *Store, want map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	for key, data := range want {
		// Read the object whole and in a range.
		rc, err := s.GetObject(ctx, s3test.BucketName, key)
		if err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		got, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("read %s = %q, %v; want %q", key, got, err, data)
		}
		part, err := s.GetObjectRange(ctx, s3test.BucketName, key, 2, 3)
		if err != nil || !bytes.Equal(part, data[2:5]) {
			t.Fatalf("range of %s = %q, %v", key, part, err)
		}

		// List its directory alone.
		listed := list(t, s, key[:bytes.LastIndexByte([]byte(key), '/')+1])
		if size, ok := listed[key]; !ok || size != int64(len(data)) {
			t.Fatalf("listing of %s's directory = %v", key, listed)
		}
	}

	// The listing of the whole prefix holds the keys at their sizes, and each
	// key it lacks is not found.
	listed := list(t, s, "world/")
	for key, size := range listed {
		if int64(len(want[key])) != size {
			t.Fatalf("listing = %v, want %d keys", listed, len(want))
		}
	}
	if len(listed) != len(want) {
		t.Fatalf("listing = %v, want %d keys", listed, len(want))
	}
	for _, key := range []string{"world/packs/p", "world/entries/p", "world/trash/packs/p", "world/trash/entries/p"} {
		if _, ok := want[key]; ok {
			continue
		}
		if _, err := s.GetObjectRange(ctx, s3test.BucketName, key, 0, 1); !errors.Is(err, block_store_s3.ErrNotFound) {
			t.Fatalf("range of %s = %v, want not found", key, err)
		}
		if _, err := s.GetObject(ctx, s3test.BucketName, key); !errors.Is(err, block_store_s3.ErrNotFound) {
			t.Fatalf("read of %s = %v, want not found", key, err)
		}
	}
}

// list lists prefix of s, failing on a key listed twice.
func list(t *testing.T, s *Store, prefix string) map[string]int64 {
	t.Helper()
	out := make(map[string]int64)
	err := s.ListObjects(context.Background(), s3test.BucketName, prefix, func(key string, size int64) error {
		if _, ok := out[key]; ok {
			return errors.Errorf("listed %s twice", key)
		}
		out[key] = size
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// readAll reads every object of the base.
func readAll(t *testing.T, client block_store_s3.ObjectStore) map[string][]byte {
	t.Helper()
	ctx := context.Background()
	out := make(map[string][]byte)
	err := client.ListObjects(ctx, s3test.BucketName, "", func(key string, _ int64) error {
		rc, err := client.GetObject(ctx, s3test.BucketName, key)
		if err != nil {
			return err
		}
		out[key], err = io.ReadAll(rc)
		_ = rc.Close()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
