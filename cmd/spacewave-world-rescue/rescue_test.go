//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

const (
	testRoot    = "hyperline-packs"
	testPrefix  = "world/packs/"
	testStoreID = "world-store"
)

// fakeServer serves the source packs from a local directory as the FTPS
// client would, counting calls and failing or stalling them on request.
type fakeServer struct {
	dir   string
	mtx   sync.Mutex
	calls int
	whole int
	// sizes counts the SIZE calls; sizeErr fails them.
	sizes   int
	sizeErr error
	// hook runs before call n (from 1) and may delay or fail it.
	hook func(n int) (time.Duration, error)
}

func (f *fakeServer) before(ctx context.Context) error {
	f.mtx.Lock()
	f.calls++
	n := f.calls
	f.mtx.Unlock()
	if f.hook == nil {
		return nil
	}
	delay, err := f.hook(n)
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (f *fakeServer) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if err := f.before(ctx); err != nil {
		return nil, err
	}
	f.mtx.Lock()
	f.whole++
	f.mtx.Unlock()
	return dirObjects{}.GetObject(ctx, filepath.Join(f.dir, bucket), key)
}

func (f *fakeServer) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	if err := f.before(ctx); err != nil {
		return nil, err
	}
	return dirObjects{}.GetObjectRange(ctx, filepath.Join(f.dir, bucket), key, off, length)
}

// ObjectSize answers SIZE from the file, or with sizeErr.
func (f *fakeServer) ObjectSize(ctx context.Context, bucket, key string) (int64, error) {
	if err := f.before(ctx); err != nil {
		return 0, err
	}
	f.mtx.Lock()
	f.sizes++
	f.mtx.Unlock()
	if f.sizeErr != nil {
		return 0, f.sizeErr
	}
	info, err := os.Stat(objectPath(filepath.Join(f.dir, bucket), key))
	if err != nil {
		return 0, block_store_s3.ErrNotFound
	}
	return info.Size(), nil
}

func (f *fakeServer) count() (calls, whole int) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.calls, f.whole
}

// source is a source PackStore on a local directory with its index.
type source struct {
	t     *testing.T
	dir   string
	kv    kvtx.Store
	index kvtx.Store
	store *block_store_s3.PackStore
	// data holds each block's bytes by block key.
	data      map[string][]byte
	closeOnce sync.Once
}

// close stops the source store, so no background write touches its index.
func (s *source) close() {
	s.closeOnce.Do(s.store.Close)
}

// stripExtents closes the source and rewrites the refs' h rows to the
// 4-byte form, the pack number alone.
func (s *source) stripExtents(refs []*block.BlockRef) {
	s.close()
	ctx := context.Background()
	tx, err := s.index.NewTransaction(ctx, true)
	if err != nil {
		s.t.Fatal(err)
	}
	defer tx.Discard()
	for _, ref := range refs {
		key := block_store_s3.IndexHashRowKey(packfile.BlockKey(ref.GetHash()))
		value, ok, err := tx.Get(ctx, key)
		if err != nil || !ok || len(value) < 4 {
			s.t.Fatalf("h row: %v %v", ok, err)
		}
		if err := tx.Set(ctx, key, append([]byte(nil), value[:4]...)); err != nil {
			s.t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.t.Fatal(err)
	}
}

func newSource(t *testing.T) *source {
	dir := t.TempDir()
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	index := kvtx_prefixer.NewPrefixer(kv, block_store_s3.PackIndexPrefix(testStoreID))
	store := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), dirObjects{}, filepath.Join(dir, testRoot), testPrefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true, SoleWriter: true, Index: index,
	})
	src := &source{t: t, dir: dir, kv: kv, index: index, store: store, data: make(map[string][]byte)}
	t.Cleanup(src.close)
	return src
}

// pack writes one pack of n random blocks and returns their refs.
func (s *source) pack(n int) []*block.BlockRef {
	var batch []*block.PutBatchEntry
	var refs []*block.BlockRef
	for range n {
		data := make([]byte, 300)
		_, _ = rand.Read(data)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			s.t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		refs = append(refs, ref)
		s.data[string(packfile.BlockKey(ref.GetHash()))] = data
	}
	if _, err := s.store.PutBlockBatch(context.Background(), batch); err != nil {
		s.t.Fatal(err)
	}
	return refs
}

