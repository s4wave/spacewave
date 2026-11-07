package packsweep

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Bounds of the quarantine and the recheck.
const (
	// DefaultQuarantineWait is how long a pack stays quarantined before its
	// recheck can pass it.
	DefaultQuarantineWait = 24 * time.Hour
	// MaxBackendPacks is the packs one backend call names, the admin
	// service's bound.
	MaxBackendPacks = 1024
	// MaxBackendKeys is the keys and their holders together one removal
	// call carries, the admin service's bound.
	MaxBackendKeys = 65536
	// MaxKeyHolders is the other holders a removal names for one key, the
	// admin service's bound.
	MaxKeyHolders = 16
)

// PackState is a pack's state in the backend's pack store, numbered as the
// pack store numbers it.
type PackState byte

// The pack states.
const (
	// PackRegistered is a pack every read and locate sees.
	PackRegistered PackState = iota
	// PackQuarantined is a pack only the fallback reads, which lifts it.
	PackQuarantined
	// PackTrashing is a pack the command is moving to the trash.
	PackTrashing
)

// PackHold is a pack's state in the backend's pack store.
type PackHold struct {
	// ID is the pack.
	ID string
	// State is the pack's state.
	State PackState
	// Since is when the pack entered its state, on the backend's clock, or
	// zero for a pack the sweep never moved.
	Since time.Time
	// Event is when the last lift or miss read the pack, on the backend's
	// clock, or zero. A trashing pack with an event was missed by a read
	// after its move, and goes back by restore.
	Event time.Time
	// Absent is set for a pack the backend does not hold: removed, or never
	// registered.
	Absent bool
}

// KeyHolders is a block key a removed pack holds, by its tail, with the
// other packs the snapshot lists holding it.
type KeyHolders struct {
	// Key is the key's tail.
	Key []byte
	// Others are the other packs that hold it.
	Others []string
}

// OpenOperations is when the oldest write and read operation still open on
// the backend's store began, each zero for none.
type OpenOperations struct {
	// Write is the start of the oldest write in flight.
	Write time.Time
	// Read is the start of the oldest read operation not yet released.
	Read time.Time
}

// BackendStatus is the backend's side of the sweep at one instant.
type BackendStatus struct {
	// Packs are the packs' states, in the order asked.
	Packs []PackHold
	// LogOn is set while the write log has an epoch open.
	LogOn bool
	// LogEpoch is the open epoch, or the last one.
	LogEpoch string
	// Operations is the oldest open write and read operation, nil when the
	// backend records none.
	Operations *OpenOperations
	// Now is the backend's clock when it read the state.
	Now time.Time
}

// Backend is the backend's side of a cycle, as the admin service serves it.
// Each call names at most MaxBackendPacks packs.
type Backend interface {
	LogSource
	// Status returns the packs' states, the log's and the open operations.
	Status(ctx context.Context, ids []string) (*BackendStatus, error)
	// Quarantine holds registered packs out of the reads and returns their
	// states after.
	Quarantine(ctx context.Context, ids []string) ([]PackHold, error)
	// Lift returns quarantined packs to the reads and returns their states
	// after.
	Lift(ctx context.Context, ids []string) ([]PackHold, error)
	// BeginTrash moves quarantined packs to the trashing state and returns
	// their states after.
	BeginTrash(ctx context.Context, ids []string) ([]PackHold, error)
	// Remove repairs the h rows of keys and drops the trashing packs ids,
	// or only repairs the rows when more is set, and returns their states
	// after. keys carry at most MaxBackendKeys keys and holders together.
	Remove(ctx context.Context, ids []string, keys []KeyHolders, more bool) ([]PackHold, error)
}

// Stage is the command's record of a pack it moved: the step the pack is at,
// written before the call that moves it.
type Stage byte

// The stages, in the order a pack goes through them.
const (
	// StageQuarantine is a pack the command quarantined, or is about to.
	StageQuarantine Stage = iota + 1
	// StagePass is a quarantined pack that passed its recheck.
	StagePass
	// StageTrash is a passed pack the command moves to the trashing state,
	// then to the trash.
	StageTrash
	// StageMoved is a trashing pack whose entry and file are in the trash.
	StageMoved
	// StageRemoved is a pack the backend removed; it stays in the trash
	// until the operator purges it.
	StageRemoved
)

