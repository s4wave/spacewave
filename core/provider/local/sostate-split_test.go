package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	store_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// withStateLayout selects the split layout, or the host key, for one test.
func withStateLayout(t *testing.T, split bool) {
	prev := splitStateOn
	splitStateOn = split
	t.Cleanup(func() { splitStateOn = prev })
}

// splitTestObject is a shared object state whose owner can sign operations.
type splitTestObject struct {
	t    *testing.T
	priv crypto.PrivKey
	pid  string
	set  *sobject.SOOperationSet
}

// seedSplitTestState stores at the host key a genesis state of
// testSharedObjectID holding n operations, and returns it with its signer.
func seedSplitTestState(t *testing.T, store kvtx.Store, n int) (*sobject.SOState, *splitTestObject) {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.New())
	state, _, err := sobject.BuildGenesisSOState(le, nil, testSharedObjectID, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	set, err := state.OperationSet(testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	o := &splitTestObject{t: t, priv: priv, pid: pid.String(), set: set}
	for range n {
		o.addOp(state)
	}
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	runSplitTestTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID), data)
	})
	return state, o
}

// addOp signs a new operation into state and returns it.
func (o *splitTestObject) addOp(state *sobject.SOState) *sobject.SOOperation {
	opData := make([]byte, 64)
	_, _ = rand.Read(opData)
	op, err := sobject.BuildSOOperation(testSharedObjectID, o.priv, opData, state.NextOperationLink(o.set, o.pid), sobject.NewSOOperationLocalID())
	if err != nil {
		o.t.Fatal(err)
	}
	if _, err := state.AddOperation(testSharedObjectID, op); err != nil {
		o.t.Fatal(err)
	}
	if _, err := o.set.Add(op); err != nil {
		o.t.Fatal(err)
	}
	return op
}

// runSplitTestTx runs fn in a write transaction on store.
func runSplitTestTx(t *testing.T, store kvtx.Store, fn func(ctx context.Context, tx kvtx.Tx) error) {
	t.Helper()
	err := kvtx.RunTransaction(t.Context(), true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, fn)
	if err != nil {
		t.Fatal(err)
	}
}

// splitTestMount is one mount of testSharedObjectID through the production
// state funcs.
type splitTestMount struct {
	lock    sobject.SOStateLockFunc
	cancel  context.CancelFunc
	reloads int
}

// mountSplitTest mounts testSharedObjectID over store.
func mountSplitTest(t *testing.T, store kvtx.Store) *splitTestMount {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	m := &splitTestMount{cancel: cancel}
	_, m.lock, _ = NewObjectStoreSOStateFuncs(ctx, store, "", &SOStateRuns{Reload: func() { m.reloads++ }})
	return m
}

// write applies fn to the mount's state and writes the result.
func (m *splitTestMount) write(t *testing.T, fn func(*sobject.SOState)) (*sobject.SOState, error) {
	t.Helper()
	lk, err := m.lock(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Release()
	next := lk.GetSOState().Clone()
	fn(next)
	return next, lk.WriteSOState(t.Context(), next)
}

// loadSplitTestEncoded mounts store afresh and returns the loaded state's
// encoding.
func loadSplitTestEncoded(t *testing.T, store kvtx.Store) []byte {
	t.Helper()
	m := mountSplitTest(t, store)
	defer m.cancel()
	lk, err := m.lock(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Release()
	data, err := lk.GetSOState().MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// storedSplitTestLayout reports which layouts store holds keys of.
func storedSplitTestLayout(t *testing.T, store kvtx.Store) (host, split bool) {
	t.Helper()
	runSplitTestTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		if host, err = tx.Exists(ctx, SobjectObjectStoreHostStateKey(testSharedObjectID)); err != nil {
			return err
		}
		keys, err := scanKeysAfter(ctx, tx, splitStatePrefix(testSharedObjectID), nil)
		split = len(keys) != 0
		return err
	})
	return host, split
}

// splitTestOpStored reports whether store holds op under the split layout.
func splitTestOpStored(t *testing.T, store kvtx.Store, op *sobject.SOOperation) bool {
	t.Helper()
	var found bool
	runSplitTestTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		found, err = tx.Exists(ctx, append(splitStateOpPrefix(testSharedObjectID), op.Hash()...))
		return err
	})
	return found
}

