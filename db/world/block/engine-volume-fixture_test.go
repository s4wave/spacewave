//go:build !js && !wasip1

package world_block

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// fixtureRootName is the World root of the volume fixture.
const fixtureRootName = "world:fixture"

// volumeFixture is a World engine over the local authority on a real s4db
// Volume whose collector frees with a grace period of a nanosecond, so a
// test can collect between any two steps.
type volumeFixture struct {
	// le is the logger.
	le *logrus.Entry
	// tb is the testbed.
	tb *testbed.Testbed
	// volume is the s4db Volume.
	volume *volume_kvtx.Volume
	// base is the empty cursor on the World's bucket.
	base *bucket_lookup.Cursor
	// store is the bucket store that holds the World root.
	store block.StoreOps
	// coordinator is the Volume's coordinator.
	coordinator *losingCoordinator
	// engine is the World engine.
	engine *Engine
}

// newVolumeFixture opens the fixture.
func newVolumeFixture(t *testing.T) *volumeFixture {
	// Open a quiet testbed on an s4db Volume that frees at once.
	t.Helper()
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_s4db.Config{
		Path:         filepath.Join(t.TempDir(), "world.s4wave"),
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	f := &volumeFixture{le: le, tb: tb}
	f.volume, _ = tb.Volume.(*volume_kvtx.Volume)
	if f.volume == nil || volume_s4db.GetDB(tb.Volume) == nil {
		t.Fatal("test requires a native s4db Volume")
	}

	// Build the World's cursor and open the engine on its authority.
	f.base, err = tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.base.Release)
	f.store = f.base.GetBucket()
	f.coordinator = &losingCoordinator{Coordinator: tb.Volume}
	f.engine = f.openEngine(t, f.authority())
	return f
}

// authority returns a new local authority on the World root through the
// Volume's coordinator, as another process sharing the Volume would hold.
func (f *volumeFixture) authority() *LocalAuthority {
	scope := coord.Scope{VolumeID: f.tb.Volume.GetID(), ObjectStoreID: fixtureRootName, ParticipantID: "fixture"}
	return NewLocalAuthority(f.le, f.base, fixtureRootName, f.coordinator, scope)
}

// openEngine opens an engine over authority at its head, closed when the
// test ends. It pins a replaced head a reader holds before the commit returns,
// since the Volume's grace is shorter than any reader, unless opts set the
// delay.
func (f *volumeFixture) openEngine(t *testing.T, authority world.Authority, opts ...EngineOption) *Engine {
	// Read the head and open the engine at it.
	t.Helper()
	ctx := t.Context()
	head, err := authority.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root := f.base.Clone()
	t.Cleanup(root.Release)
	root.SetRootRef(head.Root)
	opts = append([]EngineOption{WithReaderPinDelay(0)}, opts...)
	engine, err := NewEngine(ctx, f.le, root, world_mock.LookupMockOp, authority, false, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
	return engine
}

// seq returns the s4db database's commit sequence: one per Volume
// transaction.
func (f *volumeFixture) seq() uint64 {
	return volume_s4db.GetDB(f.tb.Volume).Seq()
}

// collect runs collector passes until one leaves no work.
func (f *volumeFixture) collect(t *testing.T) collectSum {
	// Collect until the pass reports nothing left.
	t.Helper()
	var res collectSum
	for range 64 {
		r, err := f.volume.Collect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		res.freed += r.Freed
		if !r.More {
			return res
		}
	}
	t.Fatal("collector did not finish")
	return res
}

// collectSum sums the collector passes of one collect.
type collectSum struct {
	// freed counts the blocks the passes freed.
	freed uint64
}

// stored reports whether the Volume holds the bytes of ref.
func (f *volumeFixture) stored(t *testing.T, ref *block.BlockRef) bool {
	t.Helper()
	found, err := f.store.GetBlockExists(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// worldRoot returns the block the World root holds.
func (f *volumeFixture) worldRoot(t *testing.T) *block.BlockRef {
	t.Helper()
	ref, err := f.store.GetRoot(t.Context(), fixtureRootName)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// fixtureWriter opens a write transaction on engine, discarded when the test
// ends.
func fixtureWriter(t *testing.T, engine *Engine) *EngineTx {
	// Open the transaction with a bound on the wait for the turn.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	w, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Discard)
	return w
}

// fixturePayload writes a payload block through w's storage and returns its
// reference in the World's bucket.
func fixturePayload(t *testing.T, w *EngineTx, data string) *bucket.ObjectRef {
	// Write the payload through the transaction's store.
	t.Helper()
	c, err := w.BuildStorageCursor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	btx, bcs := c.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExample(data), true)
	ref, _, err := btx.Write(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	next := c.GetRef().Clone()
	next.RootRef = ref
	return next
}

// fixtureObject writes a payload and creates or updates key at it.
func fixtureObject(t *testing.T, w *EngineTx, key, data string) *bucket.ObjectRef {
	// Write the payload.
	t.Helper()
	ref := fixturePayload(t, w, data)

	// Point the object at it.
	ctx := t.Context()
	obj, found, err := w.GetObject(ctx, key)
	if err == nil && found {
		_, err = obj.SetRootRef(ctx, ref)
	} else if err == nil {
		obj, err = w.CreateObject(ctx, key, ref)
	}
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// fixtureHas fails unless the World state's object key exists as want says.
func fixtureHas(t *testing.T, ws world.WorldState, key string, want bool) {
	t.Helper()
	got, err := ws.HasObject(t.Context(), key)
	if err != nil || got != want {
		t.Fatalf("HasObject(%s)=%v,%v want %v", key, got, err, want)
	}
}

// fixtureKeys returns n distinct object keys with prefix.
func fixtureKeys(prefix string, n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = prefix + strconv.Itoa(i)
	}
	return keys
}

// losingCoordinator is a coordinator whose test can lose the write lease a
// writer holds, as a lease that lapses mid-commit is lost.
type losingCoordinator struct {
	coord.Coordinator
	// mtx guards held.
	mtx sync.Mutex
	// held is the last lease acquired.
	held coord.WriteLease
}

// WaitAcquireWriteLease acquires the lease and records it.
func (c *losingCoordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	lease, err := c.Coordinator.WaitAcquireWriteLease(ctx, scope)
	if err == nil {
		c.mtx.Lock()
		c.held = lease
		c.mtx.Unlock()
	}
	return lease, err
}

// lose releases the last lease acquired out from under its holder.
func (c *losingCoordinator) lose(t *testing.T) {
	t.Helper()
	c.mtx.Lock()
	lease := c.held
	c.mtx.Unlock()
	if lease == nil {
		t.Fatal("no lease to lose")
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}
