package packsweep

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// fakeBackend is the backend's side of a cycle in memory: pack states as the
// pack store keeps them, a clock, the open operations and a write log.
type fakeBackend struct {
	// log is the write log.
	log *writelog.Log
	// holds are the packs' states by id, registered when absent.
	holds map[string]PackHold
	// now is the backend's clock.
	now time.Time
	// ops is the oldest open operations, nil for none recorded.
	ops *OpenOperations
	// logOff answers the log as off.
	logOff bool
	// failQuarantine fails the next quarantine after applying it, as a run
	// killed before it records the answer.
	failQuarantine bool
	// failBeginTrash and failRemove fail the next such call after applying
	// it.
	failBeginTrash, failRemove bool
	// failDrop fails the next removal that drops packs, after applying it.
	failDrop bool
	// failRestore fails the next restore after applying it.
	failRestore bool
	// failAdopt fails the next adoption after applying it.
	failAdopt bool
	// removed are the packs a removal dropped.
	removed map[string]bool
	// adopted are the keys each adopted pack carried.
	adopted map[string][][]byte
	// adoptOrder are the packs adopted, in order.
	adoptOrder []string
	// removals are the removal calls, in order.
	removals []fakeRemoval
	// calls counts the calls by method.
	calls map[string]int
}

// newFakeBackend builds a backend over log.
func newFakeBackend(log *writelog.Log) *fakeBackend {
	return &fakeBackend{log: log, holds: make(map[string]PackHold), now: time.Unix(1_000_000, 0), calls: make(map[string]int), removed: make(map[string]bool), adopted: make(map[string][][]byte)}
}

// ReadLog reads the write log.
func (b *fakeBackend) ReadLog(ctx context.Context, from writelog.Position, max int, fn func(writelog.Line) error) (writelog.ReadResult, error) {
	return b.log.ReadLog(ctx, from, max, fn)
}

// TrimLog trims the write log.
func (b *fakeBackend) TrimLog(ctx context.Context, to writelog.Position) error {
	return b.log.TrimLog(ctx, to)
}

// hold returns a pack's state.
func (b *fakeBackend) hold(id string) PackHold {
	if b.removed[id] {
		return PackHold{ID: id, Absent: true}
	}
	if h, ok := b.holds[id]; ok {
		return h
	}
	return PackHold{ID: id, State: PackRegistered}
}

// Status answers the packs' states, the log's and the operations.
func (b *fakeBackend) Status(_ context.Context, ids []string) (*BackendStatus, error) {
	// Answer each pack.
	b.calls["status"]++
	st := &BackendStatus{Now: b.now}
	for _, id := range ids {
		st.Packs = append(st.Packs, b.hold(id))
	}

	// Answer the log and the operations.
	if pos, ok := b.log.Position(); ok && !b.logOff {
		st.LogOn, st.LogEpoch = true, pos.Epoch
	}
	if b.ops != nil {
		ops := *b.ops
		st.Operations = &ops
	}
	return st, nil
}

// Quarantine holds the registered packs, keeping a held pack's time.
func (b *fakeBackend) Quarantine(_ context.Context, ids []string) ([]PackHold, error) {
	// Hold each registered pack.
	b.calls["quarantine"]++
	out := make([]PackHold, len(ids))
	for i, id := range ids {
		if h := b.hold(id); h.State == PackRegistered {
			b.holds[id] = PackHold{ID: id, State: PackQuarantined, Since: b.now}
		}
		out[i] = b.hold(id)
	}

	// Fail once after holding, when asked.
	if b.failQuarantine {
		b.failQuarantine = false
		return nil, errors.New("killed")
	}
	return out, nil
}

// Lift registers the quarantined packs.
func (b *fakeBackend) Lift(_ context.Context, ids []string) ([]PackHold, error) {
	b.calls["lift"]++
	out := make([]PackHold, len(ids))
	for i, id := range ids {
		if h := b.hold(id); h.State == PackQuarantined {
			b.holds[id] = PackHold{ID: id, State: PackRegistered, Since: b.now}
		}
		out[i] = b.hold(id)
	}
	return out, nil
}

