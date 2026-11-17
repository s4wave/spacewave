package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/sirupsen/logrus"
)

// TestCommandVerify checks the verify against a bucket alone: a clean bucket
// passes with every block counted for the root that reached it first, and a
// bucket with one block's bytes corrupted and one pack moved to the trash
// fails, naming those two blocks, the second as trashed, and counting the rest.
func TestCommandVerify(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Write a tree in two packs: a leaf alone in the older, and the root, a
	// corruptible leaf, and an inner block with its own child in the newer.
	// A lookup passes over a pack it cannot read, in whatever order it tries
	// them, so the older pack's loss stays the size of its one block.
	bucket, client := s3test.New(t)
	seed := block_store_s3.NewPackStore(le, client, s3test.BucketName, "world/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	ref := func(data string) *block.BlockRef {
		r, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	const corruptData = "corruptible leaf with bytes long enough to find in its pack"
	corruptRef, innerRef, deepRef, lostRef := ref(corruptData), ref("inner"), ref("deep"), ref("lost")
	rootRef := ref("root")
	for _, batch := range [][]*block.PutBatchEntry{
		{{Ref: lostRef, Data: []byte("lost"), RefsKnown: true}},
		{
			{Ref: rootRef, Data: []byte("root"), Refs: []*block.BlockRef{corruptRef, innerRef, lostRef}, RefsKnown: true},
			{Ref: corruptRef, Data: []byte(corruptData), RefsKnown: true},
			{Ref: innerRef, Data: []byte("inner"), Refs: []*block.BlockRef{deepRef}, RefsKnown: true},
			{Ref: deepRef, Data: []byte("deep"), RefsKnown: true},
		},
	} {
		if _, err := seed.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	seed.Close()

	// Verify the clean bucket from the root and from a second root inside
	// the first's tree, which adds nothing.
	verify := func() (*fastjson.Value, error) {
		var out bytes.Buffer
		args := &Args{
			Prefix: "world/",
			Roots:  *cli.NewStringSlice(rootRef.MarshalString(), innerRef.MarshalString()),
			Batch:  2,
			Reads:  2,
			MaxBad: 5,
			dial: func() (block_store_s3.ObjectStore, string, func(), error) {
				return client, s3test.BucketName, func() {}, nil
			},
			out: &out,
		}
		runErr := args.runVerify(nil)
		res, err := fastjson.ParseBytes(out.Bytes())
		if err != nil {
			t.Fatalf("verify printed %q: %v", out.String(), err)
		}
		return res, runErr
	}
	res, err := verify()
	if err != nil {
		t.Fatalf("clean bucket failed: %v (%s)", err, res)
	}
	if got := res.GetUint64("blocks"); got != 5 || res.GetUint64("bad") != 0 {
		t.Fatalf("clean bucket = %s, want 5 blocks and none bad", res)
	}
	perRoot := func(res *fastjson.Value) []uint64 {
		var out []uint64
		for _, rr := range res.GetArray("roots") {
			out = append(out, rr.GetUint64("blocks"))
		}
		return out
	}
	if got := perRoot(res); len(got) != 2 || got[0] != 5 || got[1] != 0 {
		t.Fatalf("clean per root = %v, want [5 0]", got)
	}

	// Corrupt the leaf's bytes in the first pack, and move the second pack
	// and its entry to the trash, as a sweep does.
	var packs []string
	if err := client.ListObjects(ctx, s3test.BucketName, "world/packs/", func(key string, _ int64) error {
		packs = append(packs, key)
		return nil
	}); err != nil || len(packs) != 2 {
		t.Fatalf("packs = %v, %v", packs, err)
	}
	corrupted := false
	for _, key := range packs {
		data := readObject(ctx, t, client, key)
		if i := bytes.Index(data, []byte(corruptData)); i >= 0 {
			data[i] ^= 0xff
			if err := client.PutObject(ctx, s3test.BucketName, key, data, ""); err != nil {
				t.Fatal(err)
			}
			corrupted = true
			continue
		}
		id := strings.TrimPrefix(key, "world/packs/")
		moves := [][2]string{
			{key, block_store_s3.TrashPackKey("world/", id)},
			{"world/entries/" + id, block_store_s3.TrashEntryKey("world/", id)},
		}
		for _, mv := range moves {
			if err := client.PutObject(ctx, s3test.BucketName, mv[1], readObject(ctx, t, client, mv[0]), ""); err != nil {
				t.Fatal(err)
			}
			if err := client.DeleteObject(ctx, s3test.BucketName, mv[0]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !corrupted {
		t.Fatal("no pack holds the leaf's bytes as written")
	}
	if keys := bucket.Keys("world/packs/"); len(keys) != 1 {
		t.Fatalf("packs left = %v, want one", keys)
	}

	// The verify fails, naming the two blocks and counting the other three.
	res, err = verify()
	if err == nil {
		t.Fatalf("damaged bucket passed: %s", res)
	}
	if res.GetUint64("blocks") != 3 || res.GetUint64("bad") != 2 {
		t.Fatalf("damaged bucket = %s, want 3 blocks and 2 bad", res)
	}
	bad := make(map[string]string)
	for _, b := range res.GetArray("bad_blocks") {
		bad[string(b.GetStringBytes("ref"))] = string(b.GetStringBytes("reason"))
	}
	if len(bad) != 2 || bad[corruptRef.MarshalString()] == "" || bad[lostRef.MarshalString()] != packsweep.ReasonTrashed {
		t.Fatalf("bad blocks = %v, want the corrupted leaf and the trashed one", bad)
	}
	if got := perRoot(res); len(got) != 2 || got[0] != 3 || got[1] != 0 {
		t.Fatalf("damaged per root = %v, want [3 0]", got)
	}
	t.Logf("bad reasons: %v", bad)
}

// readObject reads a whole object.
func readObject(ctx context.Context, t *testing.T, client block_store_s3.ObjectStore, key string) []byte {
	rc, err := client.GetObject(ctx, s3test.BucketName, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
