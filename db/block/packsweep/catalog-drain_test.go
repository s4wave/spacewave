package packsweep

import (
	"bytes"
	"context"
	"encoding/hex"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// slowStore delays or holds each write commit.
type slowStore struct {
	kvtx.Store
	// delay is slept before each write commit.
	delay time.Duration
	// gate, when set, holds each write commit once armed until it closes
	// or passes one.
	gate chan struct{}
	// armed starts holding commits on the gate.
	armed atomic.Bool
	// started counts the write commits started once armed.
	started atomic.Int64
}

// NewTransaction wraps the write transactions.
func (s *slowStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &slowTx{Tx: tx, s: s}, nil
}

// slowTx is a write transaction of a slowStore.
type slowTx struct {
	kvtx.Tx
	s *slowStore
}

// Commit waits, then commits.
func (t *slowTx) Commit(ctx context.Context) error {
	if t.s.armed.Load() {
		t.s.started.Add(1)
		if t.s.gate != nil {
			<-t.s.gate
		}
	}
	time.Sleep(t.s.delay)
	return t.Tx.Commit(ctx)
}

// trackedPacks records the packs it read whole, and abandons a read when
// its context ends.
type trackedPacks struct {
	PackSource
	// delay is each read's time.
	delay time.Duration
	// arm, when set, is armed at the first read, so the snapshot list
	// commits before the store holds commits.
	arm  *slowStore
	mu   sync.Mutex
	read []string
}

// ReadPackFile reads the pack after the delay unless ctx ends first.
func (p *trackedPacks) ReadPackFile(ctx context.Context, id string, size uint64) ([]byte, error) {
	if p.arm != nil {
		p.arm.armed.Store(true)
	}
	select {
	case <-time.After(p.delay):
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	data, err := p.PackSource.ReadPackFile(ctx, id, size)
	if err == nil {
		p.mu.Lock()
		p.read = append(p.read, id)
		p.mu.Unlock()
	}
	return data, err
}

// reads returns the packs read whole.
func (p *trackedPacks) reads() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.read)
}