// writeList writes the refs as a block-pass list.
func writeList(t *testing.T, dir string, refs []*block.BlockRef) string {
	var b strings.Builder
	for _, ref := range refs {
		fmt.Fprintf(&b, "hyperline-world %s %d\n", hex.EncodeToString(packfile.BlockKey(ref.GetHash())), 300)
	}
	path := filepath.Join(dir, "refs.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// testOptions returns the options of a run over src into out.
func testOptions(src *source, list, out string) *options {
	return &options{
		Bucket: testRoot, Prefix: testPrefix, Index: src.index, RefList: list,
		Ledger: filepath.Join(out, "rescue.ledger"), OutDir: out, Sessions: 1,
		Plan: planOptions{Overhead: 100},
		Fetch: fetchOptions{
			Timeout: 5 * time.Second, Retries: 2, Backoff: time.Millisecond, MaxBackoff: 4 * time.Millisecond,
			CapBytes: 1 << 40,
		},
	}
}

// checkOutput reads the refs back through a PackStore on the output and
// reports how many it holds, failing on any wrong bytes.
func checkOutput(t *testing.T, out string, refs []*block.BlockRef) int {
	t.Helper()
	store := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), dirObjects{}, out, "", &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer store.Close()
	var n int
	for _, ref := range refs {
		data, ok, err := store.GetBlock(context.Background(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		if err := ref.VerifyData(data, true); err != nil {
			t.Fatalf("output block %s: %v", ref.MarshalString(), err)
		}
		n++
	}
	return n
}

// TestChoosePlan checks that every pack is read whole unless ranges are on,
// every wanted block has an extent and ranges cost fewer bytes, and that the
// cost is the smaller of the two.
func TestChoosePlan(t *testing.T) {
	entry := &packfile.PackfileEntry{SizeBytes: 100_000, BlockCount: 100}
	wants := func(ext, noExt int, size uint32) []want {
		var w []want
		for range ext {
			w = append(w, want{extent: true, size: size})
		}
		for range noExt {
			w = append(w, want{})
		}
		return w
	}
	off := planOptions{Overhead: 100}
	on := planOptions{Ranges: true, Overhead: 100}
	cases := []struct {
		name string
		p    *packWork
		o    planOptions
		mode planMode
		cost uint64
		ok   bool
	}{
		{"ranges off whole", &packWork{entry: entry, wants: wants(1, 0, 10)}, off, planWhole, 100_100, true},
		{"ranges on cheaper ranges", &packWork{entry: entry, wants: wants(40, 0, 100)}, on, planRanges, 40 * 200, true},
		{"ranges on dearer whole", &packWork{entry: entry, wants: wants(3, 0, 40_000)}, on, planWhole, 100_100, true},
		{"ranges on tie whole", &packWork{entry: entry, wants: wants(1, 0, 100_000)}, on, planWhole, 100_100, true},
		{"ranges on no extent whole", &packWork{entry: entry, wants: wants(1, 1, 10)}, on, planWhole, 100_100, true},
		{"no entry ranges off cannot", &packWork{wants: wants(2, 0, 10)}, off, planWhole, infinite, false},
		{"no entry ranges on ranges", &packWork{wants: wants(2, 0, 10)}, on, planRanges, 2 * 110, true},
		{"no entry no extent cannot", &packWork{wants: wants(1, 1, 10)}, on, planWhole, infinite, false},
	}
	for _, c := range cases {
		mode, cost, ok := choosePlan(c.p, c.o)
		if mode != c.mode || cost != c.cost || ok != c.ok {
			t.Errorf("%s: got %v %d %v, want %v %d %v", c.name, mode, cost, ok, c.mode, c.cost, c.ok)
		}
		if ok && cost != min(wholeCost(c.p, c.o), rangesCost(c.p, c.o)) {
			t.Errorf("%s: cost %d is not the least", c.name, cost)
		}
	}
}

// TestOrderPacks orders packs by wanted refs, most first.
func TestOrderPacks(t *testing.T) {
	packs := []*packWork{{id: "a", wants: make([]want, 3)}, {id: "b", wants: make([]want, 1700)}, {id: "c", wants: make([]want, 14)}, {id: "d", wants: make([]want, 14)}}
	orderPacks(packs)
	var got string
	for _, p := range packs {
		got += p.id
	}
	if got != "bcda" {
		t.Fatalf("order %s, want bcda", got)
	}
}

// TestIsHardStop checks the hard stop classifier.
func TestIsHardStop(t *testing.T) {
	stop := []error{
		&textproto.Error{Code: 530, Msg: "Login incorrect."},
		&textproto.Error{Code: 421, Msg: "Service not available"},
		fmt.Errorf("log in to h:21: %w", &textproto.Error{Code: 530, Msg: "Sorry, the maximum number of clients (10) for this user are already connected."}),
		errors.New("connect to h:21: 421 Too many connections (8) from this IP"),
		errors.New("server says: login limit reached"),
	}
	for _, err := range stop {
		if !isHardStop(err) {
			t.Errorf("not a hard stop: %v", err)
		}
	}
	pass := []error{
		nil,
		errTimeout,
		&textproto.Error{Code: 550, Msg: "No such file"},
		&textproto.Error{Code: 451, Msg: "Transfer aborted"},
		errors.New("read 530 bytes then EOF"),
	}
	for _, err := range pass {
		if isHardStop(err) {
			t.Errorf("hard stop: %v", err)
		}
	}
}

// packLines returns the ledger's pack lines' fields.
func packLines(t *testing.T, out string) [][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "rescue.ledger"))
	if err != nil {
		t.Fatal(err)
	}
	var lines [][]string
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) != 0 && f[0] == "p" {
			lines = append(lines, f)
		}
	}
	return lines
}

// TestRescueWhole reads two packs whole, keeps only the wanted blocks in
// synthetic pfv1_ packs that kvfile.BuildReader opens, and records each pack's
// fetched bytes, found and not-in-pack in the ledger.
func TestRescueWhole(t *testing.T) {
	src := newSource(t)
	big := src.pack(17)
	small := src.pack(16)
	// One block of the big pack is not wanted: it must not reach the output.
	refs := append(append([]*block.BlockRef{}, big[:16]...), small...)
	out := t.TempDir()
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), testOptions(src, writeList(t, out, refs), out), srv)
	if err != nil {
		t.Fatal(err)
	}
	calls, whole := srv.count()
	if whole != 2 || calls != 2 {
		t.Fatalf("calls %d whole %d, want 2 and 2", calls, whole)
	}
	if res.Done != 32 || res.Whole != 2 || res.Ranged != 0 || res.PacksDone != 2 {
		t.Fatalf("result %+v", res)
	}
	if n := checkOutput(t, out, refs); n != 32 {
		t.Fatalf("output holds %d of 32", n)
	}
	if n := checkOutput(t, out, big[16:]); n != 0 {
		t.Fatal("an unwanted block was stored")
	}

	// The ledger's pack lines hold the bytes read and the wanted found.
	var sum uint64
	for _, f := range packLines(t, out) {
		if len(f) != 5 || f[3] != "16" || f[4] != "0" {
			t.Fatalf("pack line %v", f)
		}
		n, _ := strconv.ParseUint(f[2], 10, 64)
		sum += n
	}
	if sum != res.Bytes || sum == 0 {
		t.Fatalf("pack lines sum %d, total %d", sum, res.Bytes)
	}

	// The walker's view: every file in packs/ is a new pfv1_ pack, none a
	// source pack, and opens standalone with kvfile.BuildReader.
	files, err := os.ReadDir(filepath.Join(out, "packs"))
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]bool)
	for _, f := range files {
		if !strings.HasPrefix(f.Name(), "pfv1_") {
			t.Fatalf("pack file %s", f.Name())
		}
		if _, err := os.Stat(objectPath(filepath.Join(src.dir, testRoot), testPrefix+"packs/"+f.Name())); err == nil {
			t.Fatalf("output pack %s is a source pack", f.Name())
		}
		if _, err := os.Stat(filepath.Join(out, "entries", f.Name())); err != nil {
			t.Fatalf("pack %s has no entry: %v", f.Name(), err)
		}
		data, err := os.ReadFile(filepath.Join(out, "packs", f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range refs {
			key := packfile.BlockKey(ref.GetHash())
			value, ok, err := rd.Get(key)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				continue
			}
			if _, stored, err := packfile.DecodeBlockValue(key, value); err != nil || !bytes.Equal(stored.Data, src.data[string(key)]) {
				t.Fatalf("pack %s block: %v", f.Name(), err)
			}
			keys[string(key)] = true
		}
	}
	if len(files) == 0 || len(keys) != 32 {
		t.Fatalf("packs %d, blocks %d", len(files), len(keys))
	}
}

