//go:build !js

package paylog

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
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/logindex"
)

// crashOps is the length of a crash workload.
const crashOps = 40

// crashKeys is the key space of the workload's key-value commits.
const crashKeys = 8

// crashBlocks is the block space of the workload's puts and removes.
const crashBlocks = 12

// crashState is the durable state a store can recover to.
type crashState struct {
	// values holds the key-value store.
	values map[string]string
	// blocks holds the published blocks by payload index.
	blocks map[int]bool
	// journal counts journal entries.
	journal int
}

// clone returns a deep copy of the state.
func (s crashState) clone() crashState {
	return crashState{values: maps.Clone(s.values), blocks: maps.Clone(s.blocks), journal: s.journal}
}

// equal reports whether two states match.
func (s crashState) equal(o crashState) bool {
	return maps.Equal(s.values, o.values) && maps.Equal(s.blocks, o.blocks) && s.journal == o.journal
}

// crashEngines are the engines the crash test runs on. The log engine
// checkpoints often and in the foreground, so its device calls keep one order
// and the crash points cover its checkpoints.
var crashEngines = []engine{boltEngine, logEngine(logindex.Options{CheckpointBytes: 128, Foreground: true})}

// TestCrashRecovery runs a randomized workload on the memory device, crashes
// it at every mutating device call, applies a power loss, and checks that the
// reopened store holds the state of the last acknowledged durable commit, of an
// ordered commit after it, or of the commit the crash interrupted, and that the
// store keeps working.
func TestCrashRecovery(t *testing.T) {
	for _, e := range crashEngines {
		t.Run(e.name, func(t *testing.T) {
			for seed := range uint64(8) {
				// Count the device calls of the uncrashed workload.
				d := device.NewMemory()
				s := openStore(t, e, d)
				base := d.Calls()
				if _, err := runCrashWorkload(t.Context(), s, seed); err != nil {
					t.Fatal(err)
				}
				calls := d.Calls() - base

				// Crash at each call.
				for n := range calls {
					checkCrash(t, e, seed, n)
				}
				t.Logf("seed %d: %d crash points", seed, calls)
			}
		})
	}
}

// checkCrash runs the workload of seed, crashes after n mutating calls, and
// checks the recovered store.
func checkCrash(t *testing.T, e engine, seed uint64, n int) {
	ctx := t.Context()
	d := device.NewMemory()
	s, err := e.openStore(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.CrashAfter(n)
	allowed, err := runCrashWorkload(ctx, s, seed)
	if err == nil {
		t.Fatalf("seed %d crash %d: workload finished without crashing", seed, n)
	}
	_ = s.Close()
	d.PowerLoss(rand.New(rand.NewPCG(seed, uint64(n))))

	// Recover and compare with the allowed states.
	s, err = e.openStore(ctx, d)
	if err != nil {
		t.Fatalf("seed %d crash %d: reopen: %v", seed, n, err)
	}
	defer s.Close()
	got := readCrashState(t, s)
	if !slices.ContainsFunc(allowed, got.equal) {
		t.Fatalf("seed %d crash %d: recovered %+v, want one of %+v", seed, n, got, allowed)
	}

	// The recovered store takes and publishes new writes.
	if _, _, err := s.PutBlock(ctx, []byte("after"), &block.PutOpts{Sync: true}); err != nil {
		t.Fatalf("seed %d crash %d: put after recovery: %v", seed, n, err)
	}
	if err := setValue(ctx, s, "after", "1", false); err != nil {
		t.Fatalf("seed %d crash %d: commit after recovery: %v", seed, n, err)
	}
}

// crashCommit is how a workload step commits the index.
type crashCommit int

const (
	// commitNone leaves the index unchanged.
	commitNone crashCommit = iota
	// commitDurable commits durably with the pending blocks.
	commitDurable
	// commitOrdered commits with write ordering only.
	commitOrdered
)

// runCrashWorkload runs the workload of seed until it finishes or a call
// fails. It returns the states a crash may recover: the state of the last
// acknowledged durable commit, of each ordered commit after it, and, when a
// commit failed, the state that commit would have reached.
func runCrashWorkload(ctx context.Context, s *Store, seed uint64) ([]crashState, error) {
	rng := rand.New(rand.NewPCG(seed, 0))
	index := crashState{values: make(map[string]string), blocks: make(map[int]bool)}
	live := index.clone()
	allowed := []crashState{index}
	for step := range crashOps {
		// Apply one step to the live state and, for an ordered commit, the
		// index state. A durable commit publishes the whole live state.
		nextLive, nextIndex := live.clone(), index.clone()
		var err error
		commit := commitDurable
		switch rng.IntN(8) {
		case 0, 1:
			key := "k" + strconv.Itoa(rng.IntN(crashKeys))
			value := "v" + strconv.Itoa(step)
			nextLive.values[key] = value
			err = setValue(ctx, s, key, value, false)
		case 2:
			i := rng.IntN(crashBlocks)
			nextLive.blocks[i] = true
			_, _, err = s.PutBlock(ctx, blockData(i), nil)
			commit = commitNone
		case 3:
			i := rng.IntN(crashBlocks)
			delete(nextLive.blocks, i)
			err = s.RmBlock(ctx, blockRef(i))
			commit = commitNone
		case 4:
			nextLive.journal++
			edge := block_gc.RefEdge{Subject: "s" + strconv.Itoa(step), Object: "o"}
			err = s.AppendJournal(ctx, []block_gc.RefEdge{edge}, nil)
		case 5:
			key := "k" + strconv.Itoa(rng.IntN(crashKeys))
			value := "o" + strconv.Itoa(step)
			nextLive.values[key], nextIndex.values[key] = value, value
			err = setValue(ctx, s, key, value, true)
			commit = commitOrdered
		case 6:
			nextLive.journal++
			nextIndex.journal++
			edge := block_gc.RefEdge{Subject: "s" + strconv.Itoa(step), Object: "o"}
			err = s.AppendJournalOrdered(ctx, []block_gc.RefEdge{edge}, nil)
			commit = commitOrdered
		default:
			_, err = s.Sync(ctx)
		}
		if commit == commitDurable {
			nextIndex = nextLive.clone()
		}

		// Record the states a crash may now recover.
		if err != nil {
			if commit != commitNone {
				allowed = append(allowed, nextIndex)
			}
			return allowed, err
		}
		live, index = nextLive, nextIndex
		switch commit {
		case commitDurable:
			allowed = []crashState{index}
		case commitOrdered:
			allowed = append(allowed, index)
		}
	}
	return allowed, nil
}

// setValue sets one key in its own committed transaction, with write ordering
// only if ordered is set.
func setValue(ctx context.Context, s *Store, key, value string, ordered bool) error {
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

// readCrashState reads the key-value store, the workload's blocks, and the
// journal length from s, checking every stored payload.
func readCrashState(t *testing.T, s *Store) crashState {
	ctx := t.Context()
	got := crashState{values: make(map[string]string), blocks: make(map[int]bool)}

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
	for i := range crashBlocks {
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
