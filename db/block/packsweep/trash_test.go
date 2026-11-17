//go:build !tinygo

package packsweep

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/packfile"
)

// failMover fails the first move of one key, after moving every key before
// it.
type failMover struct {
	block_store_s3.ObjectMover
	// from is the key whose move fails.
	from string
	// failed is set once it failed.
	failed atomic.Bool
}

// MoveObject fails the first move of from.
func (m *failMover) MoveObject(ctx context.Context, bucket, from, to string) error {
	if from == m.from && m.failed.CompareAndSwap(false, true) {
		return errors.New("killed")
	}
	return m.ObjectMover.MoveObject(ctx, bucket, from, to)
}

// passAll quarantines the dead packs and passes them, and returns them.
func (h *holdFixture) passAll(t *testing.T) []string {
	t.Helper()
	if _, err := Quarantine(t.Context(), h.opts); err != nil {
		t.Fatal(err)
	}
	h.backend.now = h.backend.now.Add(2 * time.Hour)
	h.backend.ops = &OpenOperations{}
	if got := h.recheck(t); got.Passed != uint64(len(h.dead)) { //nolint:gosec // test counts.
		t.Fatalf("recheck = %+v, want %d passed", got, len(h.dead))
	}
	return h.dead
}

// objectAt reports whether key reads in the fixture's bucket.
func (h *holdFixture) objectAt(t *testing.T, key string) bool {
	t.Helper()
	_, err := h.f.client.GetObjectRange(t.Context(), s3test.BucketName, key, 0, 1)
	if err != nil && !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatal(err)
	}
	return err == nil
}