// TestRescueRanges reads by ranges only with the flag on, and then only
// where every wanted block has an extent.
func TestRescueRanges(t *testing.T) {
	src := newSource(t)
	refs := src.pack(20)
	stripped := src.pack(20)
	src.stripExtents(stripped)
	wanted := append(append([]*block.BlockRef{}, refs[:2]...), stripped[:2]...)

	// Off by default: both packs whole.
	out := t.TempDir()
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), testOptions(src, writeList(t, out, wanted), out), srv)
	if err != nil {
		t.Fatal(err)
	}
	if calls, whole := srv.count(); calls != 2 || whole != 2 || res.Done != 4 || res.Ranged != 0 {
		t.Fatalf("off: calls %d whole %d, %+v", calls, whole, res)
	}

	// On: two ranges for the pack with extents, the stripped pack whole.
	out = t.TempDir()
	srv = &fakeServer{dir: src.dir}
	o := testOptions(src, writeList(t, out, wanted), out)
	o.Plan.Ranges = true
	res, err = run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if calls, whole := srv.count(); calls != 3 || whole != 1 || res.Done != 4 || res.Ranged != 1 || res.Whole != 1 {
		t.Fatalf("on: calls %d whole %d, %+v", calls, whole, res)
	}
	if n := checkOutput(t, out, wanted); n != 4 {
		t.Fatalf("output holds %d of 4", n)
	}
}

// TestRescueOrderMinWanted fetches the packs most wanted first and leaves
// those at or below --min-wanted for a later run.
func TestRescueOrderMinWanted(t *testing.T) {
	src := newSource(t)
	var refs []*block.BlockRef
	for _, n := range []int{5, 2, 9} {
		refs = append(refs, src.pack(n)...)
	}
	out := t.TempDir()
	list := writeList(t, out, refs)
	o := testOptions(src, list, out)
	o.MinWanted = 4
	res, err := run(context.Background(), o, &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.PacksTotal != 2 || res.Done != 14 || res.Deferred != 2 {
		t.Fatalf("result %+v", res)
	}
	lines := packLines(t, out)
	if len(lines) != 2 || lines[0][3] != "9" || lines[1][3] != "5" {
		t.Fatalf("pack lines %v", lines)
	}

	// A later run takes the rest.
	o.MinWanted = 0
	res, err = run(context.Background(), o, &fakeServer{dir: src.dir})
	if err != nil || res.PacksTotal != 1 || res.Done != 16 {
		t.Fatalf("%v, %+v", err, res)
	}
}

// TestRescueNotInPack points a ref's index row at another pack: the pack
// lacks it, so the ref is recorded not-in-pack and skipped on resume.
func TestRescueNotInPack(t *testing.T) {
	src := newSource(t)
	a, b := src.pack(3), src.pack(3)
	ctx := context.Background()
	tx, err := src.index.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := tx.Get(ctx, block_store_s3.IndexHashRowKey(packfile.BlockKey(b[0].GetHash())))
	if err == nil {
		err = tx.Set(ctx, block_store_s3.IndexHashRowKey(packfile.BlockKey(a[0].GetHash())), row)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, append(append([]*block.BlockRef{}, a...), b...)), out)
	res, err := run(ctx, o, &fakeServer{dir: src.dir})
	if err != nil || res.Done != 5 || res.NotInPack != 1 || res.Failed != 0 {
		t.Fatalf("%v, %+v", err, res)
	}
	ledger, _ := os.ReadFile(filepath.Join(out, "rescue.ledger"))
	if !strings.Contains(string(ledger), hex.EncodeToString(packfile.BlockKey(a[0].GetHash()))+" not-in-pack") {
		t.Fatalf("ledger lacks the not-in-pack ref:\n%s", ledger)
	}
	var marked bool
	for _, f := range packLines(t, out) {
		marked = marked || (f[3] == "3" && f[4] == "1")
	}
	if !marked {
		t.Fatalf("no pack line with 3 found and 1 not in pack: %v", packLines(t, out))
	}
	srv := &fakeServer{dir: src.dir}
	res, err = run(ctx, o, srv)
	if c, _ := srv.count(); err != nil || c != 0 || res.NotInPack != 1 {
		t.Fatalf("resume: %v, calls %d, %+v", err, c, res)
	}
}