// errSplitTestCrash is the failure crashStore injects.
var errSplitTestCrash = errors.New("injected crash")

// crashStore fails each write transaction at its failAt'th change, counting
// Set and Delete calls and then the commit, so the transaction discards as a
// crash at that step would.
type crashStore struct {
	kvtx.Store
	failAt int
	// hit records that a transaction reached failAt.
	hit bool
}

// NewTransaction opens a transaction that fails at the failAt'th change.
func (s *crashStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &crashTx{Tx: tx, s: s}, nil
}

// crashTx is a transaction of crashStore.
type crashTx struct {
	kvtx.Tx
	s *crashStore
	n int
}

// step counts one change and fails at failAt.
func (x *crashTx) step() error {
	x.n++
	if x.n == x.s.failAt {
		x.s.hit = true
		return errSplitTestCrash
	}
	return nil
}

func (x *crashTx) Set(ctx context.Context, key, value []byte) error {
	if err := x.step(); err != nil {
		return err
	}
	return x.Tx.Set(ctx, key, value)
}

func (x *crashTx) Delete(ctx context.Context, key []byte) error {
	if err := x.step(); err != nil {
		return err
	}
	return x.Tx.Delete(ctx, key)
}

func (x *crashTx) Commit(ctx context.Context) error {
	if err := x.step(); err != nil {
		x.Discard()
		return err
	}
	return x.Tx.Commit(ctx)
}

// TestSplitStateConvertCrashEachStep crashes the mount-time conversion at
// each of its changes and at its commit, in both directions, then remounts:
// storage must hold the old layout intact, and the remount must convert it
// and load the exact bytes the host key held.
func TestSplitStateConvertCrashEachStep(t *testing.T) {
	for _, toSplit := range []bool{true, false} {
		for failAt := 1; ; failAt++ {
			// Seed the state in the layout the conversion leaves.
			store := store_inmem.NewStore()
			state, _ := seedSplitTestState(t, store, 20)
			want, err := state.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			if !toSplit {
				withStateLayout(t, true)
				loadSplitTestEncoded(t, store)
			}
			withStateLayout(t, toSplit)

			// Crash the conversion at failAt.
			crash := &crashStore{Store: store, failAt: failAt}
			m := mountSplitTest(t, crash)
			lk, err := m.lock(t.Context(), testSharedObjectID)
			if err == nil {
				lk.Release()
			}
			m.cancel()
			if !crash.hit {
				if err != nil {
					t.Fatal(err)
				}
				break
			}
			if !errors.Is(err, errSplitTestCrash) {
				t.Fatalf("toSplit=%v failAt=%d: got %v, want the injected crash", toSplit, failAt, err)
			}

			// Storage holds only the old layout.
			host, split := storedSplitTestLayout(t, store)
			if host != toSplit || split == toSplit {
				t.Fatalf("toSplit=%v failAt=%d: stored host=%v split=%v after the crash", toSplit, failAt, host, split)
			}

			// The remount converts and loads the same bytes.
			if got := loadSplitTestEncoded(t, store); !bytes.Equal(got, want) {
				t.Fatalf("toSplit=%v failAt=%d: remount loaded other bytes", toSplit, failAt)
			}
			host, split = storedSplitTestLayout(t, store)
			if host == toSplit || split != toSplit {
				t.Fatalf("toSplit=%v failAt=%d: stored host=%v split=%v after the remount", toSplit, failAt, host, split)
			}
		}
	}
}