// fakeRemoval is one removal call.
type fakeRemoval struct {
	ids  []string
	keys []KeyHolders
	more bool
}

// BeginTrash marks the quarantined packs trashing, refusing a registered
// pack as the pack store does.
func (b *fakeBackend) BeginTrash(_ context.Context, ids []string) ([]PackHold, error) {
	// Refuse a registered pack before changing any.
	b.calls["begin-trash"]++
	for _, id := range ids {
		if h := b.hold(id); h.Absent || h.State == PackRegistered {
			return nil, errors.Errorf("pack %s is not quarantined", id)
		}
	}

	// Mark each quarantined pack trashing.
	out := make([]PackHold, len(ids))
	for i, id := range ids {
		if b.hold(id).State == PackQuarantined {
			b.holds[id] = PackHold{ID: id, State: PackTrashing, Since: b.now}
		}
		out[i] = b.hold(id)
	}
	if b.failBeginTrash {
		b.failBeginTrash = false
		return nil, errors.New("killed")
	}
	return out, nil
}

// Remove records the call and drops the trashing packs no read missed,
// unless more keys follow.
func (b *fakeBackend) Remove(_ context.Context, ids []string, keys []KeyHolders, more bool) ([]PackHold, error) {
	// Refuse a pack that is not trashing, or that a read missed.
	b.calls["remove"]++
	b.removals = append(b.removals, fakeRemoval{ids: ids, keys: slices.Clone(keys), more: more})
	for _, id := range ids {
		if h := b.hold(id); !h.Absent && (h.State != PackTrashing || !h.Event.IsZero()) {
			return nil, errors.Errorf("pack %s is not removable", id)
		}
	}

	// Drop them unless more keys follow.
	out := make([]PackHold, len(ids))
	for i, id := range ids {
		if !more {
			b.removed[id] = true
		}
		out[i] = b.hold(id)
	}
	if b.failRemove || (b.failDrop && !more) {
		b.failRemove, b.failDrop = false, b.failDrop && more
		return nil, errors.New("killed")
	}
	return out, nil
}

// Restore registers the trashing packs a read missed, refusing any other
// trashing or held pack as the pack store does.
func (b *fakeBackend) Restore(_ context.Context, ids []string) ([]PackHold, error) {
	// Refuse a pack that is neither missed nor registered before changing any.
	b.calls["restore"]++
	for _, id := range ids {
		if h := b.hold(id); h.Absent || h.State == PackQuarantined {
			return nil, errors.Errorf("pack %s is not trashing", id)
		}
	}

	// Register each trashing pack, keeping its miss.
	out := make([]PackHold, len(ids))
	for i, id := range ids {
		if h := b.hold(id); h.State == PackTrashing {
			b.holds[id] = PackHold{ID: id, State: PackRegistered, Since: b.now, Event: b.now}
		}
		out[i] = b.hold(id)
	}
	if b.failRestore {
		b.failRestore = false
		return nil, errors.New("killed")
	}
	return out, nil
}

// Adopt registers the removed packs and the new ones, recording each pack's
// keys; a registered pack stays as it is.
func (b *fakeBackend) Adopt(_ context.Context, packs []AdoptedPack) error {
	// Refuse an adoption over the bound.
	b.calls["adopt"]++
	if len(packs) > MaxBackendAdopt {
		return errors.Errorf("an adoption names %d packs", len(packs))
	}

	// Register each removed or new pack.
	for _, p := range packs {
		id := p.Entry.GetId()
		_, known := b.adopted[id]
		if b.removed[id] || !known {
			delete(b.removed, id)
			b.holds[id] = PackHold{ID: id, State: PackRegistered, Since: b.now}
			b.adopted[id] = p.Keys
			b.adoptOrder = append(b.adoptOrder, id)
		}
	}

	// Fail once after registering, when asked.
	if b.failAdopt {
		b.failAdopt = false
		return errors.New("killed")
	}
	return nil
}

