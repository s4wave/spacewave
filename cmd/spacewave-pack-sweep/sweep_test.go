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
	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/block/writelog"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// TestCommandCycle runs a whole cycle of the command on one scratch file. The
// store holds a live pack under the root, with a child missing, and a dead
// pack. The judgment finds the dead pack, lists the missing child as a hole
// that blocks removal until it is accepted, keeps the dead pack once the write
// log shows a put of its block, and blocks removal once the log's epoch
// closes. A snapshot of the same scratch file on another prefix is refused.
func TestCommandCycle(t *testing.T) {
	for _, kind := range []string{packcopy.IndexSQLite, packcopy.IndexBolt, packcopy.IndexS4DB} {
		t.Run(kind, func(t *testing.T) { testCommandCycle(t, kind) })
	}
}

// testCommandCycle runs the cycle with the pack index in a file of the
// volume type kind, which the command detects.
func testCommandCycle(t *testing.T, kind string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()

	// Write a live pack, its root linking a child no pack holds, and a dead
	// pack, through an index file.
	const storeID = "p/local/acc/blk/so"
	_, client := s3test.New(t)
	indexPath := filepath.Join(dir, "index.db")
	index, closeIndex, err := packcopy.OpenIndexFile(ctx, indexPath, &packcopy.IndexFileOptions{
		Kind:   kind,
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
	rootRef, leafRef, lostRef, deadRef := ref("root"), ref("leaf"), ref("lost"), ref("dead")
	for _, batch := range [][]*block.PutBatchEntry{
		{
			{Ref: rootRef, Data: []byte("root"), Refs: []*block.BlockRef{leafRef, lostRef}, RefsKnown: true},
			{Ref: leafRef, Data: []byte("leaf"), RefsKnown: true},
		},
		{{Ref: deadRef, Data: []byte("dead"), RefsKnown: true}},
	} {
		if _, err := seed.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	packs, err := seed.Packs(ctx)
	if err != nil || len(packs) != 2 {
		t.Fatalf("seed packs = %d, %v", len(packs), err)
	}
	seed.Close()
	if err := closeIndex(); err != nil {
		t.Fatal(err)
	}

	// Turn on the backend's write log and read its position with the roots.
	logDir := filepath.Join(dir, "log")
	wlog := writelog.New(le)
	if err := wlog.Set(&writelog.Options{Dir: logDir}); err != nil {
		t.Fatal(err)
	}
	defer wlog.Close()
	position := func() writelog.Position {
		pos, ok := wlog.Position()
		if !ok {
			t.Fatal("write log is off")
		}
		return pos
	}
	start := position()

	// Point the command at the bucket and the log.
	var out bytes.Buffer
	args := &Args{
		Scratch:     filepath.Join(dir, "scratch"),
		StoreID:     storeID,
		Index:       indexPath,
		Prefix:      "world/",
		Reads:       2,
		Roots:       *cli.NewStringSlice(rootRef.MarshalString()),
		Batch:       1,
		MinAge:      time.Nanosecond,
		LowPercent:  packsweep.DefaultLowLivePercent,
		Report:      filepath.Join(dir, "report.jsonl"),
		Holes:       filepath.Join(dir, "holes.txt"),
		LogDir:      logDir,
		LogPosition: start.String(),
		PullEvery:   time.Millisecond,
		dial: func() (block_store_s3.ObjectStore, string, func(), error) {
			return client, s3test.BucketName, func() {}, nil
		},
		out: &out,
	}

	// Run the cycle. The walk refuses to start before the snapshot.
	if err := args.runNewCycle(nil); err != nil {
		t.Fatal(err)
	}
	if err := args.runWalk(nil); err == nil {
		t.Fatal("walk ran before the snapshot")
	}
	if err := args.runSnapshot(nil); err != nil {
		t.Fatal(err)
	}
	if err := args.runWalk(nil); err != nil {
		t.Fatal(err)
	}
	wlog.Put(rootRef.GetHash())
	position()
	out.Reset()
	if err := args.runJudge(nil); err != nil {
		t.Fatal(err)
	}
	result := parse(t, out.Bytes())
	if result.GetUint("summary", "packs", "dead") != 1 || result.GetUint("summary", "packs", "kept") != 1 {
		t.Fatalf("summary = %s", result)
	}
	if result.GetBool("removable") || result.GetUint("blocking", "missing") != 1 || result.Get("blocking", packsweep.BlockLogUnread) != nil {
		t.Fatalf("missing child does not block alone: %s", result)
	}

	// The report names the dead pack, the one holding no live key.
	verdicts := make(map[string]string)
	report, err := os.Open(args.Report)
	if err != nil {
		t.Fatal(err)
	}
	defer report.Close()
	lines := bufio.NewScanner(report)
	for lines.Scan() {
		j := parse(t, lines.Bytes())
		verdict := string(j.GetStringBytes("verdict"))
		verdicts[string(j.GetStringBytes("id"))] = verdict
		if verdict == "dead" && (j.GetUint("blocks") != 1 || j.GetUint("liveBlocks") != 0) {
			t.Fatalf("dead pack judgment = %s", j)
		}
	}
	if len(verdicts) != 2 {
		t.Fatalf("report = %v", verdicts)
	}

	// Accepting the lost child from the holes file makes the cycle removable.
	holes, err := os.ReadFile(args.Holes)
	if err != nil {
		t.Fatal(err)
	}
	if string(holes) != lostRef.MarshalString()+" missing\n" {
		t.Fatalf("holes file = %q", holes)
	}
	args.AcceptMissing = args.Holes
	out.Reset()
	if err := args.runJudge(nil); err != nil {
		t.Fatal(err)
	}
	if result = parse(t, out.Bytes()); !result.GetBool("removable") {
		t.Fatalf("accepted hole still blocks: %s", result)
	}

	// A put of the dead block since R keeps its pack: the pull stores it and
	// the closure walks it.
	wlog.Put(deadRef.GetHash())
	position()
	out.Reset()
	if err := args.runPullLog(nil); err != nil {
		t.Fatal(err)
	}
	if pull := parse(t, out.Bytes()); pull.GetUint("lines") != 2 || pull.GetUint("seeds") != 1 {
		t.Fatalf("pull = %s", pull)
	}
	if err := args.runClosure(nil); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := args.runJudge(nil); err != nil {
		t.Fatal(err)
	}
	if result = parse(t, out.Bytes()); result.GetUint("summary", "packs", "dead") != 0 || result.GetUint("summary", "packs", "kept") != 2 {
		t.Fatalf("put since R left its pack dead: %s", result)
	}

	// Closing the log's epoch ends its cover of the cycle.
	if err := wlog.Close(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := args.runJudge(nil); err != nil {
		t.Fatal(err)
	}
	if result = parse(t, out.Bytes()); result.GetBool("removable") || result.GetUint("blocking", packsweep.BlockLogGap) != 1 {
		t.Fatalf("closed log does not block: %s", result)
	}

	// The status reads the head, and another prefix is refused.
	out.Reset()
	if err := args.runStatus(nil); err != nil {
		t.Fatal(err)
	}
	if status := parse(t, out.Bytes()); !status.GetBool("walked") || status.GetStringBytes("logPull", "gap") == nil {
		t.Fatalf("status = %s", out.String())
	}
	other := *args
	other.Prefix = "other/"
	if err := other.runSnapshot(nil); err == nil {
		t.Fatal("snapshot of another prefix ran on the same scratch file")
	}
}

// parse parses one JSON value.
func parse(t *testing.T, data []byte) *fastjson.Value {
	t.Helper()
	v, err := fastjson.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse %q: %v", data, err)
	}
	return v
}
