package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// checkCycle is a judged cycle over a small World, for the checks that prove
// what removing its candidates would lose. Its root reaches a leaf beside it,
// an inner block in a second pack, and that block's child alone in a third; a
// fourth pack holds a block nothing reaches, the one candidate.
type checkCycle struct {
	// args runs the command on the cycle's scratch file and bucket.
	args *Args
	// out is the command's output.
	out *bytes.Buffer
	// client is the bucket.
	client *block_store_s3.Client
	// root, leaf, inner, deep and dead are the blocks.
	root, leaf, inner, deep, dead *block.BlockRef
	// packs are the packs in write order: root and leaf, inner, deep, dead.
	packs []string
	// dir is the test's directory.
	dir string
}

// newCheckCycle writes the World, runs a cycle on it to its judgment, and
// writes the candidate list.
func newCheckCycle(ctx context.Context, t *testing.T) *checkCycle {
	// Write each pack through an index file, noting its id.
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()
	const storeID = "p/local/acc/blk/so"
	_, client := s3test.New(t)
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
	c := &checkCycle{client: client, dir: dir}
	c.root, c.leaf, c.inner, c.deep, c.dead = ref("root"), ref("leaf"), ref("inner"), ref("deep"), ref("dead")
	for _, batch := range [][]*block.PutBatchEntry{
		{
			{Ref: c.root, Data: []byte("root"), Refs: []*block.BlockRef{c.leaf, c.inner}, RefsKnown: true},
			{Ref: c.leaf, Data: []byte("leaf"), RefsKnown: true},
		},
		{{Ref: c.inner, Data: []byte("inner"), Refs: []*block.BlockRef{c.deep}, RefsKnown: true}},
		{{Ref: c.deep, Data: []byte("deep"), RefsKnown: true}},
		{{Ref: c.dead, Data: []byte("dead"), RefsKnown: true}},
	} {
		if _, err := seed.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
		packs, err := seed.Packs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range packs {
			if !slices.Contains(c.packs, p.GetId()) {
				c.packs = append(c.packs, p.GetId())
			}
		}
	}
	if len(c.packs) != 4 {
		t.Fatalf("seed packs = %v", c.packs)
	}
	seed.Close()
	if err := closeIndex(); err != nil {
		t.Fatal(err)
	}

	// Open the write log the judgment reads its position from.
	logDir := filepath.Join(dir, "log")
	wlog := writelog.New(le)
	if err := wlog.Set(&writelog.Options{Dir: logDir}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wlog.Close() })
	start, ok := wlog.Position()
	if !ok {
		t.Fatal("write log is off")
	}

	// Run the cycle to its judgment and write the candidates.
	c.out = &bytes.Buffer{}
	c.args = &Args{
		Scratch:     filepath.Join(dir, "scratch"),
		StoreID:     storeID,
		Index:       indexPath,
		Prefix:      "world/",
		Reads:       2,
		Roots:       *cli.NewStringSlice(c.root.MarshalString()),
		Batch:       1,
		MinAge:      time.Nanosecond,
		LowPercent:  packsweep.DefaultLowLivePercent,
		Report:      filepath.Join(dir, "report.jsonl"),
		Holes:       filepath.Join(dir, "holes.txt"),
		Candidates:  filepath.Join(dir, "candidates.jsonl"),
		Verdicts:    *cli.NewStringSlice(packsweep.VerdictDead.String()),
		LogDir:      logDir,
		LogPosition: start.String(),
		PullEvery:   time.Millisecond,
		MaxBad:      5,
		dial: func() (block_store_s3.ObjectStore, string, func(), error) {
			return client, s3test.BucketName, func() {}, nil
		},
		out: c.out,
	}
	for _, step := range []func(*cli.Context) error{c.args.runNewCycle, c.args.runSnapshot, c.args.runWalk, c.args.runJudge, c.args.runCandidates} {
		if err := step(nil); err != nil {
			t.Fatal(err)
		}
	}
	if ids, err := readCandidateIDs(c.args.Candidates); err != nil || len(ids) != 1 || !ids[c.packs[3]] {
		t.Fatalf("candidates = %v, %v, want the dead pack %s alone", ids, err, c.packs[3])
	}
	return c
}

// run runs a step on a copy of the cycle's arguments that edit changes,
// returning what it printed and its error.
func (c *checkCycle) run(t *testing.T, step func(*Args) error, edit func(*Args)) (*fastjson.Value, error) {
	args := *c.args
	edit(&args)
	c.out.Reset()
	runErr := step(&args)
	if c.out.Len() == 0 {
		return nil, runErr
	}
	res, err := fastjson.ParseBytes(c.out.Bytes())
	if err != nil {
		t.Fatalf("printed %q: %v", c.out.String(), err)
	}
	return res, runErr
}