// TestRescueHashMismatch points one ref's index row at another block's
// extent and reads by ranges: the bytes do not hash to the ref, so it fails
// and is not stored.
func TestRescueHashMismatch(t *testing.T) {
	src := newSource(t)
	refs := src.pack(4)
	ctx := context.Background()
	tx, err := src.index.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := tx.Get(ctx, block_store_s3.IndexHashRowKey(packfile.BlockKey(refs[1].GetHash())))
	if err == nil {
		err = tx.Set(ctx, block_store_s3.IndexHashRowKey(packfile.BlockKey(refs[0].GetHash())), other)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, refs), out)
	o.Plan.Ranges, o.Plan.Overhead = true, 0
	res, err := run(ctx, o, &fakeServer{dir: src.dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 3 || res.Failed != 1 || res.Ranged != 1 {
		t.Fatalf("result %+v", res)
	}
	if n := checkOutput(t, out, refs[:1]); n != 0 {
		t.Fatal("the mismatched block was stored")
	}
	ledger, _ := os.ReadFile(filepath.Join(out, "rescue.ledger"))
	if !strings.Contains(string(ledger), hex.EncodeToString(packfile.BlockKey(refs[0].GetHash()))+" failed hash-mismatch") {
		t.Fatalf("ledger lacks the failure:\n%s", ledger)
	}
}

// TestRescueHardStop stops at once on a 421 or a 530, with no retry, and
// returns a hard stop.
func TestRescueHardStop(t *testing.T) {
	for _, code := range []int{421, 530} {
		src := newSource(t)
		refs := append(src.pack(4), src.pack(4)...)
		out := t.TempDir()
		srv := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
			if n == 2 {
				return 0, &textproto.Error{Code: code, Msg: "Too many connections"}
			}
			return 0, nil
		}}
		_, err := run(context.Background(), testOptions(src, writeList(t, out, refs), out), srv)
		var hard *hardStopError
		if !errors.As(err, &hard) {
			t.Fatalf("%d: got %v, want a hard stop", code, err)
		}
		if calls, _ := srv.count(); calls != 2 {
			t.Fatalf("%d: %d calls, want 2 with no retry", code, calls)
		}
	}
}

// TestRescueTimeoutRetry retries a request that outlives its timeout and
// then succeeds.
func TestRescueTimeoutRetry(t *testing.T) {
	src := newSource(t)
	refs := src.pack(3)
	out := t.TempDir()
	srv := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
		if n == 1 {
			return time.Second, nil
		}
		return 0, nil
	}}
	o := testOptions(src, writeList(t, out, refs), out)
	o.Fetch.Timeout = 50 * time.Millisecond
	res, err := run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if calls, _ := srv.count(); calls != 2 || res.Done != 3 || res.Requests != 2 {
		t.Fatalf("calls %d, result %+v", calls, res)
	}
}

// TestRescueResume stops a run partway on a hard stop, then resumes: the
// second run fetches only what the first did not store, and a third fetches
// nothing.
func TestRescueResume(t *testing.T) {
	src := newSource(t)
	var refs []*block.BlockRef
	for range 3 {
		refs = append(refs, src.pack(4)...)
	}
	out := t.TempDir()
	list := writeList(t, out, refs)
	first := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
		if n == 2 {
			return 0, &textproto.Error{Code: 421, Msg: "bye"}
		}
		return 0, nil
	}}
	res1, err := run(context.Background(), testOptions(src, list, out), first)
	var hard *hardStopError
	if !errors.As(err, &hard) || res1.Done != 4 || res1.PacksDone != 1 {
		t.Fatalf("first run: %v, %+v", err, res1)
	}

	second := &fakeServer{dir: src.dir}
	res2, err := run(context.Background(), testOptions(src, list, out), second)
	if err != nil {
		t.Fatal(err)
	}
	if calls, _ := second.count(); calls != 2 || res2.Done != 12 || res2.PacksTotal != 2 {
		t.Fatalf("second run: calls %d, %+v", calls, res2)
	}
	if res2.Bytes <= res1.Bytes {
		t.Fatalf("byte total did not carry over: %d then %d", res1.Bytes, res2.Bytes)
	}

	third := &fakeServer{dir: src.dir}
	res3, err := run(context.Background(), testOptions(src, list, out), third)
	if calls, _ := third.count(); err != nil || calls != 0 || res3.PacksTotal != 0 {
		t.Fatalf("third run: %v, calls %d, %+v", err, calls, res3)
	}
	if n := checkOutput(t, out, refs); n != 12 {
		t.Fatalf("output holds %d of 12", n)
	}
}

// TestRescueNoExtent reads a pack whose index rows hold no extent whole,
// counting its refs apart.
func TestRescueNoExtent(t *testing.T) {
	src := newSource(t)
	refs := src.pack(20)
	src.stripExtents(refs)
	out := t.TempDir()
	srv := &fakeServer{dir: src.dir}
	o := testOptions(src, writeList(t, out, refs[:3]), out)
	o.Plan.Ranges = true
	res, err := run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if _, whole := srv.count(); whole != 1 || res.Whole != 1 || res.NoExtent != 3 || res.NoExtentDone != 3 || res.Done != 3 {
		t.Fatalf("whole %d, result %+v", whole, res)
	}
	if n := checkOutput(t, out, refs[:3]); n != 3 {
		t.Fatalf("output holds %d of 3", n)
	}
}

// TestRescueByteCap stops cleanly before a request that would cross the
// cap, keeps the byte total for the next run, and refuses a zero cap.
func TestRescueByteCap(t *testing.T) {
	src := newSource(t)
	var refs []*block.BlockRef
	for range 3 {
		refs = append(refs, src.pack(4)...)
	}
	out := t.TempDir()
	list := writeList(t, out, refs)
	o := testOptions(src, list, out)
	o.Fetch.CapBytes = 0
	if _, err := run(context.Background(), o, &fakeServer{dir: src.dir}); err == nil {
		t.Fatal("a zero cap ran")
	}

	// Each pack is read whole: a cap of two and a half packs takes two.
	var size int64
	err := dirObjects{}.ListObjects(context.Background(), filepath.Join(src.dir, testRoot), testPrefix+"packs/", func(_ string, n int64) error {
		size = max(size, n)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	o.Fetch.CapBytes = uint64(2*size + size/2)
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), o, srv)
	if err != nil || res.Stop != stopByteCap || res.Bytes > o.Fetch.CapBytes {
		t.Fatalf("%v, %+v", err, res)
	}
	if calls, _ := srv.count(); calls != 2 || res.Done != 8 {
		t.Fatalf("calls %d, result %+v", calls, res)
	}

	// The next run starts from the recorded total and fetches nothing more.
	srv2 := &fakeServer{dir: src.dir}
	res2, err := run(context.Background(), o, srv2)
	if c, _ := srv2.count(); err != nil || c != 0 || res2.Stop != stopByteCap || res2.Bytes != res.Bytes {
		t.Fatalf("second run: %v, calls %d, %+v", err, c, res2)
	}
}