// catalogRows returns every catalog row of the store.
func catalogRows(t *testing.T, store kvtx.Store) map[string]string {
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	rows := make(map[string]string)
	err = tx.ScanPrefix(ctx, []byte("c/"), func(key, value []byte) error {
		rows[string(key)] = string(value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestSnapshotDrainsOnCancel cancels a snapshot partway through slow commits
// and checks every pack read whole was committed, no read started after the
// cancel, and the resumed catalog equals an uninterrupted one.
func TestSnapshotDrainsOnCancel(t *testing.T) {
	f := newFixture(t, 21, fixtureOptions{blocks: 400, roots: 2, packBlocks: 3, dupShare: 0.2})
	ids := f.packIDs(t)
	if len(ids) < 60 {
		t.Fatalf("fixture has %d packs, want 60 or more", len(ids))
	}

	// Cancel once a few batches committed.
	store := &slowStore{Store: store_kvtx_inmem.NewStore(), delay: 15 * time.Millisecond}
	src := &trackedPacks{PackSource: f.store, delay: 2 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var readsAtCancel atomic.Int64
	progress := func(p SnapshotProgress) {
		if p.Catalogued >= 5 && ctx.Err() == nil {
			readsAtCancel.Store(int64(len(src.reads())))
			cancel()
		}
	}
	_, err := Snapshot(ctx, &SnapshotOptions{Store: store, Packs: src, Reads: 4, Progress: progress})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot returned %v, want canceled", err)
	}

	// Every pack read whole has its row.
	rows := catalogRows(t, store)
	read := src.reads()
	for _, id := range read {
		if _, ok := rows[catalogPackPrefix+id]; !ok {
			t.Errorf("pack %s read before the cancel has no catalog row", id)
		}
	}
	var catalogued int
	for key := range rows {
		if len(key) > len(catalogPackPrefix) && key[:len(catalogPackPrefix)] == catalogPackPrefix {
			catalogued++
		}
	}
	if catalogued != len(read) {
		t.Errorf("%d packs catalogued, %d read", catalogued, len(read))
	}
	// At the cancel, at most the queue, the reads waiting to queue and the
	// reads in flight remained to finish.
	if extra := len(read) - int(readsAtCancel.Load()); extra > 4 {
		t.Errorf("%d reads finished after the cancel, want at most 4", extra)
	}
	if len(read) >= len(ids) {
		t.Fatalf("cancel came after every read: %d of %d", len(read), len(ids))
	}
	t.Logf("%d of %d packs read and committed at the cancel", len(read), len(ids))

	// Resume, and match an uninterrupted run.
	store.delay = 0
	if _, err := Snapshot(context.Background(), &SnapshotOptions{Store: store, Packs: f.store, Reads: 4}); err != nil {
		t.Fatal(err)
	}
	clean := store_kvtx_inmem.NewStore()
	if _, err := Snapshot(context.Background(), &SnapshotOptions{Store: clean, Packs: f.store, Reads: 4}); err != nil {
		t.Fatal(err)
	}
	if got, want := catalogRows(t, store), catalogRows(t, clean); !mapsEqual(got, want) {
		t.Fatalf("resumed catalog has %d rows, uninterrupted %d, or they differ", len(got), len(want))
	}
}

// TestSnapshotQueueBound holds the first commit and checks the reads stop
// once the queue fills.
func TestSnapshotQueueBound(t *testing.T) {
	f := newFixture(t, 22, fixtureOptions{blocks: 300, roots: 2, packBlocks: 3})
	ids := f.packIDs(t)
	const reads = 2
	gate := make(chan struct{})
	store := &slowStore{Store: store_kvtx_inmem.NewStore(), gate: gate}
	src := &trackedPacks{PackSource: f.store, arm: store}

	// Run with the catalog commits held.
	done := make(chan error, 1)
	go func() {
		_, err := Snapshot(context.Background(), &SnapshotOptions{Store: store, Packs: src, Reads: reads})
		done <- err
	}()

	// Wait for the reads to settle.
	deadline := time.Now().Add(5 * time.Second)
	last, stable := -1, 0
	for stable < 20 {
		if time.Now().After(deadline) {
			t.Fatal("reads did not settle")
		}
		time.Sleep(5 * time.Millisecond)
		n := len(src.reads())
		if n == last && store.started.Load() > 0 {
			stable++
		} else {
			last, stable = n, 0
		}
	}

	// Held: the batch committing, at most one more than the queue; the
	// queue; and a parsed pack in each read slot.
	q := readyQueue(reads)
	if bound := 1 + q + q + reads; last > bound {
		t.Fatalf("%d packs read while the first commit is held, bound %d", last, bound)
	}
	t.Logf("%d of %d packs read while the first commit is held", last, len(ids))

	// Release the commits; every pack is catalogued.
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	rows := catalogRows(t, store)
	for _, id := range ids {
		if _, ok := rows[catalogPackPrefix+id]; !ok {
			t.Fatalf("pack %s not catalogued", id)
		}
	}
}

// TestSnapshotDrainTimeout holds the catalog commits past the cancel and
// the drain timeout, and checks the snapshot then starts no further commit
// and returns.
func TestSnapshotDrainTimeout(t *testing.T) {
	f := newFixture(t, 23, fixtureOptions{blocks: 120, roots: 1, packBlocks: 3})
	gate := make(chan struct{})
	store := &slowStore{Store: store_kvtx_inmem.NewStore(), gate: gate}
	src := &trackedPacks{PackSource: f.store, arm: store}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Snapshot(ctx, &SnapshotOptions{Store: store, Packs: src, Reads: 2, DrainTimeout: 100 * time.Millisecond})
		done <- err
	}()

	// Cancel with the first catalog commit held, let the drain timeout
	// pass, then pass that one commit.
	for store.started.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	time.Sleep(300 * time.Millisecond)
	gate <- struct{}{}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "drain exceeded") {
			t.Fatalf("snapshot returned %v, want the drain timeout", err)
		}
		if n := store.started.Load(); n != 1 {
			t.Fatalf("%d catalog commits started, want 1", n)
		}
		t.Logf("returned: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot did not return after the drain timeout")
	}
}

// TestReadPackDropsBuffer checks a parsed pack does not keep its pack's
// bytes alive.
func TestReadPackDropsBuffer(t *testing.T) {
	// Write one pack of large values.
	f := newFixture(t, 24, fixtureOptions{blocks: 1, roots: 1, packBlocks: 1})
	ctx := context.Background()
	var batch []*block.PutBatchEntry
	for i := range 512 {
		data := bytes.Repeat([]byte{byte(i), byte(i >> 8)}, 8<<10)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
	}
	before := f.packIDs(t)
	if _, err := f.store.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	var id string
	for _, p := range f.packIDs(t) {
		if !slices.Contains(before, p) {
			id = p
		}
	}
	entries, err := f.store.Packs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var size uint64
	for _, e := range entries {
		if e.GetId() == id {
			size = e.GetSizeBytes()
		}
	}
	if size < 4<<20 {
		t.Fatalf("pack is %d bytes, want 4 MiB or more", size)
	}

	// Hold many parsed copies; the heap must grow far less than their packs.
	const copies = 16
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc
	held := make([]*readPack, 0, copies)
	for range copies {
		rp, err := readCatalogPack(ctx, f.store, pendingPack{id: id, row: snapshotPack{size: size}})
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, rp)
	}
	runtime.GC()
	runtime.ReadMemStats(&ms)
	grown := int64(ms.HeapAlloc) - int64(base)
	t.Logf("%d parsed packs of %d bytes hold %d heap bytes", copies, size, grown)
	if grown > int64(size) {
		t.Fatalf("%d parsed packs hold %d heap bytes, more than one pack of %d", copies, grown, size)
	}
	runtime.KeepAlive(held)
}