// TestTrashRemove carries the passed packs through the trash and the
// removal: a lifted pack leaves the cycle, a trash killed after the backend's
// call and one killed mid-move resume, a bucket that cannot list versions
// leaves every pack at its key, a pack a read missed after its move stays for
// restore, and the removal, killed after the backend's first call, sends each
// removed pack's keys with their other holders in bounded calls.
func TestTrashRemove(t *testing.T) {
	ctx := t.Context()
	h := newHoldFixture(t)
	passed := h.passAll(t)
	const prefix = "v2/"
	opts := &TrashOptions{CoverOptions: *h.opts, Mover: h.f.client, Bucket: s3test.BucketName, Prefix: prefix, Moves: 1}

	// A read lifts one passed pack, and a trash killed after the backend's
	// call leaves the rest trashing.
	lifted := passed[0]
	h.backend.holds[lifted] = PackHold{ID: lifted, State: PackRegistered, Since: h.backend.now}
	h.backend.failBeginTrash = true
	if _, err := Trash(ctx, opts); err == nil {
		t.Fatal("killed trash returned no error")
	}
	records := recordsByID(t, h.store)
	if _, ok := records[lifted]; ok {
		t.Fatal("lifted pack kept its record")
	}
	for _, id := range passed[1:] {
		if records[id].Stage != StageTrash || h.backend.hold(id).State != PackTrashing {
			t.Fatalf("pack %s after the kill: record %+v, state %v", id, records[id], h.backend.hold(id))
		}
	}

	// A bucket that cannot list versions fails the move and keeps the pack
	// where it was, rather than hide it with a plain delete.
	h.f.bucket.RefuseVersions(true)
	if _, err := Trash(ctx, opts); err == nil {
		t.Fatal("a trash where versions cannot be listed returned no error")
	}
	h.f.bucket.RefuseVersions(false)
	for _, id := range passed[1:] {
		if !h.objectAt(t, block_store_s3.EntryObjectKey(prefix, id)) || !h.objectAt(t, block_store_s3.PackObjectKey(prefix, id)) {
			t.Fatalf("pack %s left its key where versions cannot be listed", id)
		}
	}

	// A move killed between a pack's entry and its file resumes.
	stuck := passed[2]
	opts.Mover = &failMover{ObjectMover: h.f.client, from: block_store_s3.PackObjectKey(prefix, stuck)}
	if _, err := Trash(ctx, opts); err == nil {
		t.Fatal("killed move returned no error")
	}
	if records := recordsByID(t, h.store); records[stuck].Stage != StageTrash || h.objectAt(t, block_store_s3.EntryObjectKey(prefix, stuck)) {
		t.Fatalf("stuck pack after the kill: %+v", records[stuck])
	}
	res, err := Trash(ctx, opts)
	if err != nil || res.Moved == 0 || res.Lifted != 0 {
		t.Fatalf("resumed trash = %+v, %v", res, err)
	}
	for _, id := range passed[1:] {
		live := h.objectAt(t, block_store_s3.EntryObjectKey(prefix, id)) || h.objectAt(t, block_store_s3.PackObjectKey(prefix, id))
		trashed := h.objectAt(t, block_store_s3.TrashEntryKey(prefix, id)) && h.objectAt(t, block_store_s3.TrashPackKey(prefix, id))
		if rec := recordsByID(t, h.store)[id]; live || !trashed || rec.Stage != StageMoved {
			t.Fatalf("pack %s after the trash: live %v, trashed %v, record %+v", id, live, trashed, rec)
		}
	}
	if again, err := Trash(ctx, opts); err != nil || again.Moved != 0 || h.backend.calls["begin-trash"] != 1 {
		t.Fatalf("second trash = %+v, %v, %d calls", again, err, h.backend.calls["begin-trash"])
	}

	// A read misses one moved pack, and small bounds split the removal into
	// rounds and calls; the first call is killed after the backend's change.
	missed := passed[3]
	h.backend.holds[missed] = PackHold{ID: missed, State: PackTrashing, Since: h.backend.now, Event: h.backend.now}
	defer func(round, weight int) { removeRoundKeys, removeCallWeight = round, weight }(removeRoundKeys, removeCallWeight)
	removeRoundKeys, removeCallWeight = 15, 7
	at := time.Unix(2_000_000, 0).UTC()
	ropts := &RemoveOptions{CoverOptions: *h.opts, Now: func() time.Time { return at }}
	h.backend.failRemove = true
	if _, err := Remove(ctx, ropts); err == nil {
		t.Fatal("killed removal returned no error")
	}
	h.backend.failDrop = true
	if _, err := Remove(ctx, ropts); err == nil {
		t.Fatal("removal killed at its drop returned no error")
	}
	killed := h.backend.removals[len(h.backend.removals)-1]
	if killed.more || !h.backend.removed[killed.ids[0]] {
		t.Fatalf("the killed call = %+v", killed)
	}
	before := len(h.backend.removals)
	got, err := Remove(ctx, ropts)
	removed := len(passed) - 2
	if err != nil || got.Missed != 1 || got.Rounds < 2 {
		t.Fatalf("removal = %+v, %v", got, err)
	}
	records = recordsByID(t, h.store)
	for _, id := range passed[1:] {
		want := StageRemoved
		if id == missed {
			want = StageMoved
		}
		if p := records[id]; p.Stage != want || (want == StageRemoved && !p.Removed.Equal(at)) {
			t.Fatalf("pack %s after the removal: %+v", id, p)
		}
	}

	// Every removed pack's keys went out, each with its other holders not
	// in the trash, in calls within the bound.
	trashed := make(map[string]bool)
	for _, id := range passed[1:] {
		trashed[id] = true
	}
	sent := make(map[string][]string)
	ids := make(map[string]bool)
	dropped := make(map[string]bool)
	repairs := 0
	for _, call := range h.backend.removals {
		if call.more {
			repairs++
			if !slices.IsSortedFunc(call.keys, func(a, b KeyHolders) int { return lone(a) - lone(b) }) {
				t.Fatal("a repair sent a key no other pack holds before one with holders")
			}
		}
		for _, k := range call.keys {
			if !call.more && len(k.Others) == 0 {
				dropped[string(k.Key)] = true
			}
		}
		if keysWeight(call.keys) > removeCallWeight || (call.more && len(call.ids) != 1) {
			t.Fatalf("removal call over the bound: %d ids, weight %d, more %v", len(call.ids), keysWeight(call.keys), call.more)
		}
		for _, id := range call.ids {
			ids[id] = true
		}
		for _, k := range call.keys {
			sent[string(k.Key)] = k.Others
		}
	}
	resent := false
	for _, call := range h.backend.removals[before:] {
		resent = resent || slices.Contains(call.ids, killed.ids[0])
	}
	if len(ids) != removed || ids[missed] || ids[lifted] || repairs == 0 || !resent {
		t.Fatalf("removal calls named %d packs, want %d, with %d repairs", len(ids), removed, repairs)
	}
	for id := range ids {
		for _, b := range h.f.packs[id] {
			var want []string
			for other, blocks := range h.f.packs {
				if !trashed[other] && slices.Contains(blocks, b) {
					want = append(want, other)
				}
			}
			slices.Sort(want)
			tail := string(RefTail(b.ref))
			others, ok := sent[tail]
			if !ok || strings.Join(others, ",") != strings.Join(want, ",") {
				t.Fatalf("pack %s key holders = %v, %v, want %v", id, others, ok, want)
			}
			if len(want) == 0 && !dropped[tail] {
				t.Fatalf("pack %s key %x no other pack holds went in no call that deletes its row", id, tail)
			}
		}
	}

	// A second removal calls the backend for nothing.
	calls := h.backend.calls["remove"]
	if again, err := Remove(ctx, ropts); err != nil || again.Removed != 0 || again.Missed != 1 || h.backend.calls["remove"] != calls {
		t.Fatalf("second removal = %+v, %v", again, err)
	}

	// A purge before the wait deletes nothing. One past it deletes every
	// version of each removed pack's objects, including an older version the
	// bucket kept, and leaves the moved pack a read missed in the trash.
	var kept string
	for _, id := range passed[1:] {
		if id != missed {
			kept = id
			break
		}
	}
	keptKey := block_store_s3.TrashPackKey(prefix, kept)
	if err := h.f.client.PutObject(ctx, s3test.BucketName, keptKey, []byte("an older version"), ""); err != nil {
		t.Fatal(err)
	}
	popts := &PurgeOptions{Store: h.store, Purger: h.f.client, Bucket: s3test.BucketName, Prefix: prefix, Now: func() time.Time { return at.Add(time.Hour) }}
	if got, err := Purge(ctx, popts); err != nil || got.Purged != 0 || got.Versions != 0 || got.Waiting != uint64(removed) { //nolint:gosec // test counts.
		t.Fatalf("purge before the wait = %+v, %v", got, err)
	}
	// Past the wait every version goes: two keys a pack, the older version
	// written above, and the second copy of the one key the refused move
	// copied before its delete failed.
	popts.Now = func() time.Time { return at.Add(DefaultPurgeAfter + time.Hour) }
	purged, err := Purge(ctx, popts)
	versions := 2*removed + 2
	if err != nil || purged.Purged != uint64(removed) || purged.Versions != uint64(versions) || purged.Waiting != 0 { //nolint:gosec // test counts.
		t.Fatalf("purge = %+v, %v, want %d packs and %d versions", purged, err, removed, versions)
	}
	records = recordsByID(t, h.store)
	for _, id := range passed[1:] {
		if id == missed {
			if records[id].Stage != StageMoved || !h.objectAt(t, block_store_s3.TrashPackKey(prefix, id)) {
				t.Fatalf("the missed pack %s after the purge: %+v", id, records[id])
			}
			continue
		}
		if records[id].Stage != StagePurged {
			t.Fatalf("pack %s after the purge: %+v", id, records[id])
		}
		for _, key := range purgeKeys(prefix, id) {
			versions, err := h.f.client.ObjectVersions(ctx, s3test.BucketName, key)
			if err != nil || len(versions) != 0 {
				t.Fatalf("%s keeps versions %v, %v", key, versions, err)
			}
		}
	}

	// A second purge deletes nothing.
	if again, err := Purge(ctx, popts); err != nil || again.Purged != 0 || again.Versions != 0 {
		t.Fatalf("second purge = %+v, %v", again, err)
	}
}

