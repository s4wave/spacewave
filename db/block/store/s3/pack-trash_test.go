//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// putDatas writes the blocks of datas as one pack and returns their refs.
func putDatas(ctx context.Context, t *testing.T, s *PackStore, datas ...string) []*block.BlockRef {
	t.Helper()
	var batch []*block.PutBatchEntry
	var refs []*block.BlockRef
	for _, d := range datas {
		ref, err := block.BuildBlockRef([]byte(d), nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: []byte(d), RefsKnown: true})
		refs = append(refs, ref)
	}
	if _, err := s.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	return refs
}

// trashMove moves the entry of pack id, then its file, to the trash, as the
// command does.
func trashMove(ctx context.Context, t *testing.T, client ObjectStore, prefix, id string) {
	t.Helper()
	moveObjects(ctx, t, client, [2]string{EntryObjectKey(prefix, id), TrashEntryKey(prefix, id)}, [2]string{PackObjectKey(prefix, id), TrashPackKey(prefix, id)})
}

// moveObjects moves each object from its first key to its second, in order.
func moveObjects(ctx context.Context, t *testing.T, client ObjectStore, moves ...[2]string) {
	t.Helper()
	for _, keys := range moves {
		body, err := client.GetObject(ctx, "bucket", keys[0])
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := client.PutObject(ctx, "bucket", keys[1], data, ""); err != nil {
			t.Fatal(err)
		}
		if err := client.DeleteObject(ctx, "bucket", keys[0]); err != nil {
			t.Fatal(err)
		}
	}
}

// rowNum returns the pack number the h row of ref names, and false with no
// row.
func rowNum(t *testing.T, kv kvtx.Store, ref *block.BlockRef) (uint32, bool) {
	t.Helper()
	tx, err := kv.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	value, ok, err := tx.Get(t.Context(), hashRowKey(hashKey(ref)))
	if err != nil {
		t.Fatal(err)
	}
	row, parsed := parseHashValue(value)
	return row.num, ok && parsed
}

// TestTrashRemove trashes and removes a pack over each index volume: the moves
// the states refuse, the removal that waits for the trash, the read that
// finds a trashing pack in the trash and keeps it, and the h row repair that
// points a shared key at a registered or a held holder and deletes the rows
// no other pack holds, across a restart.
func TestTrashRemove(t *testing.T) {
	eachIndexVolume(t, testTrashRemove)
}