// TestCatalogRowsGolden pins the bytes of the catalog rows one pack writes,
// as the code before the drain change wrote them, so a scratch file already
// holding a catalog stays valid.
func TestCatalogRowsGolden(t *testing.T) {
	ctx := context.Background()
	tail := func(b byte) []byte { return bytes.Repeat([]byte{b}, TailBytes) }
	ref, err := block.BuildBlockRef([]byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := marshalRefs(refsKnown, []*block.BlockRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	rp := &readPack{
		id:  "pack-a",
		row: catalogPack{blocks: 3, valueBytes: 300, size: 1 << 20, created: 1700000000},
		keys: []packKey{
			{tail: tail(1), member: member{valueSize: 100}, state: refsKnown, refs: refs},
			{tail: tail(2), member: member{valueSize: 120, flags: memberRefsUnknown}, state: refsUnknown},
			{tail: tail(3), member: member{valueSize: 80, flags: memberCorrupt}, state: refsCorrupt},
		},
	}
	worse := &readPack{
		id:   "pack-b",
		row:  catalogPack{blocks: 1, valueBytes: 100, size: 4096},
		keys: []packKey{{tail: tail(1), member: member{valueSize: 100, flags: memberCorrupt}, state: refsCorrupt}},
	}
	store := store_kvtx_inmem.NewStore()
	if err := commitCatalogPacks(ctx, store, []*readPack{rp, worse}); err != nil {
		t.Fatal(err)
	}

	// Join the rows in key order.
	rows := catalogRows(t, store)
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range rows {
			if !yield(k) {
				return
			}
		}
	})
	var got bytes.Buffer
	for _, k := range keys {
		got.WriteString(hex.EncodeToString([]byte(k)))
		got.WriteByte('=')
		got.WriteString(hex.EncodeToString([]byte(rows[k])))
		got.WriteByte('\n')
	}
	if got.String() != catalogGolden {
		t.Fatalf("catalog rows changed:\n%s", got.String())
	}
}

// catalogGolden is the rows TestCatalogRowsGolden writes, recorded from the
// code at a3fa68264.
const catalogGolden = `632f6d2f7061636b2d612f01010101010101010101010101010101=6400
632f6d2f7061636b2d612f02020202020202020202020202020202=7802
632f6d2f7061636b2d612f03030303030303030303030303030303=5001
632f6d2f7061636b2d622f01010101010101010101010101010101=6401
632f702f7061636b2d61=03ac0280804080c49fd50c
632f702f7061636b2d62=0164802000
632f722f01010101010101010101010101010101=02260a2408011220ddc9e669194254cef019a29d3619a2c16592e5d52e1a81e98b01bd52319149a3
632f722f02020202020202020202020202020202=01
632f722f03030303030303030303030303030303=00
`