// TestIndexReadOnlyOpen opens a WAL index file through openIndexView and reads it,
// leaving the file, its -wal and its -shm byte for byte as they were.
func TestIndexReadOnlyOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index.db")
	w, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, defaultIndexTable, store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := w.NewTransaction(ctx, true)
	if err == nil {
		err = tx.Set(ctx, []byte("k"), []byte("v"))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	// Copy the files beside the open writer, as the index copy was taken,
	// with its -wal and -shm.
	copyDir := filepath.Join(t.TempDir(), "copy")
	if err := os.Mkdir(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(path + s)
		if err == nil {
			err = os.WriteFile(filepath.Join(copyDir, "index.db"+s), data, 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	path = filepath.Join(copyDir, "index.db")
	snapshot := func() map[string][]byte {
		m := make(map[string][]byte)
		for _, s := range []string{"", "-wal", "-shm"} {
			data, err := os.ReadFile(path + s)
			if err != nil {
				t.Fatal(err)
			}
			m[s] = data
		}
		return m
	}
	before := snapshot()
	if len(before["-wal"]) == 0 {
		t.Fatal("no WAL to check")
	}

	r, err := openIndexView(ctx, path, filepath.Join(filepath.Dir(path), "view"), defaultIndexTable)
	if err != nil {
		t.Fatal(err)
	}
	rtx, err := r.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, err := rtx.Get(ctx, []byte("k")); err != nil || !ok || string(v) != "v" {
		t.Fatalf("read: %q %v %v", v, ok, err)
	}
	rtx.Discard()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	after := snapshot()
	for s, data := range before {
		if !bytes.Equal(data, after[s]) {
			t.Errorf("index%s changed", s)
		}
	}
}

// dropEntry closes the source and removes a pack's e row from its index, so
// the index gives no size for the pack.
func (s *source) dropEntry(id string) {
	s.close()
	ctx := context.Background()
	tx, err := s.index.NewTransaction(ctx, true)
	if err != nil {
		s.t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, []byte("e/"+id)); err != nil {
		s.t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		s.t.Fatal(err)
	}
}

// onlyPack returns the id and size of the only pack of src.
func onlyPack(t *testing.T, src *source) (string, int64) {
	var id string
	var size int64
	err := dirObjects{}.ListObjects(context.Background(), filepath.Join(src.dir, testRoot), testPrefix+"packs/", func(key string, n int64) error {
		id, size = key[strings.LastIndex(key, "/")+1:], n
		return nil
	})
	if err != nil || id == "" {
		t.Fatalf("no pack: %v", err)
	}
	return id, size
}

// TestRescueSizeNoEntry reads a pack the index gives no size for: SIZE on the
// server sizes it, the size is charged against the cap, and its wanted blocks
// come from the pack's own index. A failed SIZE or one past the memory guard
// records the pack and fetches nothing.
func TestRescueSizeNoEntry(t *testing.T) {
	// Write a pack and drop its e row.
	src := newSource(t)
	refs := src.pack(6)
	id, size := onlyPack(t, src)
	src.dropEntry(id)

	// SIZE answers: the pack is read whole and every wanted block stored.
	out := t.TempDir()
	srv := &fakeServer{dir: src.dir}
	o := testOptions(src, writeList(t, out, refs[:4]), out)
	o.Fetch.CapBytes = uint64(size)
	res, err := run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if _, whole := srv.count(); srv.sizes != 1 || whole != 1 || res.Sized != 1 || res.Done != 4 || res.Bytes != uint64(size) {
		t.Fatalf("sizes %d whole %d, %+v", srv.sizes, whole, res)
	}
	if n := checkOutput(t, out, refs[:4]); n != 4 {
		t.Fatalf("output holds %d of 4", n)
	}

	// A cap a byte short of the SIZE answer stops before the fetch.
	out = t.TempDir()
	srv = &fakeServer{dir: src.dir}
	o = testOptions(src, writeList(t, out, refs[:4]), out)
	o.Fetch.CapBytes = uint64(size) - 1
	res, err = run(context.Background(), o, srv)
	if _, whole := srv.count(); err != nil || whole != 0 || res.Stop != stopByteCap {
		t.Fatalf("short cap: %v, whole %d, %+v", err, whole, res)
	}

	// SIZE fails: the pack is recorded no-size and not fetched.
	out = t.TempDir()
	srv = &fakeServer{dir: src.dir, sizeErr: &textproto.Error{Code: 550, Msg: "SIZE not allowed"}}
	res, err = run(context.Background(), testOptions(src, writeList(t, out, refs[:4]), out), srv)
	if _, whole := srv.count(); err != nil || whole != 0 || res.Failed != 4 {
		t.Fatalf("failed size: %v, whole %d, %+v", err, whole, res)
	}
	if l := packLines(t, out); len(l) != 1 || len(l[0]) != 6 || l[0][5] != "no-size" {
		t.Fatalf("pack lines %v", l)
	}

	// SIZE past the memory guard: recorded too-large and not fetched.
	out = t.TempDir()
	srv = &fakeServer{dir: src.dir}
	o = testOptions(src, writeList(t, out, refs[:4]), out)
	o.MaxPackBytes = uint64(size) - 1
	res, err = run(context.Background(), o, srv)
	if _, whole := srv.count(); err != nil || whole != 0 || res.TooLarge != 4 {
		t.Fatalf("too large: %v, whole %d, %+v", err, whole, res)
	}
}

// TestRescueTooLarge skips a pack whose entry is past the memory guard,
// recording its refs too-large with no request, and skips them on resume.
func TestRescueTooLarge(t *testing.T) {
	src := newSource(t)
	big := src.pack(8)
	small := src.pack(1)
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, append(append([]*block.BlockRef{}, big...), small...)), out)
	o.MaxPackBytes = 2000
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), o, srv)
	if calls, _ := srv.count(); err != nil || calls != 1 || res.TooLarge != 8 || res.Done != 1 {
		t.Fatalf("%v, calls %d, %+v", err, calls, res)
	}
	var skipped bool
	for _, f := range packLines(t, out) {
		skipped = skipped || (len(f) == 6 && f[2] == "0" && f[5] == "too-large")
	}
	if !skipped {
		t.Fatalf("no too-large pack line: %v", packLines(t, out))
	}
	srv = &fakeServer{dir: src.dir}
	res, err = run(context.Background(), o, srv)
	if calls, _ := srv.count(); err != nil || calls != 0 || res.TooLarge != 8 {
		t.Fatalf("resume: %v, calls %d, %+v", err, calls, res)
	}
}

