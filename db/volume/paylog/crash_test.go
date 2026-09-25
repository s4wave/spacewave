//go:build !js

package paylog

import (
	"bytes"
	"context"
	"maps"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume/device"
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

// TestCrashRecovery runs a randomized workload on the memory device, crashes
// it at every mutating device call, applies a power loss, and checks that the
// reopened store holds the state of the last acknowledged index commit or of
// the commit the crash interrupted, and that the store keeps working.
func TestCrashRecovery(t *testing.T) {
	for seed := range uint64(2) {
		// Count the device calls of the uncrashed workload.
		d := device.NewMemory()
		s := openStore(t, d)
		base := d.Calls()
		if _, _, err := runCrashWorkload(t.Context(), s, seed); err != nil {
			t.Fatal(err)
		}
		calls := d.Calls() - base

		// Crash at each call.
		for n := range calls {
			checkCrash(t, seed, n)
		}
		t.Logf("seed %d: %d crash points", seed, calls)
	}
}

// checkCrash runs the workload of seed, crashes after n mutating calls, and
// checks the recovered store.
func checkCrash(t *testing.T, seed uint64, n int) {
	ctx := t.Context()
	d := device.NewMemory()
	s, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.CrashAfter(n)
	acked, interrupted, err := runCrashWorkload(ctx, s, seed)
	if err == nil {
		t.Fatalf("seed %d crash %d: workload finished without crashing", seed, n)
	}
	_ = s.Close()
	d.PowerLoss(rand.New(rand.NewPCG(seed, uint64(n))))

	// Recover and compare with the allowed states.
	s, err = Open(ctx, d)
	if err != nil {
		t.Fatalf("seed %d crash %d: reopen: %v", seed, n, err)
	}
	defer s.Close()
	got := readCrashState(t, s)
	if !got.equal(acked) && (interrupted == nil || !got.equal(*interrupted)) {
		t.Fatalf("seed %d crash %d: recovered %+v, want %+v or %+v", seed, n, got, acked, interrupted)
	}

	// The recovered store takes and publishes new writes.
	if _, _, err := s.PutBlock(ctx, []byte("after"), &block.PutOpts{Sync: true}); err != nil {
		t.Fatalf("seed %d crash %d: put after recovery: %v", seed, n, err)
	}
	if err := setValue(ctx, s, "after", "1"); err != nil {
		t.Fatalf("seed %d crash %d: commit after recovery: %v", seed, n, err)
	}
}

// runCrashWorkload runs the workload of seed until it finishes or a call
// fails. It returns the state of the last acknowledged index commit and, when
// a commit failed, the state that commit would have published.
func runCrashWorkload(ctx context.Context, s *Store, seed uint64) (crashState, *crashState, error) {
	rng := rand.New(rand.NewPCG(seed, 0))
	acked := crashState{values: make(map[string]string), blocks: make(map[int]bool)}
	live := acked.clone()
	for step := range crashOps {
		next := live.clone()
		var err error
		commit := true
		switch rng.IntN(6) {
		case 0, 1:
			key := "k" + strconv.Itoa(rng.IntN(crashKeys))
			value := "v" + strconv.Itoa(step)
			next.values[key] = value
			err = setValue(ctx, s, key, value)
		case 2:
			i := rng.IntN(crashBlocks)
			next.blocks[i] = true
			_, _, err = s.PutBlock(ctx, blockData(i), nil)
			commit = false
		case 3:
			i := rng.IntN(crashBlocks)
			delete(next.blocks, i)
			err = s.RmBlock(ctx, blockRef(i))
			commit = false
		case 4:
			next.journal++
			edge := block_gc.RefEdge{Subject: "s" + strconv.Itoa(step), Object: "o"}
			err = s.AppendJournal(ctx, []block_gc.RefEdge{edge}, nil)
		default:
			_, err = s.Sync(ctx)
		}
		if err != nil {
			if !commit {
				return acked, nil, err
			}
			return acked, &next, err
		}
		live = next
		if commit {
			acked = live.clone()
		}
	}
	return acked, nil, nil
}

// setValue sets one key in its own committed transaction.
func setValue(ctx context.Context, s *Store, key, value string) error {
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
		return err
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