// String names the stage.
func (s Stage) String() string {
	switch s {
	case StageQuarantine:
		return "quarantine"
	case StagePass:
		return "pass"
	case StageTrash:
		return "trash"
	case StageMoved:
		return "moved"
	case StageRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// Reasons a recheck holds a quarantined pack another round.
const (
	// WaitQuarantine is a pack quarantined for less than the wait.
	WaitQuarantine = "waiting"
	// WaitNoOperations is a backend that records no open operations.
	WaitNoOperations = "no-operations"
	// WaitOpenWrite is a write in flight since before the quarantine: it may
	// log a block of the pack after the recheck read the log.
	WaitOpenWrite = "open-write"
	// WaitOpenRead is a read operation open since before the quarantine: it
	// may hold a block of the pack it read without the fallback.
	WaitOpenRead = "open-read"
)

// PackRecord is the command's record of a pack it moved. Records stay across
// cycles: a gap sends the cycle back to its walk, and every quarantine stays.
type PackRecord struct {
	// ID is the pack.
	ID string
	// Stage is the pack's stage.
	Stage Stage
	// Cycle is the cycle that quarantined the pack.
	Cycle int
	// Since is when the backend quarantined the pack, on its clock; zero
	// until the backend's answer was recorded.
	Since time.Time
	// Wait is why the last recheck held the pack, empty for none.
	Wait string
	// Removed is when the backend removed the pack, on the command's clock,
	// which the trash's retention counts from; zero until it did.
	Removed time.Time
}

// marshal encodes the record row.
func (p *PackRecord) marshal() []byte {
	// Append each field in order.
	b := []byte{byte(p.Stage)}
	b = appendUvarints(b, uint64(p.Cycle)) //nolint:gosec // A cycle number is positive.
	b = appendTime(b, p.Since)
	b = appendString(b, p.Wait)
	return appendTime(b, p.Removed)
}

// unmarshalPackRecord decodes a record row.
func unmarshalPackRecord(id string, b []byte) (*PackRecord, error) {
	r := &rowReader{b: b, ok: true}
	p := &PackRecord{ID: id, Stage: Stage(r.byte()), Cycle: int(r.uvarint()), Since: r.time(), Wait: r.string(), Removed: r.time()} //nolint:gosec // The row holds a cycle number this command wrote.
	return p, r.finish("pack record row")
}

// PackRecords calls fn with each pack record, in pack id order.
func PackRecords(ctx context.Context, store kvtx.Store, fn func(*PackRecord) error) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return tx.ScanPrefix(ctx, []byte(packRecordPrefix), func(key, value []byte) error {
		p, err := unmarshalPackRecord(string(key[len(packRecordPrefix):]), value)
		if err != nil {
			return err
		}
		return fn(p)
	})
}

// readPackRecords reads every pack record.
func readPackRecords(ctx context.Context, store kvtx.Store) ([]*PackRecord, error) {
	var out []*PackRecord
	err := PackRecords(ctx, store, func(p *PackRecord) error {
		out = append(out, p)
		return nil
	})
	return out, err
}

// writePackRecords writes and deletes records in one transaction.
func writePackRecords(ctx context.Context, store kvtx.Store, set []*PackRecord, del []string) error {
	// Open a write.
	if len(set) == 0 && len(del) == 0 {
		return nil
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write and delete each row, then commit them together.
	for _, p := range set {
		if err := tx.Set(ctx, []byte(packRecordPrefix+p.ID), p.marshal()); err != nil {
			return err
		}
	}
	for _, id := range del {
		if err := tx.Delete(ctx, []byte(packRecordPrefix+id)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CoverOptions configures the log pull and closure a step runs first.
type CoverOptions struct {
	// Store is the scratch store, holding a finished walk and judgment.
	Store kvtx.Store
	// Backend is the backend's side of the cycle, and the log's source.
	Backend Backend
	// Refs serves blocks the catalog lacks, for the closure.
	Refs RefSource
	// Accepted are the missing blocks the operator accepted, by b58 ref.
	Accepted map[string]bool
	// Batch is the closure's frontier keys per transaction.
	Batch int
}

// cover pulls the log from the walk's R and walks W, and returns the header
// with why the cycle may not move a pack, empty when it may: a hole the
// operator did not accept, or a log that stopped covering the cycle.
func cover(ctx context.Context, opts *CoverOptions) (*Head, string, error) {
	// Check the walk was judged, and the holes.
	head, err := ReadHead(ctx, opts.Store)
	if err != nil {
		return nil, "", err
	}
	if !head.Walked || head.JudgedAt.IsZero() {
		return nil, "", errors.New("moving a pack needs a judged walk")
	}
	var holes []Hole
	err = Holes(ctx, opts.Store, func(h Hole) error {
		holes = append(holes, h)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if blocking := BlockingHoles(head, opts.Accepted, holes); len(blocking) != 0 {
		return head, "holes " + strings.Join(slices.Sorted(maps.Keys(blocking)), ","), nil
	}
	if head.LogPosition == "" {
		return head, BlockNoLog, nil
	}

	// Pull the log to its open end, then walk what it reached.
	pull, err := PullLog(ctx, &PullOptions{Store: opts.Store, Source: opts.Backend, Start: head.LogPosition})
	if err != nil {
		return nil, "", err
	}
	if reason := LogBlocking(head, pull); reason != "" {
		return head, reason, nil
	}
	head, err = Closure(ctx, &ClosureOptions{Store: opts.Store, Refs: opts.Refs, Batch: opts.Batch})
	return head, "", err
}

// removable reports whether no key pack id holds is in L or W. A key in L or
// W that another registered pack holds would also do, as the rule allows, but
// the catalog keeps only each key's tail, so the backend cannot place it; a
// dead pack that gained a live key since its judgment is lifted instead.
func removable(ctx context.Context, tx kvtx.TxOps, id string) (bool, error) {
	// Check the pack is catalogued.
	found, err := tx.Exists(ctx, []byte(catalogPackPrefix+id))
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.Errorf("pack %s is not in the catalog", id)
	}

	// Stop at the first live key.
	errLive := errors.New("live")
	prefix := []byte(catalogMemberPrefix + id + "/")
	err = tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		live, err := isLive(ctx, tx, key[len(prefix):])
		if err == nil && live {
			err = errLive
		}
		return err
	})
	if err == errLive {
		return false, nil
	}
	return err == nil, err
}

// removablePacks returns which of ids is removable, in one read.
func removablePacks(ctx context.Context, store kvtx.Store, ids []string) ([]bool, error) {
	// Check each pack in one read transaction.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	out := make([]bool, len(ids))
	for i, id := range ids {
		if out[i], err = removable(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// QuarantineResult reports a quarantine.
type QuarantineResult struct {
	// Blocked is why the cycle may not move a pack, empty when it may.
	Blocked string
	// Quarantined is the packs the backend holds quarantined for the cycle.
	Quarantined uint64
	// Live is the dead packs that gained a live key since the judgment, left
	// registered.
	Live uint64
	// Unmoved is the packs the backend answered were not quarantined, such
	// as one it no longer holds; their records stay for the next run.
	Unmoved uint64
}

// Quarantine pulls the log and walks W, then quarantines through the backend
// every pack judged dead that is still removable, a batch at a time. Each
// batch's records are written before the call and their quarantine times
// after it, so a run killed between the two calls the backend again for the
// same packs, which keeps each held pack's first quarantine time. Those packs
// are not checked again here: the recheck lifts one that turned live. A pack
// a read lifted leaves the cycle, and a later run may quarantine it again
// once the log line of that read is in W.
func Quarantine(ctx context.Context, opts *CoverOptions) (*QuarantineResult, error) {
	// Cover the cycle from R.
	head, blocked, err := cover(ctx, opts)
	if err != nil || blocked != "" {
		return &QuarantineResult{Blocked: blocked}, err
	}

	// Take the packs recorded and not yet answered, then the dead packs with
	// no record.
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	recorded := make(map[string]bool, len(records))
	var ids []string
	for _, p := range records {
		recorded[p.ID] = true
		if p.Stage == StageQuarantine && p.Since.IsZero() {
			ids = append(ids, p.ID)
		}
	}
	var dead []string
	err = Judgments(ctx, opts.Store, func(j *Judgment) error {
		if j.Verdict == VerdictDead && !recorded[j.ID] {
			dead = append(dead, j.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Keep the dead packs still removable.
	res := &QuarantineResult{}
	ok, err := removablePacks(ctx, opts.Store, dead)
	if err != nil {
		return nil, err
	}
	for i, id := range dead {
		if ok[i] {
			ids = append(ids, id)
		} else {
			res.Live++
		}
	}

	// Quarantine a batch at a time, recording before and after each call.
	for batch := range slices.Chunk(ids, MaxBackendPacks) {
		if err := quarantineBatch(ctx, opts, head.Cycle, batch, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// quarantineBatch records a batch of packs, quarantines them, and records the
// backend's answer.
func quarantineBatch(ctx context.Context, opts *CoverOptions, cycle int, ids []string, res *QuarantineResult) error {
	// Record the packs before the call.
	records := make([]*PackRecord, len(ids))
	for i, id := range ids {
		records[i] = &PackRecord{ID: id, Stage: StageQuarantine, Cycle: cycle}
	}
	if err := writePackRecords(ctx, opts.Store, records, nil); err != nil {
		return err
	}

	// Quarantine them, then record when the backend did.
	holds, err := opts.Backend.Quarantine(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "quarantine")
	}
	if len(holds) != len(ids) {
		return errors.Errorf("the backend answered %d of %d packs", len(holds), len(ids))
	}
	for i, h := range holds {
		if h.ID != ids[i] || h.State != PackQuarantined || h.Since.IsZero() {
			res.Unmoved++
			continue
		}
		records[i].Since = h.Since
		res.Quarantined++
	}
	return writePackRecords(ctx, opts.Store, records, nil)
}

// RecheckOptions configures Recheck.
type RecheckOptions struct {
	CoverOptions
	// Wait is how long a pack stays quarantined before it can pass. Zero
	// uses DefaultQuarantineWait.
	Wait time.Duration
}

// RecheckResult reports a recheck.
type RecheckResult struct {
	// Blocked is why the cycle may not move a pack, empty when it may.
	Blocked string
	// Passed is the packs that passed, now and before, still held.
	Passed uint64
	// Lifted is the packs a read lifted, dropped from the cycle.
	Lifted uint64
	// Live is the packs that gained a live key, lifted by the command.
	Live uint64
	// Gone is the packs the backend no longer holds, dropped from the cycle.
	Gone uint64
	// Waiting counts the packs held another round, by reason.
	Waiting map[string]uint64
}

// Recheck checks each quarantined pack again: it reads the backend's status
// first, then pulls the log to its open end and walks W, so every write that
// ended before the status is in W. A pack passes once it has waited out the
// quarantine, no write or read operation open since before its quarantine
// remains, the log covers the cycle in one epoch from R, and no key it holds
// is in L or W. A pack a read lifted leaves the cycle, passed or not, and one
// that gained a live key is lifted. A pack that passed stays passed through a
// later gap: no log line after its recheck could keep it.
func Recheck(ctx context.Context, opts *RecheckOptions) (*RecheckResult, error) {
	// Take the quarantined packs, and read their states first.
	res := &RecheckResult{Waiting: make(map[string]uint64)}
	records, err := readPackRecords(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	var quar []*PackRecord
	for _, p := range records {
		// A quarantined pack has its quarantine time; a pack past its pass
		// is the trash's.
		if !p.Since.IsZero() && p.Stage <= StagePass {
			quar = append(quar, p)
		}
	}
	if len(quar) == 0 {
		return res, nil
	}
	status, err := backendStatus(ctx, opts.Backend, quar)
	if err != nil {
		return nil, err
	}

	// Cover the cycle from R after the status, in the epoch the log is in.
	head, blocked, err := cover(ctx, &opts.CoverOptions)
	if err != nil || blocked != "" {
		res.Blocked = blocked
		return res, err
	}
	if r, err := writelog.ParsePosition(head.LogPosition); err != nil {
		return nil, err
	} else if !status.LogOn || status.LogEpoch != r.Epoch {
		res.Blocked = BlockLogGap
		return res, nil
	}

	// Judge each pack, then write the records and lift the live packs.
	wait := opts.Wait
	if wait <= 0 {
		wait = DefaultQuarantineWait
	}
	for start := 0; start < len(quar); start += MaxBackendPacks {
		end := min(start+MaxBackendPacks, len(quar))
		if err := recheckBatch(ctx, opts, quar[start:end], status.Packs[start:end], status, wait, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// backendStatus reads the states of the recorded packs, a batch per call,
// keeping the earliest clock and the oldest open operations of the calls.
func backendStatus(ctx context.Context, backend Backend, records []*PackRecord) (*BackendStatus, error) {
	out := &BackendStatus{Operations: &OpenOperations{}}
	for start := 0; start < len(records); start += MaxBackendPacks {
		// Read one batch, and check it answers each pack.
		batch := records[start:min(start+MaxBackendPacks, len(records))]
		ids := make([]string, len(batch))
		for i, p := range batch {
			ids[i] = p.ID
		}
		st, err := backend.Status(ctx, ids)
		if err != nil {
			return nil, errors.Wrap(err, "status")
		}
		if len(st.Packs) != len(ids) {
			return nil, errors.Errorf("the backend answered %d of %d packs", len(st.Packs), len(ids))
		}
		for i, h := range st.Packs {
			if h.ID != ids[i] {
				return nil, errors.Errorf("the backend answered pack %s for %s", h.ID, ids[i])
			}
		}

		// Merge it: the first call's log and clock, the oldest operations.
		out.Packs = append(out.Packs, st.Packs...)
		if start == 0 {
			out.LogOn, out.LogEpoch, out.Now = st.LogOn, st.LogEpoch, st.Now
		} else if st.LogEpoch != out.LogEpoch || !st.LogOn {
			out.LogOn = false
		}
		if st.Operations == nil {
			out.Operations = nil
		}
		if out.Operations != nil {
			out.Operations.Write = earliest(out.Operations.Write, st.Operations.Write)
			out.Operations.Read = earliest(out.Operations.Read, st.Operations.Read)
		}
	}
	return out, nil
}

// earliest returns the earlier of two times, zero standing for none.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// recheckBatch rechecks a batch of quarantined packs against their states.
func recheckBatch(ctx context.Context, opts *RecheckOptions, records []*PackRecord, holds []PackHold, status *BackendStatus, wait time.Duration, res *RecheckResult) error {
	// Sort each pack by its state and its wait: a lifted pack leaves the
	// cycle, and a passed pack stays passed.
	var set, live []*PackRecord
	var del, check []string
	var checked []*PackRecord
	for i, p := range records {
		h := holds[i]
		switch {
		case h.Absent:
			del = append(del, p.ID)
			res.Gone++
			continue
		case h.State == PackRegistered:
			del = append(del, p.ID)
			res.Lifted++
			continue
		case p.Stage == StagePass:
			res.Passed++
			continue
		case h.State != PackQuarantined:
			return errors.Errorf("pack %s is in state %d with a quarantine record", p.ID, h.State)
		}
		p.Since = h.Since
		p.Wait = holdWait(h, status, wait)
		if p.Wait != "" {
			res.Waiting[p.Wait]++
			set = append(set, p)
			continue
		}
		check = append(check, p.ID)
		checked = append(checked, p)
	}

	// Pass the removable packs; the rest are live.
	ok, err := removablePacks(ctx, opts.Store, check)
	if err != nil {
		return err
	}
	for i, p := range checked {
		if ok[i] {
			p.Stage = StagePass
			res.Passed++
			set = append(set, p)
		} else {
			live = append(live, p)
		}
	}
	if err := writePackRecords(ctx, opts.Store, set, del); err != nil {
		return err
	}

	// Lift the live packs, then drop their records.
	if len(live) == 0 {
		return nil
	}
	ids := make([]string, len(live))
	for i, p := range live {
		ids[i] = p.ID
	}
	if _, err := opts.Backend.Lift(ctx, ids); err != nil {
		return errors.Wrap(err, "lift the live packs")
	}
	res.Live += uint64(len(ids))
	return writePackRecords(ctx, opts.Store, nil, ids)
}

// holdWait returns why a quarantined pack waits another round, empty for
// none.
func holdWait(h PackHold, status *BackendStatus, wait time.Duration) string {
	ops := status.Operations
	switch {
	case status.Now.Sub(h.Since) < wait:
		return WaitQuarantine
	case ops == nil:
		return WaitNoOperations
	case !ops.Write.IsZero() && !ops.Write.After(h.Since):
		return WaitOpenWrite
	case !ops.Read.IsZero() && !ops.Read.After(h.Since):
		return WaitOpenRead
	default:
		return ""
	}
}