// TestTrashRecheck shows a pass is not final: a block a root reaches after a
// pack's recheck keeps the pack. The trash lifts a passed pack holding it, and
// leaves a trashing one for restore; the removal leaves a moved one for
// restore; the restore brings both back.
func TestTrashRecheck(t *testing.T) {
	ctx := t.Context()
	h := newHoldFixture(t)
	passed := h.passAll(t)
	const prefix = "v2/"
	ropts := &RestoreOptions{TrashOptions: TrashOptions{CoverOptions: *h.opts, Mover: h.f.client, Bucket: s3test.BucketName, Prefix: prefix, Moves: 1}}
	opts := &ropts.TrashOptions
	within := func(ids, set []string) []string {
		return slices.DeleteFunc(ids, func(id string) bool { return !slices.Contains(set, id) })
	}
	without := func(set, ids []string) []string {
		return slices.DeleteFunc(slices.Clone(set), func(id string) bool { return slices.Contains(ids, id) })
	}

	// A link after the pass lifts the passed packs holding its block; a
	// trash killed after the backend's call leaves the rest trashing.
	lifted := h.relink(t, passed...)
	h.backend.failBeginTrash = true
	if _, err := Trash(ctx, opts); err == nil {
		t.Fatal("killed trash returned no error")
	}
	trashing := without(passed, lifted)
	for _, id := range lifted {
		if _, ok := recordsByID(t, h.store)[id]; ok || h.backend.hold(id).State != PackRegistered {
			t.Fatalf("relinked passed pack %s was not lifted: %v", id, h.backend.hold(id))
		}
	}

	// A link to a trashing pack leaves it unmoved and marked for restore.
	liveTrash := within(h.relink(t, trashing...), trashing)
	res, err := Trash(ctx, opts)
	if err != nil || res.Live != uint64(len(liveTrash)) || res.Moved != uint64(len(trashing)-len(liveTrash)) { //nolint:gosec // test counts.
		t.Fatalf("trash = %+v, %v, want %d live", res, err, len(liveTrash))
	}
	for _, id := range liveTrash {
		if rec := recordsByID(t, h.store)[id]; rec.Stage != StageTrash || rec.Wait != MarkLive || !h.objectAt(t, block_store_s3.PackObjectKey(prefix, id)) {
			t.Fatalf("relinked trashing pack %s: %+v", id, rec)
		}
	}

	// A link to a moved pack keeps the removal from removing it.
	moved := without(trashing, liveTrash)
	liveMoved := within(h.relink(t, moved...), moved)
	got, err := Remove(ctx, &RemoveOptions{CoverOptions: *h.opts})
	if err != nil || got.Live != uint64(len(liveMoved)) || got.Removed != uint64(len(moved)-len(liveMoved)) { //nolint:gosec // test counts.
		t.Fatalf("removal = %+v, %v, want %d live", got, err, len(liveMoved))
	}
	for _, id := range liveMoved {
		if h.backend.removed[id] {
			t.Fatalf("the removal removed relinked pack %s", id)
		}
	}

	// The restore brings back every pack marked live, unnamed.
	back, err := Restore(ctx, ropts)
	if err != nil || back.Restored != uint64(len(liveTrash)+len(liveMoved)) { //nolint:gosec // test counts.
		t.Fatalf("restore = %+v, %v", back, err)
	}
	for _, id := range append(liveTrash, liveMoved...) {
		if h.backend.hold(id).State != PackRegistered || !h.objectAt(t, block_store_s3.PackObjectKey(prefix, id)) {
			t.Fatalf("relinked pack %s is not back", id)
		}
	}
}