// TestSplitStateStaleGenerationWritesNothing writes through two mounts of one
// object: b holds its lock, and so its loaded generation, across a's write, so
// b's write over the stale generation must fail, write none of its operation,
// and reload; the stored state must be a's.
func TestSplitStateStaleGenerationWritesNothing(t *testing.T) {
	withStateLayout(t, true)
	store := store_inmem.NewStore()
	_, o := seedSplitTestState(t, store, 5)

	// Load b and hold its lock, then write through a.
	a, b := mountSplitTest(t, store), mountSplitTest(t, store)
	lk, err := b.lock(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Release()
	won, err := a.write(t, func(s *sobject.SOState) { o.addOp(s) })
	if err != nil {
		t.Fatal(err)
	}

	// b's write over the stale generation fails and stores nothing.
	next := lk.GetSOState().Clone()
	lost := o.addOp(next)
	err = lk.WriteSOState(t.Context(), next)
	if !errors.Is(err, ErrStateChanged) {
		t.Fatalf("stale write: got %v, want ErrStateChanged", err)
	}
	if b.reloads == 0 {
		t.Fatal("stale mount did not reload")
	}
	if splitTestOpStored(t, store, lost) {
		t.Fatal("stale write stored its operation")
	}
	want, err := won.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if got := loadSplitTestEncoded(t, store); !bytes.Equal(got, want) {
		t.Fatal("stored state is not the winning write")
	}
}

// TestSplitStateDropRacesAdd races a write that drops covered operations, as
// a checkpoint adoption does, against a write that adds one, through two
// mounts. One commits and the other fails; the loser then retries on the
// reloaded state. The final state must hold neither dropped operation nor
// lose the added one.
func TestSplitStateDropRacesAdd(t *testing.T) {
	withStateLayout(t, true)
	store := store_inmem.NewStore()
	seeded, o := seedSplitTestState(t, store, 8)
	dropped := seeded.GetOps()[:3]
	isDropped := func(op *sobject.SOOperation) bool {
		for _, d := range dropped {
			if d.EqualVT(op) {
				return true
			}
		}
		return false
	}
	drop := func(s *sobject.SOState) {
		kept := s.Ops[:0:0]
		for _, op := range s.Ops {
			if !isDropped(op) {
				kept = append(kept, op)
			}
		}
		s.Ops = kept
	}
	added := o.addOp(seeded.Clone())
	add := func(s *sobject.SOState) {
		if _, err := s.AddOperation(testSharedObjectID, added); err != nil {
			t.Error(err)
		}
	}

	// Race the two writes, each retrying on a fresh mount once it loses.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, fn := range []func(*sobject.SOState){drop, add} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				m := mountSplitTest(t, store)
				lk, err := m.lock(t.Context(), testSharedObjectID)
				if err != nil {
					errs[i] = err
					return
				}
				next := lk.GetSOState().Clone()
				fn(next)
				errs[i] = lk.WriteSOState(t.Context(), next)
				lk.Release()
				m.cancel()
				if !errors.Is(errs[i], ErrStateChanged) {
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// The stored state holds the added op and none of the dropped ones.
	data := loadSplitTestEncoded(t, store)
	got := &sobject.SOState{}
	if err := got.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if len(got.GetOps()) != 8-3+1 {
		t.Fatalf("stored %d ops, want %d", len(got.GetOps()), 8-3+1)
	}
	var found bool
	for _, op := range got.GetOps() {
		if isDropped(op) {
			t.Fatal("a dropped operation came back")
		}
		found = found || op.EqualVT(added)
	}
	if !found {
		t.Fatal("the added operation was lost")
	}
	for _, op := range dropped {
		if splitTestOpStored(t, store, op) {
			t.Fatal("a dropped operation's key remains")
		}
	}
}

// TestSplitStateLoadOrder10k converts a state of 10,000 operations, adds
// three through the lock, and reloads: each load must equal the in-memory
// state byte for byte.
func TestSplitStateLoadOrder10k(t *testing.T) {
	withStateLayout(t, true)
	store := store_inmem.NewStore()
	state, o := seedSplitTestState(t, store, 10000)
	want, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if got := loadSplitTestEncoded(t, store); !bytes.Equal(got, want) {
		t.Fatal("converted state loads other bytes")
	}

	m := mountSplitTest(t, store)
	var next *sobject.SOState
	for range 3 {
		if next, err = m.write(t, func(s *sobject.SOState) { o.addOp(s) }); err != nil {
			t.Fatal(err)
		}
	}
	m.cancel()
	if want, err = next.MarshalVT(); err != nil {
		t.Fatal(err)
	}
	if got := loadSplitTestEncoded(t, store); !bytes.Equal(got, want) {
		t.Fatal("state after split writes loads other bytes")
	}
}

// TestSplitStateFreshWriteDropsStrayKeys leaves operation keys under the split
// prefix with no head, as a clear interrupted between batches does, then
// stores a fresh state through storeNewSOState, which create, pairing and
// replace use: the stray operation must not load.
func TestSplitStateFreshWriteDropsStrayKeys(t *testing.T) {
	withStateLayout(t, true)
	store := store_inmem.NewStore()
	seeded, o := seedSplitTestState(t, store, 2)
	stray := o.addOp(seeded.Clone())
	runSplitTestTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		return setVT(ctx, tx, append(splitStateOpPrefix(testSharedObjectID), stray.Hash()...), stray)
	})

	// Store the seeded state fresh.
	data, err := seeded.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	runSplitTestTx(t, store, func(ctx context.Context, tx kvtx.Tx) error {
		_, err := storeNewSOState(ctx, tx, testSharedObjectID, seeded, data)
		return err
	})
	if host, _ := storedSplitTestLayout(t, store); host {
		t.Fatal("fresh split write kept the host key")
	}
	if got := loadSplitTestEncoded(t, store); !bytes.Equal(got, data) {
		t.Fatal("fresh state loads other bytes, such as a stray operation")
	}
}

