//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
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
	if !s.Registered(packR) || s.Registered("nope") {
		t.Fatal("Registered misreports a written pack or one the table lacks")
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
	if s.Registered(packR) || s.Registered(packH2) || !s.Registered(packH1) {
		t.Fatal("Registered reports a trashing or quarantined pack, or misses a registered one")
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
	if _, _, err := s.GetBlock(ctx, d[1]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second read of a trashed block = %v, want not found", err)
	}
	if again, _, err := s.index.readHold(ctx, packD); err != nil || again.event != hold.event || !bytes.Equal(again.key, hold.key) {
		t.Fatalf("a second miss rewrote the record: %+v, %v; first %+v", again, err, hold)
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
	if s.Registered(packR) {
		t.Fatal("Registered reports a removed pack")
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
	if !s.Registered(packD) {
		t.Fatal("Registered misses a restored pack, which carries an event")
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

	// The removed pack, moved back, is adopted with the keys its index lists
	// under a new number, and each of its blocks reads from it, across a
	// restart.
	moveObjects(ctx, t, client, [2]string{TrashPackKey("p/", packR), PackObjectKey("p/", packR)}, [2]string{TrashEntryKey("p/", packR), EntryObjectKey("p/", packR)})
	body, err := client.GetObject(ctx, "bucket", EntryObjectKey("p/", packR))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	entry := &packfile.PackfileEntry{}
	if err != nil || entry.UnmarshalVT(data) != nil {
		t.Fatalf("read the removed pack's entry: %v", err)
	}
	keysR := make([][]byte, len(r))
	for i, ref := range r {
		keysR[i] = hashKey(ref)
	}
	if err := s.AdoptPack(ctx, entry, keysR); err != nil {
		t.Fatal(err)
	}
	checkAdopted := func(s *PackStore) {
		t.Helper()
		if err := s.loadEntries(ctx); err != nil {
			t.Fatal(err)
		}
		if heldStateOf(t, s, packR) != PackRegistered {
			t.Fatal("the adopted pack is not registered")
		}
		s.index.mtx.RLock()
		num := s.index.rows[packR].num
		s.index.mtx.RUnlock()
		if num == numR {
			t.Fatal("the adopted pack took its removed number again")
		}
		for i, ref := range r {
			if data, found, err := s.GetBlock(ctx, ref); err != nil || !found || string(data) != []string{"x", "y", "r0", "r1"}[i] {
				t.Fatalf("read of the adopted pack's block %d = %q, %v, %v", i, data, found, err)
			}
		}
		if located, err := s.LocatePacks(ctx, r[2:]); err != nil || located[0] != packR || located[1] != packR {
			t.Fatalf("locate of the adopted pack's own blocks = %v, %v", located, err)
		}
	}
	checkAdopted(s)
	s.Close()
	s = openSweepStore(client, kv)
	checkAdopted(s)

	// A restore named on the adopted pack records it, and a quarantine
	// leaves both restored packs registered until forget clears them.
	if err := s.Restore(ctx, []string{packR}); err != nil {
		t.Fatal(err)
	}
	if err := s.Quarantine(ctx, []string{packD, packR}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{packD, packR} {
		if hold, _, err := s.index.readHold(ctx, id); err != nil || heldStateOf(t, s, id) != PackRegistered || hold.event == 0 {
			t.Fatalf("restored pack %s after a quarantine: %+v, %v", id, hold, err)
		}
	}
	if err := s.Forget(ctx, []string{packD, packR}); err != nil {
		t.Fatal(err)
	}
	if err := s.Quarantine(ctx, []string{packD, packR}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{packD, packR} {
		if heldStateOf(t, s, id) != PackQuarantined {
			t.Fatalf("forgotten pack %s was not quarantined", id)
		}
	}
}

// trashGate is an ObjectStore whose reads of trash keys wait for gate to
// close, signaling entered first, so a test can hold a removal in its bucket
// checks.
type trashGate struct {
	ObjectStore
	entered chan struct{}
	gate    chan struct{}
}

// GetObjectRange waits on the gate for a trash key, then reads the range.
func (g *trashGate) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Hold a read of the trash until the gate opens.
	if strings.HasPrefix(key, "p/"+trashDir) {
		select {
		case g.entered <- struct{}{}:
		default:
		}
		select {
		case <-g.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

// TestRemoveReadsOutsideHold holds a removal in its bucket checks of the
// trash and reads meanwhile: a read that lifts a quarantined pack completes
// without waiting for the removal, and a read that misses the trashing pack
// records the miss, which the removal sees when it takes the lock and refuses.
func TestRemoveReadsOutsideHold(t *testing.T) {
	// Write a pack to quarantine and a pack to remove, and move the second
	// to the trash.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	gated := &trashGate{ObjectStore: client, entered: make(chan struct{}, 1), gate: make(chan struct{})}
	s := openSweepStore(gated, newIndexKV(t, "memory"))
	defer s.Close()
	q := putDatas(ctx, t, s, "q0", "q1")
	r := putDatas(ctx, t, s, "r0", "r1")
	packQ, packR := packOf(t, s, q[0]), packOf(t, s, r[0])
	if err := s.Quarantine(ctx, []string{packQ, packR}); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTrash(ctx, []string{packR}); err != nil {
		t.Fatal(err)
	}
	trashMove(ctx, t, client, "p/", packR)

	// Start the removal and wait for it to reach the trash.
	done := make(chan error, 1)
	go func() {
		done <- s.Remove(ctx, []string{packR}, []KeyHolders{{Key: hashKey(r[0])}, {Key: hashKey(r[1])}})
	}()
	select {
	case <-gated.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the removal never read the trash")
	}

	// A read of the quarantined pack completes while the removal waits.
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if data, found, err := s.GetBlock(readCtx, q[0]); err != nil || !found || string(data) != "q0" {
		t.Fatalf("read of a quarantined block during a removal = %q, %v, %v", data, found, err)
	}
	if heldStateOf(t, s, packQ) != PackRegistered {
		t.Fatal("the read did not lift the quarantined pack")
	}

	// A read that misses the trashing pack records the miss, and the removal
	// refuses once the gate opens.
	if _, _, err := s.GetBlock(readCtx, r[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read of a trashed block = %v, want not found", err)
	}
	close(gated.gate)
	if err := <-done; err == nil {
		t.Fatal("removed a pack a read missed during the removal")
	}
	if heldStateOf(t, s, packR) != PackTrashing {
		t.Fatal("the refused removal moved the pack")
	}
}
