//go:build !js && !wasip1

package provider_local

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// MeasureStateWriteEnv names the environment variable that, set to 1, runs
// TestMeasureOpWriteCost.
const MeasureStateWriteEnv = "SPACEWAVE_MEASURE_STATE_WRITE"

// maxCheckpointData is near the most state data a checkpoint can carry:
// sobject.MaxSyncStateSize bounds the whole signed checkpoint, so the state
// data leaves room for the signature and the other fields.
const maxCheckpointData = sobject.MaxSyncStateSize - 4096

// TestMeasureOpWriteCost measures what one operation costs through the
// mounted state lock on the desktop's on-disk volume, in each layout, at
// three counts of operations above the checkpoint and with a checkpoint of
// maxCheckpointData state data: the bytes s4db wrote to the file per op,
// including its own flushes, merges and cleaning, the mean time of a write
// through the lock, and the time of a fresh mount's load. Runs only when
// MeasureStateWriteEnv is 1.
func TestMeasureOpWriteCost(t *testing.T) {
	if os.Getenv(MeasureStateWriteEnv) != "1" {
		t.Skip("set " + MeasureStateWriteEnv + "=1 to measure")
	}
	for _, split := range []bool{false, true} {
		for _, c := range []struct{ held, n, cp int }{{32, 40, 0}, {1000, 40, 0}, {10000, 10, 0}, {32, 20, maxCheckpointData}} {
			measureOpWriteCost(t, split, c.held, 200, c.n, c.cp)
		}
	}
}

// TestMeasureLayoutWriterHold measures how long each whole-state write of
// the split layout holds the store's writer on the desktop's on-disk volume,
// for a state of 10,000 operations: the conversion from the host key to the
// split layout, the conversion back, and the removal of the split state's
// keys that DeleteSharedObject's transaction runs. Each is timed from the
// transaction's open to its commit, after the volume made the step before it
// durable. It also times the round trip check a split runs before its
// transaction. Runs only when MeasureStateWriteEnv is 1.
func TestMeasureLayoutWriterHold(t *testing.T) {
	if os.Getenv(MeasureStateWriteEnv) != "1" {
		t.Skip("set " + MeasureStateWriteEnv + "=1 to measure")
	}
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.WarnLevel)

	// Store the state at the host key.
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	vol, store, release := openMeasureStore(ctx, t, le, conf)
	defer release()
	state, _ := seedSplitTestState(t, store, 10000)
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// hold runs fn in a write transaction once the volume is durable, and
	// returns how long the transaction held the writer.
	hold := func(fn func(ctx context.Context, tx kvtx.Tx) error) time.Duration {
		if err := volume.WaitDurable(ctx, vol); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, fn)
		if err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	stamp := soStateStamp{v: data}
	convert := func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		stamp, err = convertSOState(ctx, tx, testSharedObjectID, state, stamp)
		return err
	}

	// Check the round trip, then split the state and put it back.
	withStateLayout(t, true)
	start := time.Now()
	if same, err := splitRoundTrips(state, data); err != nil || !same {
		t.Fatalf("round trip: %v %v", same, err)
	}
	check := time.Since(start)
	toSplit := hold(convert)
	withStateLayout(t, false)
	toHost := hold(convert)

	// Split it again and delete the object's keys.
	withStateLayout(t, true)
	hold(convert)
	del := hold(func(ctx context.Context, tx kvtx.Tx) error {
		return deleteKeysWithPrefix(ctx, tx, SobjectObjectStorePrefix(testSharedObjectID))
	})

	loadavg, _ := os.ReadFile("/proc/loadavg")
	t.Logf(
		"10000 ops, state %d bytes: round trip check %v (no writer); writer held: host to split %v, split to host %v, delete split %v (loadavg %s)",
		len(data), check, toSplit, toHost, del, loadavg,
	)
}

// measureOpWriteCost adds n ops of opDataSize bytes through the state lock to
// a stored state already holding held ops above a checkpoint of cpDataSize
// state data, the genesis checkpoint when zero, and logs the cost per op.
func measureOpWriteCost(t *testing.T, split bool, held, opDataSize, n, cpDataSize int) {
	withStateLayout(t, split)
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.WarnLevel)
	const id = "measure"

	// Build the state with held ops.
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := sobject.BuildGenesisSOState(le, nil, id, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cpDataSize != 0 {
		cpData := make([]byte, cpDataSize)
		_, _ = rand.Read(cpData)
		if state.Checkpoint, err = state.BuildNextCheckpoint(id, priv, cpData); err != nil {
			t.Fatal(err)
		}
	}
	set, err := state.OperationSet(id)
	if err != nil {
		t.Fatal(err)
	}
	addOp := func(state *sobject.SOState) {
		opData := make([]byte, opDataSize)
		_, _ = rand.Read(opData)
		op, err := sobject.BuildSOOperation(id, priv, opData, state.NextOperationLink(set, pid.String()), sobject.NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.AddOperation(id, op); err != nil {
			t.Fatal(err)
		}
		if _, err := set.Add(op); err != nil {
			t.Fatal(err)
		}
	}
	for range held {
		addOp(state)
	}
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Store it at the host key; the first mount converts it to the layout.
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	vol, store, release := openMeasureStore(ctx, t, le, conf)
	defer release()
	err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, SobjectObjectStoreHostStateKey(id), data)
	})
	if err != nil {
		t.Fatal(err)
	}
	mount := func() (sobject.SOStateLockFunc, context.CancelFunc) {
		mctx, cancel := context.WithCancel(ctx)
		_, lockFn, _ := NewObjectStoreSOStateFuncs(mctx, store, "", &SOStateRuns{Reload: func() {}})
		return lockFn, cancel
	}
	lockFn, cancel := mount()
	defer cancel()
	lk, err := lockFn(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	lk.Release()
	db := volume_s4db.GetDB(vol)
	before, err := db.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Add n ops, one write each, as AddLocalOperation does.
	writeStart := time.Now()
	for range n {
		lk, err := lockFn(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		next := lk.GetSOState().Clone()
		addOp(next)
		if err := lk.WriteSOState(ctx, next); err != nil {
			t.Fatal(err)
		}
		lk.Release()
	}
	write := time.Since(writeStart) / time.Duration(n)
	after, err := db.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	// Time a fresh mount's load.
	lockFn, cancel = mount()
	defer cancel()
	start := time.Now()
	lk, err = lockFn(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	load := time.Since(start)
	lk.Release()

	loadavg, _ := os.ReadFile("/proc/loadavg")
	t.Logf(
		"split=%v held=%d ops of %d bytes, checkpoint data %d bytes, %d writes: state %d bytes; per op: write %v, file written %d bytes, commit user bytes %d, flushes %.2f; load %v (loadavg %s)",
		split, held, opDataSize, cpDataSize, n, len(data), write,
		(after.Written-before.Written)/int64(n),
		(after.UserBytes-before.UserBytes)/int64(n),
		float64(after.Flushes-before.Flushes)/float64(n),
		load, loadavg,
	)
}
