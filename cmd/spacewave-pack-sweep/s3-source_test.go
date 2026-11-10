package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// TestSnapshotOverS3 runs a mark-only cycle, new-cycle, snapshot, walk and
// judge, reading the packs from an S3 bucket by the S3 flags alone. The
// report gives each pack's live and total bytes and finds the dead pack, and
// the bucket sees no write, copy or hide.
func TestSnapshotOverS3(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()

	// Write a live pack under the root and a dead pack, through an index file.
	const storeID = "p/local/acc/blk/so"
	bucket, client := s3test.New(t)
	indexPath := filepath.Join(dir, "index.db")
	index, closeIndex, err := packcopy.OpenIndexFile(ctx, indexPath, &packcopy.IndexFileOptions{
		Kind:   "sqlite",
		Table:  defaultIndexTable,
		SQLite: store_kvtx_sqlite.Pragmas{WithoutRowID: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	seed := block_store_s3.NewPackStore(le, client, s3test.BucketName, "world/", &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     kvtx_prefixer.NewPrefixer(index, block_store_s3.PackIndexPrefix(storeID)),
	})
	ref := func(data string) *block.BlockRef {
		r, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	rootRef, leafRef, deadRef := ref("root"), ref("leaf"), ref("dead block")
	for _, batch := range [][]*block.PutBatchEntry{
		{
			{Ref: rootRef, Data: []byte("root"), Refs: []*block.BlockRef{leafRef}, RefsKnown: true},
			{Ref: leafRef, Data: []byte("leaf"), RefsKnown: true},
		},
		{{Ref: deadRef, Data: []byte("dead block"), RefsKnown: true}},
	} {
		if _, err := seed.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	seed.Close()
	if err := closeIndex(); err != nil {
		t.Fatal(err)
	}
	puts, copies := bucket.Puts(), bucket.Copies()

	// Name the bucket by the S3 flags, with the keys in the environment.
	t.Setenv(s3AccessKeyEnv, "key")
	t.Setenv(s3SecretKeyEnv, "secret")
	var out bytes.Buffer
	args := &Args{
		Scratch:    filepath.Join(dir, "scratch"),
		StoreID:    storeID,
		Index:      indexPath,
		Prefix:     "world/",
		S3Endpoint: bucket.URL(),
		S3Bucket:   s3test.BucketName,
		Reads:      2,
		Roots:      *cli.NewStringSlice(rootRef.MarshalString()),
		Batch:      1,
		MinAge:     time.Nanosecond,
		LowPercent: packsweep.DefaultLowLivePercent,
		Report:     filepath.Join(dir, "report.jsonl"),
		Holes:      filepath.Join(dir, "holes.txt"),
		out:        &out,
	}

	// Run the mark-only steps.
	for _, step := range []func(*cli.Context) error{args.runNewCycle, args.runSnapshot, args.runWalk} {
		if err := step(nil); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	if err := args.runJudge(nil); err != nil {
		t.Fatal(err)
	}
	if result := parse(t, out.Bytes()); result.GetUint("summary", "packs", "dead") != 1 || result.GetUint("summary", "packs", "kept") != 1 {
		t.Fatalf("summary = %s", result)
	}

	// The report gives each pack's live and total bytes.
	report, err := os.Open(args.Report)
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	lines := bufio.NewScanner(report)
	var judged int
	for lines.Scan() {
		j := parse(t, lines.Bytes())
		judged++
		total, live := j.GetUint("valueBytes"), j.GetUint("liveBytes")
		switch string(j.GetStringBytes("verdict")) {
		case "dead":
			if total == 0 || live != 0 {
				t.Fatalf("dead pack judgment = %s", j)
			}
		default:
			if total == 0 || live != total {
				t.Fatalf("live pack judgment = %s", j)
			}
		}
	}
	if judged != 2 {
		t.Fatalf("report judged %d packs, want 2", judged)
	}

	// The bucket saw only reads.
	if bucket.Puts() != puts || bucket.Copies() != copies || bucket.Hidden("") != 0 {
		t.Fatalf("bucket changed: puts %d -> %d, copies %d -> %d, hidden %d", puts, bucket.Puts(), copies, bucket.Copies(), bucket.Hidden(""))
	}
}

// TestS3SourceRefuses checks the S3 source refuses a store named twice or by
// halves, a missing key, every write and delete, and offers no move.
func TestS3SourceRefuses(t *testing.T) {
	// A store named twice, or by halves, or without keys is refused.
	t.Setenv(s3AccessKeyEnv, "")
	t.Setenv(s3SecretKeyEnv, "")
	for name, args := range map[string]*Args{
		"both":      {S3Endpoint: "s3.example", S3Bucket: "b", SFTPAddr: "h:22"},
		"no bucket": {S3Endpoint: "s3.example"},
		"no keys":   {S3Endpoint: "s3.example", S3Bucket: "b"},
		"no source": {},
	} {
		if _, _, _, err := args.dialBase(); err == nil {
			t.Fatalf("%s: dial passed", name)
		}
	}

	// The client refuses every change before it reaches the bucket.
	t.Setenv(s3AccessKeyEnv, "key")
	t.Setenv(s3SecretKeyEnv, "secret")
	bucket, _ := s3test.New(t)
	args := &Args{S3Endpoint: bucket.URL(), S3Bucket: s3test.BucketName}
	objects, root, closeStore, err := args.dialBase()
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	ctx := t.Context()
	if err := objects.PutObject(ctx, root, "k", []byte("v"), ""); err != errReadOnly {
		t.Fatalf("put = %v", err)
	}
	if err := objects.DeleteObject(ctx, root, "k"); err != errReadOnly {
		t.Fatalf("delete = %v", err)
	}
	if _, ok := objects.(block_store_s3.ObjectMover); ok {
		t.Fatal("the read-only source can move objects")
	}
	if bucket.Puts() != 0 || len(bucket.Keys("")) != 0 {
		t.Fatal("a refused change reached the bucket")
	}
}