// TestRescueLedgerPerPack syncs the byte total after every pack, ahead of
// the blocks' flush, so a crash loses no finished pack's bytes.
func TestRescueLedgerPerPack(t *testing.T) {
	src := newSource(t)
	var refs []*block.BlockRef
	for range 3 {
		refs = append(refs, src.pack(2)...)
	}
	out := t.TempDir()
	if _, err := run(context.Background(), testOptions(src, writeList(t, out, refs), out), &fakeServer{dir: src.dir}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "rescue.ledger"))
	if err != nil {
		t.Fatal(err)
	}
	var totals []uint64
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "r" && f[2] == "done" {
			break
		}
		if len(f) == 2 && f[0] == "b" {
			n, _ := strconv.ParseUint(f[1], 10, 64)
			totals = append(totals, n)
		}
	}
	// One total per pack lands before any ref line, rising.
	if len(totals) != 3 || totals[0] == 0 || totals[0] > totals[1] || totals[1] > totals[2] {
		t.Fatalf("byte totals before the first block line: %v", totals)
	}
}

// TestRescueLedgerTiming checks each fetched pack gets a t line with its
// time and requests after its p line, and that a ledger without t lines, as
// an older run wrote, still loads its packs.
func TestRescueLedgerTiming(t *testing.T) {
	// Rescue two packs and find their t lines.
	src := newSource(t)
	refs := append(src.pack(2), src.pack(2)...)
	out := t.TempDir()
	if _, err := run(context.Background(), testOptions(src, writeList(t, out, refs), out), &fakeServer{dir: src.dir}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, "rescue.ledger")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	packs := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 4 && f[0] == "t" && packs[f[1]] {
			if ms, err := strconv.ParseInt(f[2], 10, 64); err != nil || ms < 0 || f[3] == "0" {
				t.Fatalf("bad t line %q", line)
			}
			delete(packs, f[1])
			packs["timed "+f[1]] = true
			continue
		}
		if len(f) >= 2 && f[0] == "p" {
			packs[f[1]] = true
		}
		kept = append(kept, line)
	}
	timed := 0
	for k := range packs {
		if strings.HasPrefix(k, "timed ") {
			timed++
		} else {
			t.Fatalf("pack %s has no t line after its p line:\n%s", k, data)
		}
	}
	if timed != 2 {
		t.Fatalf("%d packs timed, want 2:\n%s", timed, data)
	}

	// The ledger with its t lines dropped loads both packs.
	old := filepath.Join(t.TempDir(), "old.ledger")
	if err := os.WriteFile(old, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := openLedger(old)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if len(l.packs) != 2 {
		t.Fatalf("old ledger loads %d packs, want 2", len(l.packs))
	}
}

