//go:build !tinygo

package packsweep

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/packfile"
)

// failObjects fails the first put of a key fail matches, writing nothing.
type failObjects struct {
	block_store_s3.ObjectStore
	// fail matches the key whose put fails.
	fail func(key string) bool
	// failed is set once it failed.
	failed bool
}

// PutObject fails the first put of a matching key.
func (o *failObjects) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	if !o.failed && o.fail != nil && o.fail(key) {
		o.failed = true
		return errors.New("killed")
	}
	return o.ObjectStore.PutObject(ctx, bucket, key, data, contentType)
}

// packValue is one value of a pack's file.
type packValue struct {
	// entry is its index entry.
	entry *kvfile.IndexEntry
	// block is its decoded block.
	block *block.StoredBlock
}

// readPack reads every value of pack id from the fixture's bucket, by offset.
func (h *holdFixture) readPack(t *testing.T, prefix, id string) []packValue {
	t.Helper()
	ctx := t.Context()
	ap, err := readAdopted(ctx, h.f.client, s3test.BucketName, prefix, id)
	if err != nil {
		t.Fatal(err)
	}
	key := block_store_s3.PackObjectKey(prefix, id)
	read := func(ctx context.Context, off int64, length int) ([]byte, error) {
		return h.f.client.GetObjectRange(ctx, s3test.BucketName, key, off, length)
	}
	size := ap.Entry.GetSizeBytes()
	tail, err := packfile.ReadIndexTail(ctx, size, packfile.IndexTailReadSize(ap.Entry.GetBlockCount()), read)
	if err != nil {
		t.Fatal(err)
	}
	var out []packValue
	err = packfile.ScanIndexTail(tail, size, func(e *kvfile.IndexEntry) error {
		data, err := read(ctx, int64(e.GetOffset()), int(e.GetSize())) //nolint:gosec // test sizes.
		if err != nil {
			return err
		}
		b, err := block.DecodeBlockObject(data)
		out = append(out, packValue{entry: &kvfile.IndexEntry{Key: bytes.Clone(e.GetKey()), Offset: e.GetOffset(), Size: e.GetSize()}, block: b})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(out, func(a, b packValue) int { return int(a.entry.GetOffset()) - int(b.entry.GetOffset()) }) //nolint:gosec // test offsets.
	return out
}

// TestCompact carries the low packs through compaction and quarantine: the
// quarantine leaves them until their live blocks are placed, a compaction
// killed before its file, before its entry and after the backend's adoption
// resumes without losing or doubling a block, each new pack but the last is
// full, and a key relinked after the quarantine lifts its victim, which the
// next compaction and quarantine take again.
func TestCompact(t *testing.T) {
	ctx := t.Context()
	h := newHoldFixture(t)
	const prefix = "v2/"
	const maxPack = 2048
	expect := h.f.expect(t, 30)
	verdicts, _ := readResult(t, h.store)
	var low []string
	for id, v := range verdicts {
		if v == VerdictLow {
			low = append(low, id)
		}
	}
	slices.Sort(low)
	if len(low) < 4 {
		t.Fatalf("fixture has %d low packs, want 4", len(low))
	}

	// The live blocks of the victims, by key.
	want := make(map[string]*fixtureBlock)
	for _, id := range low {
		for _, b := range h.f.packs[id] {
			if expect.live[b.ref.MarshalString()] {
				want[string(packfile.BlockKey(b.ref.GetHash()))] = b
			}
		}
	}

	// The quarantine leaves the low packs before their blocks are placed.
	if res, err := Quarantine(ctx, h.opts); err != nil || res.Unplaced != uint64(len(low)) { //nolint:gosec // test counts.
		t.Fatalf("quarantine before compaction = %+v, %v, want %d unplaced", res, err, len(low))
	}
	objects := &failObjects{ObjectStore: h.f.client}
	copts := &CompactOptions{CoverOptions: *h.opts, Objects: objects, Bucket: s3test.BucketName, Prefix: prefix, MaxPackBytes: maxPack}
	isFile := func(key string) bool { return strings.HasPrefix(key, block_store_s3.PackObjectKey(prefix, "")) }
	isEntry := func(key string) bool { return strings.HasPrefix(key, block_store_s3.EntryObjectKey(prefix, "")) }
	unadopted := func() []string {
		rows, err := readCompactPacks(ctx, h.store)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			if !r.adopted {
				out = append(out, r.id)
			}
		}
		return out
	}

	// A run killed before its first file leaves a record and no objects.
	objects.fail = isFile
	if _, err := Compact(ctx, copts); err == nil {
		t.Fatal("killed compaction returned no error")
	}
	first := unadopted()
	if len(first) != 1 || h.objectAt(t, block_store_s3.PackObjectKey(prefix, first[0])) {
		t.Fatalf("unadopted packs after a killed file = %v", first)
	}

	// A run killed before its first entry drops the first record and leaves
	// its own pack's file.
	objects.fail, objects.failed = isEntry, false
	if _, err := Compact(ctx, copts); err == nil {
		t.Fatal("killed compaction returned no error")
	}
	second := unadopted()
	if len(second) != 1 || second[0] == first[0] || !h.objectAt(t, block_store_s3.PackObjectKey(prefix, second[0])) || h.objectAt(t, block_store_s3.EntryObjectKey(prefix, second[0])) {
		t.Fatalf("unadopted packs after a killed entry = %v", second)
	}

	// A run killed after the backend adopted its pack deletes the earlier
	// file, and the pack it wrote stays unadopted in the sweep.
	objects.fail = nil
	h.backend.failAdopt = true
	if _, err := Compact(ctx, copts); err == nil {
		t.Fatal("killed adoption returned no error")
	}
	third := unadopted()
	if len(third) != 1 || h.objectAt(t, block_store_s3.PackObjectKey(prefix, second[0])) || len(h.backend.adoptOrder) != 1 || h.backend.adoptOrder[0] != third[0] {
		t.Fatalf("unadopted packs after a killed adoption = %v, adopted %v", third, h.backend.adoptOrder)
	}

	// The quarantine leaves the victims while the pack is unadopted.
	if res, err := Quarantine(ctx, h.opts); err != nil || res.Unplaced != uint64(len(low)) || res.Quarantined != 0 { //nolint:gosec // test counts.
		t.Fatalf("quarantine with an unadopted pack = %+v, %v", res, err)
	}

	// A full run adopts the killed run's pack and writes the rest.
	res, err := Compact(ctx, copts)
	if err != nil || res.Resumed != 1 || res.Dropped != 0 || res.Packs == 0 {
		t.Fatalf("compaction = %+v, %v", res, err)
	}
	if n := len(unadopted()); n != 0 {
		t.Fatalf("%d packs unadopted after the compaction", n)
	}

	// Each live block of the victims reads from exactly one new pack, and
	// each new pack but the last is full.
	got := make(map[string]string)
	var prev []packValue
	for i, id := range h.backend.adoptOrder {
		values := h.readPack(t, prefix, id)
		keys := slices.Clone(h.backend.adopted[id])
		slices.SortFunc(keys, bytes.Compare)
		var listed [][]byte
		var used int64
		for _, v := range values {
			key := string(v.entry.GetKey())
			b := want[key]
			if b == nil || got[key] != "" || !bytes.Equal(v.block.Data, b.data) {
				t.Fatalf("pack %s holds key %x: wanted %v, also in %q", id, key, b != nil, got[key])
			}
			got[key] = id
			listed = append(listed, v.entry.GetKey())
			used += int64(len(v.entry.GetKey())) + int64(v.entry.GetSize()) + packEntryBytes //nolint:gosec // test sizes.
		}
		slices.SortFunc(listed, bytes.Compare)
		if !slices.EqualFunc(keys, listed, bytes.Equal) {
			t.Fatalf("pack %s adopted %d keys, its index lists %d", id, len(keys), len(listed))
		}
		if used > maxPack-packTrailerBytes {
			t.Fatalf("pack %s uses %d bytes over its bound", id, used)
		}
		if i > 0 {
			var prevUsed int64
			for _, v := range prev {
				prevUsed += int64(len(v.entry.GetKey())) + int64(v.entry.GetSize()) + packEntryBytes //nolint:gosec // test sizes.
			}
			next := int64(len(values[0].entry.GetKey())) + int64(values[0].entry.GetSize()) + packEntryBytes //nolint:gosec // test sizes.
			if prevUsed+next <= maxPack-packTrailerBytes {
				t.Fatalf("pack %s has room for the next pack's first block", h.backend.adoptOrder[i-1])
			}
		}
		prev = values
	}
	if len(got) != len(want) {
		t.Fatalf("the new packs hold %d live blocks, want %d", len(got), len(want))
	}

	// No file of a dropped pack stays in the bucket.
	known := make(map[string]bool)
	for id := range h.f.packs {
		known[id] = true
	}
	for _, id := range h.backend.adoptOrder {
		known[id] = true
	}
	err = h.f.client.ListObjects(ctx, s3test.BucketName, block_store_s3.PackObjectKey(prefix, ""), func(key string, _ int64) error {
		if !known[strings.TrimPrefix(key, block_store_s3.PackObjectKey(prefix, ""))] {
			return errors.Errorf("stray file %s", key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A repeat writes nothing.
	if again, err := Compact(ctx, copts); err != nil || again.Packs != 0 || again.Victims != 0 || again.Resumed != 0 {
		t.Fatalf("second compaction = %+v, %v", again, err)
	}

	// A new pack the sweep holds places nothing, so no victim is removable
	// while every new pack has a record.
	var held []*PackRecord
	for _, id := range h.backend.adoptOrder {
		held = append(held, &PackRecord{ID: id, Stage: StageQuarantine})
	}
	if err := writePackRecords(ctx, h.store, held, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := removablePacks(ctx, h.store, low); err != nil || slices.Contains(ok, true) {
		t.Fatalf("victims removable while the new packs are held = %v, %v", ok, err)
	}
	if err := writePackRecords(ctx, h.store, nil, h.backend.adoptOrder); err != nil {
		t.Fatal(err)
	}

	// The quarantine now takes every victim.
	if res, err := Quarantine(ctx, h.opts); err != nil || res.Unplaced != 0 || res.Quarantined != uint64(len(low)) { //nolint:gosec // test counts.
		t.Fatalf("quarantine after compaction = %+v, %v, want %d quarantined", res, err, len(low))
	}
	records := recordsByID(t, h.store)
	for _, id := range low {
		if _, ok := records[id]; !ok {
			t.Fatalf("victim %s has no record", id)
		}
	}

	// A dead leaf of a victim relinked after R lifts its victims at the
	// recheck.
	var relinked *fixtureBlock
	for _, id := range low {
		for _, b := range h.f.packs[id] {
			if relinked == nil && len(b.refs) == 0 && !expect.live[b.ref.MarshalString()] {
				relinked = b
			}
		}
	}
	if relinked == nil {
		t.Fatal("no victim holds a dead leaf")
	}
	h.log.Put(relinked.ref.GetHash())
	var lifted []string
	for _, id := range low {
		if slices.Contains(h.f.packs[id], relinked) {
			lifted = append(lifted, id)
		}
	}
	h.backend.now = h.backend.now.Add(2 * time.Hour)
	h.backend.ops = &OpenOperations{}
	h.recheck(t)
	records = recordsByID(t, h.store)
	for _, id := range lifted {
		if _, ok := records[id]; ok || h.backend.hold(id).State != PackRegistered {
			t.Fatalf("relinked victim %s still held", id)
		}
	}

	// The next compaction writes the relinked block alone, read from one of
	// the lifted packs that hold it, and the quarantine takes them all again.
	res, err = Compact(ctx, copts)
	if err != nil || res.Packs != 1 || res.Blocks != 1 || res.Victims != 1 {
		t.Fatalf("compaction after the relink = %+v, %v", res, err)
	}
	last := h.readPack(t, prefix, h.backend.adoptOrder[len(h.backend.adoptOrder)-1])
	if len(last) != 1 || !bytes.Equal(last[0].block.Data, relinked.data) {
		t.Fatal("the last pack does not hold the relinked block")
	}
	if res, err := Quarantine(ctx, h.opts); err != nil || res.Unplaced != 0 || res.Quarantined < uint64(len(lifted)) { //nolint:gosec // test counts.
		t.Fatalf("quarantine after the relink = %+v, %v", res, err)
	}
	records = recordsByID(t, h.store)
	for _, id := range lifted {
		if _, ok := records[id]; !ok {
			t.Fatalf("relinked victim %s not held again", id)
		}
	}
}