// writeCandidates writes a candidate list naming ids, returning its path.
func (c *checkCycle) writeCandidates(t *testing.T, name string, ids ...string) string {
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(`{"id":"` + id + `","verdict":"dead"}` + "\n")
	}
	path := filepath.Join(c.dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCommandVerifyCandidates checks the verify with candidate packs left
// out, by trying to make it pass a candidate a live block needs. The real
// candidate passes, with the cycle's roots matching the fresh ones. A list
// that also names the pack holding the deep block alone fails, naming that
// block candidate. A fresh root that no longer reaches the first pack still
// fails when that pack is a candidate, because the cycle's root is walked too.
// The automatic mutant is caught, a mutant nothing reaches is not and fails
// the run, and a list line without an id is refused rather than skipped.
func TestCommandVerifyCandidates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := newCheckCycle(ctx, t)
	verify := func(a *Args) error { return a.runVerify(nil) }
	badReason := func(res *fastjson.Value, ref *block.BlockRef) string {
		for _, b := range res.GetArray("bad_blocks") {
			if string(b.GetStringBytes("ref")) == ref.MarshalString() {
				return string(b.GetStringBytes("reason"))
			}
		}
		return ""
	}

	// The real candidate passes, every live block read, the roots matching.
	res, err := c.run(t, verify, func(a *Args) {})
	if err != nil {
		t.Fatalf("real candidate failed: %v (%s)", err, res)
	}
	if res.GetUint64("blocks") != 4 || res.GetUint64("bad") != 0 || res.GetInt("candidates") != 1 {
		t.Fatalf("real candidate = %s, want 4 blocks, none bad, 1 candidate", res)
	}
	if res.GetInt("cycle_roots") != 1 || len(res.GetArray("roots_added")) != 0 || len(res.GetArray("roots_dropped")) != 0 {
		t.Fatalf("roots compared = %s, want the one cycle root matching", res)
	}

	// Adversary: leave out the deep block's only pack as well.
	res, err = c.run(t, verify, func(a *Args) {
		a.Candidates = c.writeCandidates(t, "deep.jsonl", c.packs[3], c.packs[2])
	})
	if err == nil {
		t.Fatalf("a live pack in the candidates passed: %s", res)
	}
	if res.GetUint64("bad") != 1 || badReason(res, c.deep) != packsweep.ReasonCandidate {
		t.Fatalf("live candidate = %s, want the deep block alone, named %s", res, packsweep.ReasonCandidate)
	}

	// Adversary: the fresh root has moved to the inner block, so a check on
	// fresh roots alone would pass the root's own pack; the cycle's root is
	// walked as well and fails it.
	res, err = c.run(t, verify, func(a *Args) {
		a.Roots = *cli.NewStringSlice(c.inner.MarshalString())
		a.Candidates = c.writeCandidates(t, "first.jsonl", c.packs[3], c.packs[0])
	})
	if err == nil {
		t.Fatalf("the cycle root's pack passed as a candidate: %s", res)
	}
	added, dropped := res.GetArray("roots_added"), res.GetArray("roots_dropped")
	if len(added) != 1 || string(added[0].GetStringBytes()) != c.inner.MarshalString() || len(dropped) != 1 || string(dropped[0].GetStringBytes()) != c.root.MarshalString() {
		t.Fatalf("moved roots = %s, want the inner added and the root dropped", res)
	}
	if badReason(res, c.root) != packsweep.ReasonCandidate {
		t.Fatalf("moved roots = %s, want the cycle's root named %s", res, packsweep.ReasonCandidate)
	}

	// The automatic mutant is caught, and its run passes.
	res, err = c.run(t, verify, func(a *Args) { a.Mutant = mutantAuto })
	if err != nil {
		t.Fatalf("automatic mutant: %v (%s)", err, res)
	}
	if !res.GetBool("mutant_caught") || !slices.Contains(c.packs[:3], string(res.GetStringBytes("mutant"))) {
		t.Fatalf("automatic mutant = %s, want a live pack caught", res)
	}

	// A mutant nothing reaches is not caught, and fails the run.
	res, err = c.run(t, verify, func(a *Args) {
		a.Candidates = ""
		a.Mutant = c.packs[3]
	})
	if err == nil || res.GetBool("mutant_caught") {
		t.Fatalf("a dead mutant passed: %v (%s)", err, res)
	}

	// A mutant already among the candidates is refused.
	if _, err = c.run(t, verify, func(a *Args) { a.Mutant = c.packs[3] }); err == nil {
		t.Fatal("a mutant among the candidates was taken")
	}

	// A line without an id is refused, not skipped.
	bad := filepath.Join(c.dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"id":"`+c.packs[3]+`"}`+"\n"+`{"verdict":"dead"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = c.run(t, verify, func(a *Args) { a.Candidates = bad }); err == nil {
		t.Fatal("a candidate line without an id was skipped")
	}
}