// TestSplitStateRefusesUnsortedOps writes operations out of hash order: the
// split layout cannot hold that order, so the write must fail and store
// nothing.
func TestSplitStateRefusesUnsortedOps(t *testing.T) {
	withStateLayout(t, true)
	store := store_inmem.NewStore()
	seeded, o := seedSplitTestState(t, store, 4)
	want, err := seeded.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	m := mountSplitTest(t, store)
	var added *sobject.SOOperation
	_, err = m.write(t, func(s *sobject.SOState) {
		added = o.addOp(s)
		s.Ops[0], s.Ops[len(s.Ops)-1] = s.Ops[len(s.Ops)-1], s.Ops[0]
	})
	m.cancel()
	if !errors.Is(err, errStateOrder) {
		t.Fatalf("got %v, want errStateOrder", err)
	}
	if splitTestOpStored(t, store, added) {
		t.Fatal("refused write stored its operation")
	}
	if got := loadSplitTestEncoded(t, store); !bytes.Equal(got, want) {
		t.Fatal("refused write changed the stored state")
	}
}

// TestSplitStateAccountPaths runs the create, delete, replace and mount
// paths' own tests with the split layout selected.
func TestSplitStateAccountPaths(t *testing.T) {
	for name, fn := range map[string]func(*testing.T){
		"DeleteRemovesOnlyItsState":   TestDeleteSharedObjectRemovesOnlyItsState,
		"DeleteRelistStartsFresh":     TestDeleteSharedObjectRelistStartsFresh,
		"DeleteRacingHandleWrite":     TestDeleteSharedObjectRacingHandleWriteFails,
		"ReplaceRacingHandleWrite":    TestReplaceSharedObjectRacingHandleWrite,
		"DeleteInterruptedCompletes":  TestDeleteSharedObjectInterruptedCompletesOnRerun,
		"CreateStaleKeys":             TestCreateSharedObjectStaleKeys,
		"MountListedWithoutState":     TestMountListedObjectWithoutState,
		"DeleteKeepsRecreatedObject":  TestDeleteSharedObjectKeepsRecreatedObject,
		"RestartLoadWaitsForOldWrite": TestRestartLoadWaitsForOldWriter,
	} {
		t.Run(name, func(t *testing.T) {
			withStateLayout(t, true)
			fn(t)
		})
	}
}

// TestSplitStateRefusedOnce checks that a host key state whose parts do not
// load back to its bytes stays at the host key, and that the object's runs
// check and log that once rather than on every mount.
func TestSplitStateRefusedOnce(t *testing.T) {
	// A stamp holding other bytes than the state's encoding fails the check.
	state, _ := seedSplitTestState(t, store_inmem.NewStore(), 3)
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	stamp := soStateStamp{v: append(data, 0)}

	// Each run asks; only the first checks and logs.
	var logged bytes.Buffer
	log := logrus.New()
	log.SetOutput(&logged)
	runs := &SOStateRuns{Log: logrus.NewEntry(log)}
	for range 3 {
		if runs.mayConvert(state, stamp) {
			t.Fatal("a state that does not round trip may convert")
		}
	}
	if n := bytes.Count(logged.Bytes(), []byte("\n")); n != 1 {
		t.Fatalf("logged %d lines, want 1", n)
	}

	// The exact bytes convert, and a split state always does.
	if !(&SOStateRuns{}).mayConvert(state, soStateStamp{v: data}) || !runs.mayConvert(state, newSplitStamp()) {
		t.Fatal("a state that round trips may not convert")
	}
}
