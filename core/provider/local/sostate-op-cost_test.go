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
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// MeasureStateWriteEnv names the environment variable that, set to 1, runs
// TestMeasureOpWriteCost.
const MeasureStateWriteEnv = "SPACEWAVE_MEASURE_STATE_WRITE"

// TestMeasureOpWriteCost measures what one operation costs through the
// mounted state lock on the desktop's on-disk volume, in each layout, at
// three counts of operations above the checkpoint: the bytes s4db wrote to the
// file per op, including its own flushes, merges and cleaning, and the time of
// a fresh mount's load. Runs only when MeasureStateWriteEnv is 1.
func TestMeasureOpWriteCost(t *testing.T) {
	if os.Getenv(MeasureStateWriteEnv) != "1" {
		t.Skip("set " + MeasureStateWriteEnv + "=1 to measure")
	}
	for _, split := range []bool{false, true} {
		for _, c := range []struct{ held, n int }{{32, 40}, {1000, 40}, {10000, 10}} {
			measureOpWriteCost(t, split, c.held, 200, c.n)
		}
	}
}

// measureOpWriteCost adds n ops of opDataSize bytes through the state lock to
// a stored state already holding held ops, and logs the cost per op.
func measureOpWriteCost(t *testing.T, split bool, held, opDataSize, n int) {
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
		"split=%v held=%d ops of %d bytes, %d writes: state %d bytes; per op: file written %d bytes, commit user bytes %d, flushes %.2f; load %v (loadavg %s)",
		split, held, opDataSize, n, len(data),
		(after.Written-before.Written)/int64(n),
		(after.UserBytes-before.UserBytes)/int64(n),
		float64(after.Flushes-before.Flushes)/float64(n),
		load, loadavg,
	)
}