// TestRestore returns the trashing packs reads missed to the store, at each
// point the trash leaves them: before any move, between the entry and the
// file, and after both. A restore killed mid-move and one killed after the
// backend's call resume, and the packs no read missed stay in the trash
// until one is named, and a removed pack named is adopted with its keys.
func TestRestore(t *testing.T) {
	ctx := t.Context()
	h := newHoldFixture(t)
	passed := h.passAll(t)
	const prefix = "v2/"
	ropts := &RestoreOptions{TrashOptions: TrashOptions{CoverOptions: *h.opts, Mover: h.f.client, Bucket: s3test.BucketName, Prefix: prefix, Moves: 1}}
	opts := &ropts.TrashOptions
	miss := func(id string) {
		h.backend.holds[id] = PackHold{ID: id, State: PackTrashing, Since: h.backend.now, Event: h.backend.now}
	}

	// A read misses one pack before any move, one between its entry and its
	// file, and one after both.
	h.backend.failBeginTrash = true
	if _, err := Trash(ctx, opts); err == nil {
		t.Fatal("killed trash returned no error")
	}
	miss(passed[0])
	opts.Mover = &failMover{ObjectMover: h.f.client, from: block_store_s3.PackObjectKey(prefix, passed[1])}
	if _, err := Trash(ctx, opts); err == nil {
		t.Fatal("killed move returned no error")
	}
	miss(passed[1])
	opts.Mover = h.f.client
	if res, err := Trash(ctx, opts); err != nil || res.Missed != 2 {
		t.Fatalf("trash = %+v, %v", res, err)
	}
	miss(passed[2])
	records := recordsByID(t, h.store)
	if records[passed[0]].Stage != StageTrash || records[passed[1]].Stage != StageTrash || records[passed[2]].Stage != StageMoved {
		t.Fatal("the missed packs are not at the trash's stages")
	}

	// A restore killed between a pack's file and its entry, then one killed
	// after the backend's call, resume.
	opts.Mover = &failMover{ObjectMover: h.f.client, from: block_store_s3.TrashEntryKey(prefix, passed[2])}
	if _, err := Restore(ctx, ropts); err == nil {
		t.Fatal("killed restore move returned no error")
	}
	if !h.objectAt(t, block_store_s3.PackObjectKey(prefix, passed[2])) || !h.objectAt(t, block_store_s3.TrashEntryKey(prefix, passed[2])) {
		t.Fatal("the restore moved the entry back before the file")
	}
	opts.Mover = h.f.client
	h.backend.failRestore = true
	if _, err := Restore(ctx, ropts); err == nil {
		t.Fatal("killed restore returned no error")
	}
	got, err := Restore(ctx, ropts)
	if err != nil || got.Restored+got.Dropped == 0 {
		t.Fatalf("resumed restore = %+v, %v", got, err)
	}

	// Each missed pack is back in the store and registered, with no record;
	// the rest stay moved and trashing.
	records = recordsByID(t, h.store)
	for i, id := range passed {
		live := h.objectAt(t, block_store_s3.EntryObjectKey(prefix, id)) && h.objectAt(t, block_store_s3.PackObjectKey(prefix, id))
		trashed := h.objectAt(t, block_store_s3.TrashEntryKey(prefix, id)) || h.objectAt(t, block_store_s3.TrashPackKey(prefix, id))
		_, recorded := records[id]
		state := h.backend.hold(id).State
		if restored := i < 3; live != restored || trashed == restored || recorded == restored || (state == PackRegistered) != restored {
			t.Fatalf("pack %s after the restore: live %v, trashed %v, recorded %v, state %v", id, live, trashed, recorded, state)
		}
	}

	// A repeat calls the backend for nothing.
	calls := h.backend.calls["restore"]
	if again, err := Restore(ctx, ropts); err != nil || again.Restored != 0 || again.Dropped != 0 || h.backend.calls["restore"] != calls {
		t.Fatalf("second restore = %+v, %v", again, err)
	}

	// A named pack no read missed is restored; one the cycle never trashed,
	// or a lifted one, is refused.
	ropts.Packs = []string{passed[3]}
	if got, err := Restore(ctx, ropts); err != nil || got.Restored != 1 {
		t.Fatalf("named restore = %+v, %v", got, err)
	}
	for _, id := range []string{passed[0], "nope"} {
		ropts.Packs = []string{id}
		if _, err := Restore(ctx, ropts); err == nil {
			t.Fatalf("restored %s, which the cycle has not trashed", id)
		}
	}

	// Remove the rest; a restore naming one moves it back and adopts it with
	// the keys its own index lists, a killed one resumes, and a repeat
	// refuses it, since its record is gone.
	if got, err := Remove(ctx, &RemoveOptions{CoverOptions: *h.opts}); err != nil || got.Removed != uint64(len(passed)-4) { //nolint:gosec // test counts.
		t.Fatalf("removal = %+v, %v", got, err)
	}
	gone := passed[4]
	ropts.Packs = []string{gone}
	ropts.Mover = &failMover{ObjectMover: h.f.client, from: block_store_s3.TrashEntryKey(prefix, gone)}
	if _, err := Restore(ctx, ropts); err == nil || h.backend.calls["adopt"] != 0 {
		t.Fatal("killed adoption returned no error, or adopted")
	}

	// A purge past the wait after the killed adoption purges the other
	// removed packs and leaves this one: its record left StageRemoved before
	// the first move, so the file already back under its original key stays.
	if rec := recordsByID(t, h.store)[gone]; rec.Stage != StageRestoring {
		t.Fatalf("the killed adoption's record = %+v", rec)
	}
	late := func() time.Time { return time.Now().Add(2 * DefaultPurgeAfter) }
	popts := &PurgeOptions{Store: h.store, Purger: h.f.client, Bucket: s3test.BucketName, Prefix: prefix, Now: late}
	if got, err := Purge(ctx, popts); err != nil || got.Purged != uint64(len(passed)-5) { //nolint:gosec // test counts.
		t.Fatalf("purge during the adoption = %+v, %v", got, err)
	}
	if rec := recordsByID(t, h.store)[gone]; rec.Stage != StageRestoring {
		t.Fatalf("the purge changed the killed adoption's record to %+v", rec)
	}
	if !h.objectAt(t, block_store_s3.PackObjectKey(prefix, gone)) || !h.objectAt(t, block_store_s3.TrashEntryKey(prefix, gone)) {
		t.Fatal("the purge deleted the pack the killed adoption began")
	}
	ropts.Mover = h.f.client
	got, err = Restore(ctx, ropts)
	if err != nil || got.Adopted != 1 {
		t.Fatalf("adoption = %+v, %v", got, err)
	}
	var want [][]byte
	for _, b := range h.f.packs[gone] {
		want = append(want, packfile.BlockKey(b.ref.GetHash()))
	}
	slices.SortFunc(want, bytes.Compare)
	keys := slices.Clone(h.backend.adopted[gone])
	slices.SortFunc(keys, bytes.Compare)
	if !slices.EqualFunc(keys, want, bytes.Equal) || !h.objectAt(t, block_store_s3.EntryObjectKey(prefix, gone)) || h.objectAt(t, block_store_s3.TrashPackKey(prefix, gone)) {
		t.Fatalf("adopted keys %d, want %d", len(keys), len(want))
	}
	if _, ok := recordsByID(t, h.store)[gone]; ok {
		t.Fatal("the adopted pack kept its record")
	}
	if _, err := Restore(ctx, ropts); err == nil {
		t.Fatal("adopted a pack twice")
	}
}
