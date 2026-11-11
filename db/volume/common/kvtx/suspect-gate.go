package kvtx

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// suspectGate lets a Volume's reads skip suspect records while it holds none,
// as block_gc.Records.AssumeNoSuspects describes. Only a list rebuild's Finish
// writes a counted suspect record, and it opens the gate first.
//
// The gate keeps an epoch, even while the Volume may hold suspect records and
// odd while it proves it holds none, and the number of Finish calls running.
// The zero gate is open, so a Volume reads suspect records until its open
// reads its state. Finish opens the gate on entry and bumps the epoch again on
// exit. A reader that sees an open gate reads the suspect count in a snapshot
// opened after it loaded the epoch, and when the count is zero and no Finish
// runs, it moves the epoch from the value it loaded to the next odd one, which
// fails when a Finish entered or exited since.
//
// Invariant: a reader that skips the suspect read has observed a zero
// suspect count at a snapshot no Finish overlaps.
//
// A writer holding the store's writer loads the epoch after opening its
// transaction, so a Finish that opens the gate later writes no suspect record
// before that transaction ends. An optimistic fill holds no writer, so it
// registers as an assumer, and Finish waits on entry for the assumers that saw
// a closed gate to end.
type suspectGate struct {
	// epoch is even while the Volume may hold suspect records.
	epoch atomic.Uint64
	// running counts the Finish calls running.
	running atomic.Int64
	// assumers counts the optimistic transactions that skip suspect reads.
	assumers atomic.Int64
	// mu guards drained.
	mu sync.Mutex
	// drained is closed when the last assumer ends, nil unless a Finish
	// waits for it.
	drained chan struct{}
}

// init closes the gate when the Volume's state counts no suspect records.
// The Volume calls it once at open, before any Finish.
func (g *suspectGate) init(suspects bool) {
	if !suspects {
		g.epoch.Store(1)
	}
}

// load returns the epoch and whether the gate is closed: whether it proves
// the Volume holds no suspect records.
func (g *suspectGate) load() (uint64, bool) {
	e := g.epoch.Load()
	return e, e&1 == 1
}

// bump moves the epoch to its next even value, failing any close in flight.
func (g *suspectGate) bump() {
	for {
		e := g.epoch.Load()
		next := e + 2
		if e&1 == 1 {
			next = e + 1
		}
		if g.epoch.CompareAndSwap(e, next) {
			return
		}
	}
}

// close closes the gate opened at epoch e, after a snapshot opened after
// loading e counted no suspect records, unless a Finish runs.
func (g *suspectGate) close(e uint64) {
	if e&1 == 0 && g.running.Load() == 0 {
		g.epoch.CompareAndSwap(e, e+1)
	}
}

// marking opens the gate until the returned func is called, after the
// optimistic transactions that saw it closed have ended. It returns ctx's
// error, with the gate open and no Finish counted, when ctx ends first.
func (g *suspectGate) marking(ctx context.Context) (func(), error) {
	// Open the gate, then wait for the assumers that saw it closed.
	g.running.Add(1)
	g.bump()
	end := func() {
		g.bump()
		g.running.Add(-1)
	}
	for {
		g.mu.Lock()
		if g.assumers.Load() == 0 {
			g.mu.Unlock()
			return end, nil
		}
		if g.drained == nil {
			g.drained = make(chan struct{})
		}
		drained := g.drained
		g.mu.Unlock()
		select {
		case <-drained:
		case <-ctx.Done():
			end()
			return nil, context.Cause(ctx)
		}
	}
}

// assume registers an optimistic transaction that skips suspect reads, and
// reports whether it may: only while the gate is closed. The caller calls the
// returned func once the transaction ends.
func (g *suspectGate) assume() (bool, func()) {
	// Assume only while closed, registering, then check the gate did not
	// open before the registration, when a Finish may not have seen it.
	e, closed := g.load()
	if !closed {
		return false, func() {}
	}
	g.assumers.Add(1)
	if g.epoch.Load() != e {
		g.release()
		return false, func() {}
	}
	return true, g.release
}

// release ends an assumer, waking a Finish waiting for the last one. A
// closed gate has no Finish waiting: one that opens it after the decrement
// reads no assumer.
func (g *suspectGate) release() {
	if g.assumers.Add(-1) != 0 || g.epoch.Load()&1 == 1 {
		return
	}
	g.mu.Lock()
	if g.drained != nil {
		close(g.drained)
		g.drained = nil
	}
	g.mu.Unlock()
}

// hideSuspects clears exists for each of refs the Volume's records name a
// suspect block, so a writer that checks before it puts puts the block again,
// which writes its list from its content, as block_gc.SuspectError describes.
// While the gate is closed it reads nothing; while open it reads the suspect
// count in one snapshot, and closes the gate when the count is zero.
func (v *Volume) hideSuspects(ctx context.Context, refs []*block.BlockRef, exists []bool) error {
	// A Volume without liveness, or with the gate closed, has none.
	if !v.atomic {
		return nil
	}
	e, closed := v.gate.load()
	if closed {
		return nil
	}

	// Read the count, then the record of each block found.
	return v.readRecords(ctx, func(rec *block_gc.Records) error {
		if _, err := rec.GetState(ctx); err != nil {
			return err
		}
		if rec.NoSuspects() {
			v.gate.close(e)
			return nil
		}
		for i, ref := range refs {
			if !exists[i] {
				continue
			}
			suspect, err := rec.Suspect(ctx, ref)
			if err != nil {
				return err
			}
			exists[i] = !suspect
		}
		return nil
	})
}