func testTrashRemove(t *testing.T, kind string) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := newIndexKV(t, kind)
	s := openSweepStore(client, kv)
	defer func() { s.Close() }()

	// Write a registered holder of x, a holder of y to quarantine, a pack to
	// remove holding both and two keys of its own, and a pack a read keeps.
	h1 := putDatas(ctx, t, s, "x", "h1")
	h2 := putDatas(ctx, t, s, "y", "h2")
	r := putDatas(ctx, t, s, "x", "y", "r0", "r1")
	d := putDatas(ctx, t, s, "d0", "d1")
	packH1, packH2, packR, packD := packOf(t, s, h1[1]), packOf(t, s, h2[1]), packOf(t, s, r[2]), packOf(t, s, d[0])
	if packOf(t, s, r[0]) != packR || packOf(t, s, r[1]) != packR {
		t.Fatal("the last pack written does not name x and y")
	}

	// Only a quarantined pack begins trashing, and a trashing pack is
	// neither quarantined nor lifted.
	if err := s.BeginTrash(ctx, []string{packR}); err == nil {
		t.Fatal("began trashing a registered pack")
	}
	if err := s.Quarantine(ctx, []string{packH2, packR, packD}); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTrash(ctx, []string{packR, "nope"}); err == nil {
		t.Fatal("began trashing a pack the index lacks")
	}
	for range 2 {
		if err := s.BeginTrash(ctx, []string{packR, packD}); err != nil {
			t.Fatal(err)
		}
	}
	if heldStateOf(t, s, packR) != PackTrashing || heldStateOf(t, s, packH2) != PackQuarantined {
		t.Fatal("trashing moved the wrong packs")
	}
	if s.Quarantine(ctx, []string{packR}) == nil || s.Lift(ctx, []string{packR}) == nil {
		t.Fatal("quarantined or lifted a trashing pack")
	}

	// The removal waits for the entry and file to reach the trash, and a
	// quarantined pack is not removed.
	keys := []KeyHolders{
		{Key: hashKey(r[0]), Others: []string{packR, packH1}},
		{Key: hashKey(r[1]), Others: []string{packH2}},
		{Key: hashKey(r[2])},
		{Key: hashKey(r[3]), Others: []string{"ghost"}},
	}
	if err := s.Remove(ctx, []string{packR}, keys); err == nil {
		t.Fatal("removed a pack still in the store")
	}
	if err := s.Remove(ctx, []string{packH2}, nil); err == nil {
		t.Fatal("removed a quarantined pack")
	}
	if heldStateOf(t, s, packR) != PackTrashing {
		t.Fatal("a refused removal moved the pack")
	}

	// A read of a trashing pack in the trash fails, records the miss, and
	// keeps the pack.
	trashMove(ctx, t, client, "p/", packR)
	trashMove(ctx, t, client, "p/", packD)
	if _, _, err := s.GetBlock(ctx, d[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read of a trashed block = %v, want not found", err)
	}
	hold, _, err := s.index.readHold(ctx, packD)
	if err != nil || hold.state != PackTrashing || hold.event == 0 || !bytes.Equal(hold.key, hashKey(d[0])) {
		t.Fatalf("miss record = %+v, %v", hold, err)
	}
	if err := s.Remove(ctx, []string{packD}, nil); err == nil {
		t.Fatal("removed a pack a read missed")
	}

	// A repair alone, as for the first keys of a pack too large for one
	// call, keeps the pack and the row of a key no other pack holds, so a
	// read still falls back to the pack.
	s.index.mtx.RLock()
	numR := s.index.rows[packR].num
	s.index.mtx.RUnlock()
	if err := s.RepairRemoved(ctx, []string{packR}, keys[:3]); err != nil {
		t.Fatal(err)
	}
	if heldStateOf(t, s, packR) != PackTrashing {
		t.Fatal("a repair alone dropped the pack")
	}
	if num, ok := rowNum(t, kv, r[2]); !ok || num != numR {
		t.Fatalf("a repair alone moved the row of a key only the pack holds to %d, %v", num, ok)
	}

	// A removal killed after its repair is finished by the next, and a
	// repeat changes nothing. The row of the key the removal left out still
	// names the dropped pack, until a call naming it with the key prunes it.
	if err := s.index.repairRows(ctx, []string{packR}, keys[:3], true); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.Remove(ctx, []string{packR}, keys[:3]); err != nil {
			t.Fatal(err)
		}
	}
	if num, ok := rowNum(t, kv, r[3]); !ok || num != numR {
		t.Fatalf("the left out key's row = %d, %v; want the dropped pack's %d", num, ok, numR)
	}
	if err := s.Remove(ctx, []string{packR}, keys[3:]); err != nil {
		t.Fatal(err)
	}
	checkRemoved := func(s *PackStore) {
		t.Helper()
		if err := s.loadEntries(ctx); err != nil {
			t.Fatal(err)
		}
		s.index.mtx.RLock()
		numH1, numH2 := s.index.rows[packH1].num, s.index.rows[packH2].num
		s.index.mtx.RUnlock()
		if numH1 == numH2 {
			t.Fatal("the holders share a number")
		}
		if num, ok := rowNum(t, kv, r[0]); !ok || num != numH1 {
			t.Fatalf("x's row names %d, %v; want pack h1's %d", num, ok, numH1)
		}
		if num, ok := rowNum(t, kv, r[1]); !ok || num != numH2 {
			t.Fatalf("y's row names %d, %v; want pack h2's %d", num, ok, numH2)
		}
		for _, ref := range r[2:] {
			if _, ok := rowNum(t, kv, ref); ok {
				t.Fatal("a row remains for a key only the removed pack held")
			}
		}
		if _, ok := s.index.heldState(packR); ok {
			t.Fatal("the table still holds the removed pack")
		}
		if data, found, err := s.GetBlock(ctx, r[2]); err != nil || found {
			t.Fatalf("read of a removed block = %q, %v, %v", data, found, err)
		}
		if data, found, err := s.GetBlock(ctx, r[0]); err != nil || !found || string(data) != "x" {
			t.Fatalf("read of x = %q, %v, %v", data, found, err)
		}
	}
	checkRemoved(s)
	holds, err := s.Holds(ctx, []string{packR, packD, packH1})
	if err != nil || !holds[0].Absent || holds[1].Absent || holds[1].State != PackTrashing || holds[2].Absent || holds[2].State != PackRegistered {
		t.Fatalf("holds after the removal = %+v, %v", holds, err)
	}

	// The read of y falls back to its quarantined holder and lifts it.
	if data, found, err := s.GetBlock(ctx, r[1]); err != nil || !found || string(data) != "y" {
		t.Fatalf("read of y = %q, %v, %v", data, found, err)
	}
	if heldStateOf(t, s, packH2) != PackRegistered {
		t.Fatal("the read of y did not lift its holder")
	}

	// A restart finds the same rows and no removed pack.
	s.Close()
	s = openSweepStore(client, kv)
	checkRemoved(s)
	packs, err := s.Packs(ctx)
	if err != nil || len(packs) != 3 {
		t.Fatalf("Packs after the restart = %d, %v; want h1, h2 and d", len(packs), err)
	}
	if heldStateOf(t, s, packD) != PackTrashing {
		t.Fatal("the restart lost the kept trashing pack")
	}

	// The restore refuses a removed or a quarantined pack, and a missed pack
	// until its file and its entry are both back from the trash.
	if err := s.Restore(ctx, []string{packR}); err == nil {
		t.Fatal("restored a removed pack")
	}
	if err := s.Quarantine(ctx, []string{packH1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(ctx, []string{packH1}); err == nil {
		t.Fatal("restored a quarantined pack")
	}
	if err := s.Lift(ctx, []string{packH1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(ctx, []string{packD}); err == nil {
		t.Fatal("restored a pack in the trash")
	}
	moveObjects(ctx, t, client, [2]string{TrashPackKey("p/", packD), PackObjectKey("p/", packD)})
	if err := s.Restore(ctx, []string{packD}); err == nil {
		t.Fatal("restored a pack whose entry is in the trash")
	}
	if heldStateOf(t, s, packD) != PackTrashing {
		t.Fatal("a refused restore moved the pack")
	}

	// Once both are back, the restore registers the pack, keeping its miss,
	// and a repeat, or one naming a registered pack, changes nothing. The
	// missed block reads again, across a restart.
	moveObjects(ctx, t, client, [2]string{TrashEntryKey("p/", packD), EntryObjectKey("p/", packD)})
	for range 2 {
		if err := s.Restore(ctx, []string{packD, packH1}); err != nil {
			t.Fatal(err)
		}
	}
	checkRestored := func(s *PackStore) {
		t.Helper()
		if err := s.loadEntries(ctx); err != nil {
			t.Fatal(err)
		}
		if heldStateOf(t, s, packD) != PackRegistered {
			t.Fatal("the restore left the pack held")
		}
		hold, _, err := s.index.readHold(ctx, packD)
		if err != nil || hold.state != PackRegistered || hold.event == 0 || !bytes.Equal(hold.key, hashKey(d[0])) {
			t.Fatalf("hold after the restore = %+v, %v", hold, err)
		}
		for _, ref := range d {
			if _, found, err := s.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("read of a restored block = %v, %v", found, err)
			}
		}
		if located, err := s.LocatePacks(ctx, d[:1]); err != nil || located[0] != packD {
			t.Fatalf("locate of a restored block = %v, %v", located, err)
		}
	}
	checkRestored(s)
	s.Close()
	s = openSweepStore(client, kv)
	checkRestored(s)
}
