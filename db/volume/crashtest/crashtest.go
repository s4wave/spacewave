// Package crashtest checks that a volume engine recovers from a crash at any
// point of a randomized workload to a state its commits allow.
//
// The workload mixes durable and ordered key-value commits, block puts and
// removes, journal appends, and syncs. A durable commit or sync makes the whole
// live state durable. An ordered commit may survive a crash only after every
// ordered commit before it, and block writes survive only through a later
// commit that publishes them. The commit a crash interrupts may or may not
// survive.
package crashtest

import (
	"bytes"
	"context"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// ops is the length of a workload.
const ops = 40

// keys is the key space of the workload's key-value commits.
const keys = 8

// blockSpace is the block space of the workload's puts and removes.
const blockSpace = 12

// seeds is the number of workloads Run checks.
const seeds = 8

// Blocks is which commits of a target publish its pending block writes and
// removes.
type Blocks int

const (
	// DurableBlocks targets publish pending blocks with durable commits only.
	DurableBlocks Blocks = iota
	// AllBlocks targets publish pending blocks with every commit.
	AllBlocks
)

// Device is storage that crashes and loses power on request.
type Device interface {
	// Calls returns the number of mutating calls made so far.
	Calls() int
	// CrashAfter fails every mutating call after the next n succeed.
	CrashAfter(n int)
	// PowerLoss replaces the contents with a state a power loss could leave
	// and accepts calls again.
	PowerLoss(rng *rand.Rand)
}

// Target is the engine under test.
type Target interface {
	kvtx.Store
	block.StoreOps

	// AppendJournal durably journals reference graph changes.
	AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error
	// AppendJournalOrdered journals reference graph changes with write
	// ordering only.
	AppendJournalOrdered(ctx context.Context, adds, removes []block_gc.RefEdge) error
	// ReplayJournal passes every journal entry in order to apply and removes
	// the entries.
	ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error
	// Close releases the engine.
	Close() error
}

// Run runs each workload on a new device from newDevice, crashes it at every
// mutating device call, applies a power loss, and checks that the target open
// reopens holds an allowed state and keeps working. blocks is which commits of
// the target publish its pending blocks.
func Run[D Device](t *testing.T, blocks Blocks, newDevice func() D, open func(ctx context.Context, d D) (Target, error)) {
	for seed := range uint64(seeds) {
		// Count the device calls of the uncrashed workload.
		d := newDevice()
		s, err := open(t.Context(), d)
		if err != nil {
			t.Fatal(err)
		}
		base := d.Calls()
		if _, err := runWorkload(t.Context(), s, blocks, seed); err != nil {
			t.Fatal(err)
		}
		calls := d.Calls() - base
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		// Crash at each call.
		for n := range calls {
			checkCrash(t, newDevice(), open, blocks, seed, n)
		}
		t.Logf("seed %d: %d crash points", seed, calls)
	}
}

// checkCrash runs the workload of seed on d, crashes after n mutating calls,
// and checks the recovered target.
func checkCrash[D Device](t *testing.T, d D, open func(ctx context.Context, d D) (Target, error), blocks Blocks, seed uint64, n int) {
	ctx := t.Context()
	s, err := open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.CrashAfter(n)
	allowed, err := runWorkload(ctx, s, blocks, seed)
	if err == nil {
		t.Fatalf("seed %d crash %d: workload finished without crashing", seed, n)
	}
	_ = s.Close()
	d.PowerLoss(rand.New(rand.NewPCG(seed, uint64(n)))) //nolint:gosec

	// Recover and compare with the allowed states.
	s, err = open(ctx, d)
	if err != nil {
		t.Fatalf("seed %d crash %d: reopen: %v", seed, n, err)
	}
	defer s.Close()
	got := readState(t, s)
	if !slices.ContainsFunc(allowed, got.equal) {
		t.Fatalf("seed %d crash %d: recovered %+v, want one of %+v", seed, n, got, allowed)
	}

	// The recovered target takes and publishes new writes.
	if _, _, err := s.PutBlock(ctx, []byte("after"), &block.PutOpts{Sync: true}); err != nil {
		t.Fatalf("seed %d crash %d: put after recovery: %v", seed, n, err)
	}
	if err := setValue(ctx, s, "after", "1", false); err != nil {
		t.Fatalf("seed %d crash %d: commit after recovery: %v", seed, n, err)
	}
}

// state is the durable state a target can recover to.
type state struct {
	// values holds the key-value store.
	values map[string]string
	// blocks holds the stored blocks by payload index.
	blocks map[int]bool
	// journal counts journal entries.
	journal int
}

// clone returns a deep copy of the state.
func (s state) clone() state {
	return state{values: maps.Clone(s.values), blocks: maps.Clone(s.blocks), journal: s.journal}
}

// equal reports whether two states match.
func (s state) equal(o state) bool {
	return maps.Equal(s.values, o.values) && maps.Equal(s.blocks, o.blocks) && s.journal == o.journal
}

// commitKind is how a workload step commits.
type commitKind int

const (
	// commitNone commits nothing.
	commitNone commitKind = iota
	// commitDurable commits durably with the pending blocks.
	commitDurable
	// commitOrdered commits with write ordering only.
	commitOrdered
)

// runWorkload runs the workload of seed until it finishes or a call fails. It
// returns the states a crash may recover: the state of the last acknowledged
// durable commit, of each ordered commit after it, and, when a commit failed,
// the state that commit would have reached.
func runWorkload(ctx context.Context, s Target, blocks Blocks, seed uint64) ([]state, error) {
	rng := rand.New(rand.NewPCG(seed, 0)) //nolint:gosec
	committed := state{values: make(map[string]string), blocks: make(map[int]bool)}
	live := committed.clone()
	allowed := []state{committed}
	for step := range ops {
		// Apply one step to the live state and, for an ordered commit, the
		// committed state. A durable commit makes the whole live state durable.
		nextLive, nextCommitted := live.clone(), committed.clone()
		var err error
		commit := commitDurable
		switch rng.IntN(8) {
		case 0, 1:
			key := "k" + strconv.Itoa(rng.IntN(keys))
			value := "v" + strconv.Itoa(step)
			nextLive.values[key] = value
			err = setValue(ctx, s, key, value, false)
		case 2:
			i := rng.IntN(blockSpace)
			nextLive.blocks[i] = true
			_, _, err = s.PutBlock(ctx, blockData(i), nil)
			commit = commitNone
		case 3:
			i := rng.IntN(blockSpace)
			delete(nextLive.blocks, i)
			err = s.RmBlock(ctx, blockRef(i))
			commit = commitNone
		case 4:
			nextLive.journal++
			edge := block_gc.RefEdge{Subject: "s" + strconv.Itoa(step), Object: "o"}
			err = s.AppendJournal(ctx, []block_gc.RefEdge{edge}, nil)
		case 5:
			key := "k" + strconv.Itoa(rng.IntN(keys))
			value := "o" + strconv.Itoa(step)
			nextLive.values[key], nextCommitted.values[key] = value, value
			err = setValue(ctx, s, key, value, true)
			commit = commitOrdered
		case 6:
			nextLive.journal++
			nextCommitted.journal++
			edge := block_gc.RefEdge{Subject: "s" + strconv.Itoa(step), Object: "o"}
			err = s.AppendJournalOrdered(ctx, []block_gc.RefEdge{edge}, nil)
			commit = commitOrdered
		default:
			_, err = s.Sync(ctx)
		}
		switch {
		case commit == commitDurable:
			nextCommitted = nextLive.clone()
		case commit == commitOrdered && blocks == AllBlocks:
			nextCommitted.blocks = maps.Clone(nextLive.blocks)
		}

		// Record the states a crash may now recover.
		if err != nil {
			if commit != commitNone {
				allowed = append(allowed, nextCommitted)
			}
			return allowed, err
		}
		live, committed = nextLive, nextCommitted
		switch commit {
		case commitDurable:
			allowed = []state{committed}
		case commitOrdered:
			allowed = append(allowed, committed)
		}
	}
	return allowed, nil
}

// setValue sets one key in its own committed transaction, with write ordering
// only if ordered is set.
func setValue(ctx context.Context, s Target, key, value string, ordered bool) error {
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
		return err
	}
	if ordered {
		return kvtx.CommitOrdered(ctx, tx)
	}
	return tx.Commit(ctx)
}

// readState reads the key-value store, the workload's blocks, and the journal
// length from s, checking every stored payload.
func readState(t *testing.T, s Target) state {
	ctx := t.Context()
	got := state{values: make(map[string]string), blocks: make(map[int]bool)}

	// Read the key-value store.
	tx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	err = tx.ScanPrefix(ctx, nil, func(key, value []byte) error {
		got.values[string(key)] = string(value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read every block the workload can put.
	for i := range blockSpace {
		data, found, err := s.GetBlock(ctx, blockRef(i))
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			continue
		}
		if !bytes.Equal(data, blockData(i)) {
			t.Fatalf("block %d reads %q", i, data)
		}
		got.blocks[i] = true
	}

	// Count the journal entries.
	err = s.ReplayJournal(ctx, func(adds, removes []block_gc.RefEdge) error {
		got.journal++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// blockData returns the payload of workload block i.
func blockData(i int) []byte {
	return bytes.Repeat([]byte("block-"+strconv.Itoa(i)+"."), 64)
}

// blockRef returns the reference of workload block i.
func blockRef(i int) *block.BlockRef {
	ref, err := block.BuildBlockRef(blockData(i), nil)
	if err != nil {
		panic(err)
	}
	return ref
}