// recordsByID reads the pack records.
func recordsByID(t *testing.T, store kvtx.Store) map[string]*PackRecord {
	t.Helper()
	out := make(map[string]*PackRecord)
	if err := PackRecords(t.Context(), store, func(p *PackRecord) error {
		out[p.ID] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// holdFixture is a judged cycle with its dead packs, its log and backend.
type holdFixture struct {
	f       *fixture
	store   kvtx.Store
	log     *writelog.Log
	backend *fakeBackend
	opts    *CoverOptions
	dead    []string
}

// newHoldFixture snapshots, walks from R and judges a fixture.
func newHoldFixture(t *testing.T) *holdFixture {
	t.Helper()
	ctx := t.Context()
	h := &holdFixture{f: newFixture(t, 23, fixtureOptions{blocks: 400, roots: 2, packBlocks: 10, dupShare: 0.2}), store: store_kvtx_inmem.NewStore()}
	h.log, _ = newLog(t)
	h.backend = newFakeBackend(h.log)
	later := func() time.Time { return time.Now().Add(48 * time.Hour) }
	if _, err := Snapshot(ctx, &SnapshotOptions{Store: h.store, Packs: h.f.store, Reads: 3, Now: later}); err != nil {
		t.Fatal(err)
	}
	start := position(t, h.log)
	if _, err := Walk(ctx, &WalkOptions{Store: h.store, Roots: h.f.roots, Refs: h.f, LogPosition: start.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := Judge(ctx, &JudgeOptions{Store: h.store, LowLivePercent: 30}); err != nil {
		t.Fatal(err)
	}
	verdicts, _ := readResult(t, h.store)
	for id, v := range verdicts {
		if v == VerdictDead {
			h.dead = append(h.dead, id)
		}
	}
	slices.Sort(h.dead)
	if len(h.dead) < 6 {
		t.Fatalf("fixture has %d dead packs, want 6", len(h.dead))
	}
	h.opts = &CoverOptions{Store: h.store, Backend: h.backend, Refs: h.f}
	return h
}

// relink puts a leaf block of the first of ids holding one after R and
// returns the dead packs that hold it, which turn live.
func (h *holdFixture) relink(t *testing.T, ids ...string) []string {
	t.Helper()
	var b *fixtureBlock
	for _, id := range ids {
		if i := slices.IndexFunc(h.f.packs[id], func(b *fixtureBlock) bool { return len(b.refs) == 0 }); i >= 0 {
			b = h.f.packs[id][i]
			break
		}
	}
	if b == nil {
		t.Fatal("no dead pack holds a leaf block")
	}
	h.log.Put(b.ref.GetHash())
	var out []string
	for _, other := range h.dead {
		if slices.Contains(h.f.packs[other], b) {
			out = append(out, other)
		}
	}
	return out
}

// recheck runs a recheck with an hour's wait.
func (h *holdFixture) recheck(t *testing.T) *RecheckResult {
	t.Helper()
	res, err := Recheck(t.Context(), &RecheckOptions{CoverOptions: *h.opts, Wait: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestQuarantineRecheck carries the dead packs through quarantine and
// recheck: a pack relinked before the quarantine stays registered, the
// recheck holds the packs while they are young or an operation open since
// before them remains, lifts one a read lifted and one relinked since, and
// passes the rest, which a gap leaves passed.
func TestQuarantineRecheck(t *testing.T) {
	ctx := t.Context()
	h := newHoldFixture(t)

	// A dead pack relinked after R stays registered.
	liveBefore := h.relink(t, h.dead...)
	res, err := Quarantine(ctx, h.opts)
	if err != nil {
		t.Fatal(err)
	}
	quarantined := len(h.dead) - len(liveBefore)
	if res.Blocked != "" || res.Quarantined != uint64(quarantined) || res.Live != uint64(len(liveBefore)) || res.Unmoved != 0 { //nolint:gosec // test counts.
		t.Fatalf("quarantine = %+v, want %d quarantined and %d live", res, quarantined, len(liveBefore))
	}
	records := recordsByID(t, h.store)
	var held []string
	for _, id := range h.dead {
		p, ok := records[id]
		if slices.Contains(liveBefore, id) {
			if ok || h.backend.hold(id).State != PackRegistered {
				t.Fatalf("live pack %s quarantined", id)
			}
			continue
		}
		if !ok || p.Stage != StageQuarantine || !p.Since.Equal(h.backend.now) || h.backend.hold(id).State != PackQuarantined {
			t.Fatalf("pack %s: record %+v, state %v", id, p, h.backend.hold(id))
		}
		held = append(held, id)
	}

	// A second run calls the backend for nothing.
	if again, err := Quarantine(ctx, h.opts); err != nil || again.Quarantined != 0 || h.backend.calls["quarantine"] != 1 {
		t.Fatalf("second quarantine = %+v, %v, %d calls", again, err, h.backend.calls["quarantine"])
	}

	// The recheck holds young packs, then a backend that records no
	// operations, an open write and an open read from the quarantine.
	since := h.backend.now
	if got := h.recheck(t); got.Waiting[WaitQuarantine] != uint64(quarantined) || got.Passed != 0 { //nolint:gosec // test counts.
		t.Fatalf("young recheck = %+v", got)
	}
	h.backend.now = since.Add(2 * time.Hour)
	if got := h.recheck(t); got.Waiting[WaitNoOperations] != uint64(quarantined) { //nolint:gosec // test counts.
		t.Fatalf("recheck with no operations = %+v", got)
	}
	h.backend.ops = &OpenOperations{Write: since.Add(-time.Second), Read: since.Add(time.Second)}
	if got := h.recheck(t); got.Waiting[WaitOpenWrite] != uint64(quarantined) { //nolint:gosec // test counts.
		t.Fatalf("recheck with an old write = %+v", got)
	}
	h.backend.ops = &OpenOperations{Write: since.Add(time.Second), Read: since}
	if got := h.recheck(t); got.Waiting[WaitOpenRead] != uint64(quarantined) { //nolint:gosec // test counts.
		t.Fatalf("recheck with an old read = %+v", got)
	}
	if p := recordsByID(t, h.store)[held[0]]; p.Wait != WaitOpenRead || p.Stage != StageQuarantine {
		t.Fatalf("held record = %+v", p)
	}

	// A read lifts one pack, a put relinks another, and the rest pass.
	h.backend.ops = &OpenOperations{Write: since.Add(time.Second)}
	readID := held[0]
	h.backend.holds[readID] = PackHold{ID: readID, State: PackRegistered, Since: h.backend.now}
	relinked := h.relink(t, held[1:]...)
	if slices.Contains(relinked, readID) {
		t.Fatal("the relinked block is in the lifted pack")
	}
	got := h.recheck(t)
	passed := quarantined - 1 - len(relinked)
	if got.Blocked != "" || got.Lifted != 1 || got.Live != uint64(len(relinked)) || got.Passed != uint64(passed) || len(got.Waiting) != 0 { //nolint:gosec // test counts.
		t.Fatalf("recheck = %+v, want 1 lifted, %d live and %d passed", got, len(relinked), passed)
	}
	records = recordsByID(t, h.store)
	if len(records) != passed {
		t.Fatalf("%d records, want %d", len(records), passed)
	}
	for _, id := range relinked {
		if _, ok := records[id]; ok || h.backend.hold(id).State != PackRegistered {
			t.Fatalf("relinked pack %s still held", id)
		}
	}
	for _, p := range records {
		if p.Stage != StagePass || p.Wait != "" {
			t.Fatalf("record %+v", p)
		}
	}

	// A gap blocks the recheck and keeps every record.
	h.backend.logOff = true
	if got := h.recheck(t); got.Blocked != BlockLogGap {
		t.Fatalf("recheck in a gap = %+v", got)
	}
	if n := len(recordsByID(t, h.store)); n != passed {
		t.Fatalf("%d records after the gap, want %d", n, passed)
	}

	// A read that lifts a passed pack drops it from the cycle.
	h.backend.logOff = false
	var passedID string
	for id := range records {
		passedID = id
		break
	}
	h.backend.holds[passedID] = PackHold{ID: passedID, State: PackRegistered, Since: h.backend.now}
	if got := h.recheck(t); got.Lifted != 1 || got.Passed != uint64(passed-1) { //nolint:gosec // test counts.
		t.Fatalf("recheck after a passed pack's lift = %+v", got)
	}
	if _, ok := recordsByID(t, h.store)[passedID]; ok {
		t.Fatal("lifted passed pack kept its record")
	}
}

// TestQuarantineResumes kills a quarantine between the backend's change and
// the record of its answer: the next run asks again and records the first
// quarantine time. A state the recheck does not know then fails it.
func TestQuarantineResumes(t *testing.T) {
	ctx := t.Context()
	h := newHoldFixture(t)
	first := h.backend.now
	h.backend.failQuarantine = true
	if _, err := Quarantine(ctx, h.opts); err == nil {
		t.Fatal("killed quarantine returned no error")
	}
	for id, p := range recordsByID(t, h.store) {
		if !p.Since.IsZero() || h.backend.hold(id).State != PackQuarantined {
			t.Fatalf("pack %s after the kill: record %+v, state %v", id, p, h.backend.hold(id))
		}
	}

	// The recheck skips unanswered records; the next quarantine answers
	// them with the first time.
	if got := h.recheck(t); got.Passed != 0 || len(got.Waiting) != 0 || h.backend.calls["status"] != 0 {
		t.Fatalf("recheck of unanswered records = %+v", got)
	}
	h.backend.now = first.Add(time.Minute)
	res, err := Quarantine(ctx, h.opts)
	if err != nil || res.Quarantined != uint64(len(h.dead)) { //nolint:gosec // test counts.
		t.Fatalf("resumed quarantine = %+v, %v", res, err)
	}
	records := recordsByID(t, h.store)
	if len(records) != len(h.dead) {
		t.Fatalf("%d records, want %d", len(records), len(h.dead))
	}
	for _, p := range records {
		if !p.Since.Equal(first) {
			t.Fatalf("record %+v, want the first time %v", p, first)
		}
	}

	// A state the recheck does not know fails it rather than passing the
	// pack.
	h.backend.now = first.Add(2 * time.Hour)
	h.backend.ops = &OpenOperations{}
	h.backend.holds[h.dead[0]] = PackHold{ID: h.dead[0], State: PackState(7), Since: first}
	if _, err := Recheck(ctx, &RecheckOptions{CoverOptions: *h.opts, Wait: time.Hour}); err == nil {
		t.Fatal("rechecked a pack in an unknown state")
	}
	if p := recordsByID(t, h.store)[h.dead[0]]; p.Stage != StageQuarantine {
		t.Fatalf("record after the refused recheck = %+v", p)
	}
}

// TestPackRecordRow round-trips a record row.
func TestPackRecordRow(t *testing.T) {
	p := &PackRecord{ID: "pack", Stage: StageRemoved, Cycle: 3, Since: time.Unix(5, 6), Wait: WaitOpenRead, Removed: time.Unix(7, 8)}
	got, err := unmarshalPackRecord(p.ID, p.marshal())
	if err != nil || got.Stage != p.Stage || got.Cycle != p.Cycle || !got.Since.Equal(p.Since) || got.Wait != p.Wait || !got.Removed.Equal(p.Removed) {
		t.Fatalf("round trip = %+v, %v, want %+v", got, err, p)
	}
	if _, err := unmarshalPackRecord(p.ID, p.marshal()[:2]); err == nil {
		t.Fatal("decoded a short row")
	}
}