// sharedNumSource writes three packs; gives the second the first's number,
// and points the h rows of the second and one block of the third at it. It
// reports clean when no pack's bloom filter holds a key of another, which
// random blocks give most of the time.
func sharedNumSource(ctx context.Context, t *testing.T) (src *source, a, b, c []*block.BlockRef, clean bool) {
	// Write three packs; give the second the first's number, and point the
	// h rows of the second and one block of the third at it.
	src = newSource(t)
	a, b, c = src.pack(4), src.pack(4), src.pack(1)
	src.close()
	tx, err := src.index.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	hrow := func(ref *block.BlockRef) []byte {
		return block_store_s3.IndexHashRowKey(packfile.BlockKey(ref.GetHash()))
	}
	numA, _, err := tx.Get(ctx, hrow(a[0]))
	if err != nil {
		t.Fatal(err)
	}
	rowB, _, err := tx.Get(ctx, hrow(b[0]))
	if err != nil {
		t.Fatal(err)
	}
	rowB = append([]byte(nil), rowB...)
	num := append([]byte(nil), numA[:4]...)
	for _, ref := range append(append([]*block.BlockRef{}, b...), c...) {
		if err := tx.Set(ctx, hrow(ref), num); err != nil {
			t.Fatal(err)
		}
	}
	// Give b's pack a's p row, read before the h rows changed.
	before, err := block_store_s3.ReadIndexPackIDs(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	idOf := func(value []byte) string {
		row, ok := block_store_s3.ParseIndexHashRow(value)
		if !ok || len(before[row.Num]) != 1 {
			t.Fatalf("h row %x", value)
		}
		return before[row.Num][0]
	}
	idA, idB := idOf(numA), idOf(rowB)
	pa, _, err := tx.Get(ctx, []byte("p/"+idA))
	if err != nil {
		t.Fatal(err)
	}
	pb := []byte("p/" + idB)
	if err := tx.Set(ctx, pb, pa); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, _ = src.index.NewTransaction(ctx, false)
	ids, err := block_store_s3.ReadIndexPackIDs(ctx, tx)
	tx.Discard()
	if err != nil {
		t.Fatal(err)
	}
	var shared bool
	for _, list := range ids {
		shared = shared || len(list) == 2
	}
	if !shared {
		t.Fatalf("no shared number: %v", ids)
	}

	// Test each pack's filter against the other packs' keys.
	tx, _ = src.index.NewTransaction(ctx, false)
	defer tx.Discard()
	filters := make(map[string]*packWork, 2)
	for _, id := range []string{idA, idB} {
		entry, err := block_store_s3.ReadIndexEntry(ctx, tx, id)
		if err != nil || entry == nil {
			t.Fatalf("e row of %s: %v", id, err)
		}
		filters[id] = &packWork{entry: entry}
	}
	holds := func(id string, refs []*block.BlockRef) bool {
		for _, ref := range refs {
			if filters[id].mayHold(packfile.BlockKey(ref.GetHash())) {
				return true
			}
		}
		return false
	}
	clean = !holds(idB, a) && !holds(idA, b) && !holds(idA, c) && !holds(idB, c)
	return src, a, b, c, clean
}

// TestRescueSharedNum gives two packs one pack number: each wanted ref goes
// to the pack whose bloom filter may hold it, and a ref neither filter holds
// is recorded ambiguous-num.
func TestRescueSharedNum(t *testing.T) {
	// Build packs on a shared number with no cross-pack false positive.
	ctx := context.Background()
	var src *source
	var a, b, c []*block.BlockRef
	for try := 0; ; try++ {
		var clean bool
		src, a, b, c, clean = sharedNumSource(ctx, t)
		if clean {
			break
		}
		if try == 20 {
			t.Fatal("every source had a bloom false positive")
		}
	}

	// Run: every ref of a and b is stored, the c ref is ambiguous, and no
	// pack fetched lacks a ref it was asked for.
	out := t.TempDir()
	wanted := append(append(append([]*block.BlockRef{}, a...), b...), c...)
	srv := &fakeServer{dir: src.dir}
	res, err := run(ctx, testOptions(src, writeList(t, out, wanted), out), srv)
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 8 || res.Ambiguous != 1 || res.NotInPack != 0 || res.PacksDone != 2 {
		t.Fatalf("result %+v", res)
	}
	for _, f := range packLines(t, out) {
		if f[3] != "4" || f[4] != "0" {
			t.Fatalf("pack line %v: the filter did not pick the pack", f)
		}
	}
	if n := checkOutput(t, out, wanted); n != 8 {
		t.Fatalf("output holds %d of 8", n)
	}
}

// TestSettle records a ref of several candidate packs done by the first that
// stored it, or by the last result when none did.
func TestSettle(t *testing.T) {
	r := &rescuer{multi: map[string]int{"x": 2, "y": 2}, multiDone: map[string]bool{}}
	if r.settle([]byte("x"), false) || !r.settle([]byte("x"), true) || r.settle([]byte("x"), false) {
		t.Fatal("x: a miss then a hit records the hit alone")
	}
	if r.settle([]byte("y"), false) || !r.settle([]byte("y"), false) {
		t.Fatal("y: two misses record the last")
	}
	if !r.settle([]byte("z"), false) {
		t.Fatal("z: a ref of one pack records its result")
	}
}

// redialServer is a fakeServer that keeps the redial hook the fetcher sets.
type redialServer struct {
	*fakeServer
	hook func(ctx context.Context, err error) error
}

// OnRedial keeps fn for the test to call.
func (r *redialServer) OnRedial(fn func(ctx context.Context, err error) error) {
	r.hook = fn
}

// TestFetcherRedialHook checks that a redial logs one line, counts as a
// request and takes a request token, that a redial the rate limit holds back
// returns its error, and that under StopOnDrop the hook refuses the redial
// with no token.
func TestFetcherRedialHook(t *testing.T) {
	// Build a fetcher at one request per second that logs to a buffer.
	srv := &redialServer{fakeServer: &fakeServer{dir: t.TempDir()}}
	var log bytes.Buffer
	f := newFetcher(srv, "b", fetchOptions{RequestsPerSec: 1, CapBytes: 1 << 20, Log: &log}, 0)
	if srv.hook == nil {
		t.Fatal("no redial hook set")
	}

	// The first redial takes the one token at once and logs the error.
	cerr := errors.New("connection reset by peer")
	if err := srv.hook(context.Background(), cerr); err != nil {
		t.Fatalf("first redial: %v", err)
	}
	want := "ftps redial after a connection error: connection reset by peer\n"
	if log.String() != want || f.requests.Load() != 1 {
		t.Fatalf("log %q, requests %d; want %q and 1", log.String(), f.requests.Load(), want)
	}

	// A second redial must wait for a token; under a short deadline it fails.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := srv.hook(ctx, cerr); err == nil {
		t.Fatal("second redial took no token")
	}
	if n := strings.Count(log.String(), "\n"); n != 2 {
		t.Fatalf("%d log lines, want 2", n)
	}

	// Under StopOnDrop the hook refuses at once and logs the refusal.
	log.Reset()
	newFetcher(srv, "b", fetchOptions{RequestsPerSec: 1, CapBytes: 1 << 20, Log: &log, StopOnDrop: true}, 0)
	err := srv.hook(context.Background(), cerr)
	want = "ftps redial refused after a connection error: connection reset by peer\n"
	if !errors.Is(err, errRedialRefused) || !isDrop(err) || log.String() != want {
		t.Fatalf("refused redial %v, log %q; want errRedialRefused and %q", err, log.String(), want)
	}
}

// dropOptions returns testOptions with no retries and StopOnDrop, logging to
// log.
func dropOptions(src *source, list, out string, log io.Writer) *options {
	o := testOptions(src, list, out)
	o.Fetch.Retries = 0
	o.Fetch.StopOnDrop = true
	o.Log = log
	return o
}

// TestRescueStopOnDrop drops the connection on the second of three packs:
// the run stops at once with the drop line, the third pack is never asked
// for, the dropped pack's refs stay unrecorded, and a restart on the same
// output and ledger fetches the second and third packs only.
func TestRescueStopOnDrop(t *testing.T) {
	// Fail the second request as a lost connection fails, with no reply.
	src := newSource(t)
	var refs []*block.BlockRef
	for range 3 {
		refs = append(refs, src.pack(4)...)
	}
	out := t.TempDir()
	list := writeList(t, out, refs)
	first := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
		if n == 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, nil
	}}
	var log bytes.Buffer
	res1, err := run(context.Background(), dropOptions(src, list, out, &log), first)

	// The drop is a hard stop after two requests, logged once.
	var hard *hardStopError
	if !errors.As(err, &hard) || res1.Done != 4 || res1.PacksDone != 1 {
		t.Fatalf("first run: %v, %+v", err, res1)
	}
	if calls, _ := first.count(); calls != 2 {
		t.Fatalf("first run: %d calls, want 2: no pack after the drop", calls)
	}
	if n := strings.Count(log.String(), "stop: connection dropped: "); n != 1 {
		t.Fatalf("%d drop lines in %q, want 1", n, log.String())
	}

	// The restart fetches the dropped pack again and the one after it.
	second := &fakeServer{dir: src.dir}
	res2, err := run(context.Background(), dropOptions(src, list, out, io.Discard), second)
	if calls, _ := second.count(); err != nil || calls != 2 || res2.Done != 12 || res2.PacksTotal != 2 {
		t.Fatalf("restart: %v, calls %d, %+v", err, calls, res2)
	}
	if n := checkOutput(t, out, refs); n != 12 {
		t.Fatalf("output holds %d of 12", n)
	}
}

// TestRescueResumeAfterKill leaves in the adopt output what a kill -9 in the
// middle of a pack's write leaves: a half written temporary file, a staged
// pack with no staged entry, and a ledger line for one of the pack's refs
// cut before its newline. The restart counts none of them: it fetches the
// pack again, stores its refs and records them, and every published pack
// verifies.
func TestRescueResumeAfterKill(t *testing.T) {
	// Run until the second pack's request drops: the first pack is done.
	src := newSource(t)
	a, b := src.pack(4), src.pack(4)
	refs := append(append([]*block.BlockRef{}, a...), b...)
	out := t.TempDir()
	list := writeList(t, out, refs)
	srv := &fakeServer{dir: src.dir, hook: func(n int) (time.Duration, error) {
		if n == 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, nil
	}}
	o := dropOptions(src, list, out, io.Discard)
	o.Layout = layoutAdopt
	if res, err := run(context.Background(), o, srv); err == nil || res.Done != 4 {
		t.Fatalf("first run: %v, %+v", err, res)
	}
	packs0, keys0 := checkAdoptDir(t, out)

	// Leave a kill's debris for the second pack: a torn temporary file, a
	// staged pack of half its bytes with no entry, and a torn ledger line.
	names, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var published string
	for _, n := range names {
		if id, ok := strings.CutSuffix(n.Name(), adoptEntrySuffix); ok {
			published = id
		}
	}
	data, err := os.ReadFile(filepath.Join(out, published))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, dirTempPrefix+"killed"), data[:len(data)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	staged := "pfv1_killedmidwrite" + adoptPartSuffix
	if err := os.WriteFile(filepath.Join(out, staged), data[:len(data)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	lf, err := os.OpenFile(filepath.Join(out, "rescue.ledger"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(lf, "r %s done", hex.EncodeToString(packfile.BlockKey(b[0].GetHash())))
	if cerr := lf.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}

	// The restart counts only the first pack's refs as done and fetches the
	// second pack again.
	again := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), o, again)
	if calls, _ := again.count(); err != nil || calls != 1 || res.Done != 8 || res.PacksTotal != 1 || res.PacksDone != 1 {
		t.Fatalf("restart: %v, calls %d, %+v", err, calls, res)
	}

	// Every published pack verifies, and they hold the eight refs once more
	// than before: the second pack's four.
	packs1, keys1 := checkAdoptDir(t, out)
	if packs1 != packs0+1 || keys1 != keys0+4 {
		t.Fatalf("packs %d to %d, keys %d to %d", packs0, packs1, keys0, keys1)
	}
	got := make(map[string]bool)
	names, err = os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		id, ok := strings.CutSuffix(n.Name(), adoptEntrySuffix)
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(out, id))
		if err != nil {
			t.Fatal(err)
		}
		index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		for _, ie := range index {
			off, size := ie.GetOffset(), ie.GetSize()
			if _, ok := verifyKeyed(ie.GetKey(), data[off:off+size]); !ok {
				t.Fatalf("pack %s holds a block that does not verify", id)
			}
			got[string(ie.GetKey())] = true
		}
	}
	for _, ref := range refs {
		if !got[string(packfile.BlockKey(ref.GetHash()))] {
			t.Fatalf("ref %s missing from the output", ref.MarshalString())
		}
	}
}

// TestRescuePackList plans only the pack the list names: the other packs'
// refs are deferred with no ledger line.
func TestRescuePackList(t *testing.T) {
	// Write three packs and name the one holding the first.
	src := newSource(t)
	a := src.pack(4)
	refs := append(append(append([]*block.BlockRef{}, a...), src.pack(4)...), src.pack(4)...)
	out := t.TempDir()
	list := writeList(t, out, refs)
	ctx := context.Background()
	tx, err := src.index.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := block_store_s3.ReadIndexPackIDs(ctx, tx)
	if err != nil || len(ids) != 3 {
		t.Fatalf("pack ids %v, %v", ids, err)
	}
	value, ok, err := tx.Get(ctx, block_store_s3.IndexHashRowKey(packfile.BlockKey(a[0].GetHash())))
	tx.Discard()
	row, parsed := block_store_s3.ParseIndexHashRow(value)
	if err != nil || !ok || !parsed || len(ids[row.Num]) != 1 {
		t.Fatalf("h row of the first ref: %v %v %v", ok, parsed, err)
	}
	named := ids[row.Num][0]

	// Only that pack is fetched; the rest are deferred, unrecorded.
	o := testOptions(src, list, out)
	o.Packs = map[string]bool{named: true}
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), o, srv)
	if calls, _ := srv.count(); err != nil || calls != 1 || res.Done != 4 || res.Deferred != 8 || res.PacksTotal != 1 {
		t.Fatalf("%v, calls %d, %+v", err, calls, res)
	}
	if n := checkOutput(t, out, a); n != 4 {
		t.Fatalf("output holds %d of 4", n)
	}
	raw, err := os.ReadFile(filepath.Join(out, "rescue.ledger"))
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for line := range strings.Lines(string(raw)) {
		if strings.HasPrefix(line, "r ") {
			n++
		}
	}
	if n != 4 {
		t.Fatalf("%d ref lines in the ledger, want 4", n)
	}
}
