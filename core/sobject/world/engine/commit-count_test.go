//go:build !js

package sobject_world_engine_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/core/metrics"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// localWorld is the World engine of a SharedObject in a local provider
// account whose volume is an s4db database.
type localWorld struct {
	// bus is the testbed bus the SharedObject mounts on.
	bus bus.Bus
	// le is the testbed logger the engine logs to.
	le *logrus.Entry
	// eng is the World engine.
	eng sobject_world_engine.Engine
	// acc is the provider account holding the SharedObject.
	acc *provider_local.ProviderAccount
	// db is the account volume's database.
	db *s4db.DB
	// vol is the account volume.
	vol volume.Volume
	// soRef identifies the SharedObject.
	soRef *sobject.SharedObjectRef
}

// startLocalWorld starts a local provider on s4db storage under dir and the
// World engine of the SharedObject soRef in its account, creating the
// SharedObject when soRef is nil. Test cleanup stops them.
func startLocalWorld(t *testing.T, dir string, soRef *sobject.SharedObjectRef) *localWorld {
	// Open the account and its SharedObject.
	t.Helper()
	ctx := t.Context()
	tb, acc, soRef := openLocalAccount(t, dir, soRef)

	// Start its World engine with the mock operations.
	engineID := "local-world-engine"
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("local-world-ops", engineID, world_mock.LookupMockOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(relOpc)
	eng, engRef, err := world.StartEngine(ctx, tb.Bus, sobject_world_engine.NewConfig(engineID, soRef), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engRef.Release)

	// Find the account database.
	return newLocalWorld(t, tb, acc, eng, soRef)
}

// openLocalAccount starts a local provider on s4db storage under dir and
// opens the SharedObject soRef in its account, creating it when soRef is nil.
// Test cleanup stops them.
func openLocalAccount(t *testing.T, dir string, soRef *sobject.SharedObjectRef) (*testbed.Testbed, provider.ProviderAccount, *sobject.SharedObjectRef) {
	t.Helper()
	return openLocalAccountOn(t, storage_native.NewS4db(false, dir), soRef)
}

// openLocalAccountOn opens the account as openLocalAccount does, on st.
func openLocalAccountOn(t *testing.T, st storage.Storage, soRef *sobject.SharedObjectRef) (*testbed.Testbed, provider.ProviderAccount, *sobject.SharedObjectRef) {
	// Start a testbed on the storage.
	t.Helper()
	ctx := t.Context()
	tb, err := testbed.Default(ctx, testbed.WithStorages(st))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Run the local provider and the World engine factory on it.
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)

	// Open the account and its SharedObject.
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, "local", "local-world-account", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRef.Release)
	if soRef == nil {
		soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
		if err != nil {
			t.Fatal(err)
		}
		soRef, err = soProv.CreateSharedObject(ctx, "local-world", &sobject.SharedObjectMeta{BodyType: "local-world"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	return tb, acc, soRef
}

// newLocalWorld returns the World eng of the SharedObject soRef in the local
// account acc on tb.
func newLocalWorld(t *testing.T, tb *testbed.Testbed, acc provider.ProviderAccount, eng sobject_world_engine.Engine, soRef *sobject.SharedObjectRef) *localWorld {
	// Find the account volume and its database.
	t.Helper()
	local := acc.(*provider_local.ProviderAccount)
	vol := local.GetVolume()
	db := volume_s4db.GetDB(vol)
	if db == nil {
		t.Fatal("account volume is not s4db")
	}
	return &localWorld{bus: tb.Bus, le: tb.Logger, eng: eng, acc: local, db: db, vol: vol, soRef: soRef}
}

// create commits a World write that creates the object key.
func (w *localWorld) create(t *testing.T, key string) {
	// Build the write and commit it.
	t.Helper()
	tx := w.newWrite(t, key)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// createWithPayload commits a World write that creates the object key with
// a root block it puts, the operation's payload.
func (w *localWorld) createWithPayload(t *testing.T, key string) {
	// Put the root block through the write.
	t.Helper()
	ctx := t.Context()
	tx, err := w.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var oref *bucket.ObjectRef
	err = tx.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		// Encode the block and put it as the object's root.
		data, err := block_mock.NewExample(key).MarshalVT()
		if err != nil {
			return err
		}
		oref = bls.GetRef().Clone()
		oref.RootRef, _, err = bls.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create the object at it and commit.
	obj, err := tx.CreateObject(ctx, key, oref)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// newWrite returns an uncommitted World write that creates the object key.
func (w *localWorld) newWrite(t *testing.T, key string) world.Tx {
	// Open a write and create the object in it.
	t.Helper()
	tx, err := w.eng.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := tx.CreateObject(t.Context(), key, &bucket.ObjectRef{BucketId: "local-world-bucket"})
	world.ReleaseObjectState(obj)
	if err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	return tx
}

// seq returns the sequence of the last database commit.
func (w *localWorld) seq() uint64 {
	return w.db.Seq()
}

// TestLocalWorldCommitPhysicalCommits checks that one local SharedObject World
// commit is one database commit: the blocks, the operation, the
// accepted World and the replay save publish together.
func TestLocalWorldCommitPhysicalCommits(t *testing.T) {
	// Commit one object per transaction, after a warm-up commit.
	w := startLocalWorld(t, t.TempDir(), nil)
	w.create(t, "commit-count/0")
	const n = 8
	before := w.seq()
	for i := 1; i <= n; i++ {
		w.create(t, "commit-count/"+strconv.Itoa(i))
	}
	if got := w.seq() - before; got != n {
		t.Fatalf("%d World commits made %d physical commits", n, got)
	}
}

// TestCheckpointInstallPhysicalCommits checks that a checkpoint and its
// install are four database commits: the publication storing the checkpoint
// World, the SharedObject state, the reader pin the engine took on a kept page
// moving to the store, then the hold of the checkpoint World, the replay span
// and the replay save together, and that the checkpoint composes what the writes
// replaced.
func TestCheckpointInstallPhysicalCommits(t *testing.T) {
	// Write one operation short of a checkpoint, the first with a payload, and
	// let the engine settle.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	flows := scrapeWriteFlows(t)
	w.createWithPayload(t, "checkpoint/payload")
	var n int
	for ; admittedOps(t, so) < sobject.MinCheckpointOperations-1; n++ {
		w.create(t, "checkpoint/"+strconv.Itoa(n))
	}
	waitSeqIdle(t, w)

	// Write the operation that reaches it, and wait for the checkpoint.
	w.create(t, "checkpoint/"+strconv.Itoa(n))
	written := w.seq()
	durable, relDurable, err := so.AccessSharedObjectState(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relDurable()
	snap, err := durable.WaitValueWithValidator(t.Context(), func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		return checkpointTrimmed(t.Context(), snap)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The checkpoint and its install commit four times.
	if got := waitSeqIdle(t, w) - written; got != 4 {
		t.Fatalf("checkpoint and install made %d physical commits, want 4", got)
	}

	// The metrics count every commit, each kind of byte, and the checkpoint.
	after := scrapeWriteFlows(t)
	if got := after["spacewave_world_commits_total"] - flows["spacewave_world_commits_total"]; got < float64(n+2) {
		t.Fatalf("counted %v commits, want at least %d", got, n+2)
	}
	for _, kind := range []string{"world", "payload", "state", "replay"} {
		key := "spacewave_world_commit_bytes_total{kind=" + kind + "}"
		if after[key] <= flows[key] {
			t.Fatalf("counted no %s bytes", kind)
		}
	}
	if after["spacewave_world_checkpoints_total"] <= flows["spacewave_world_checkpoints_total"] {
		t.Fatal("counted no checkpoint")
	}
	if _, ok := after["spacewave_world_checkpoint_skips_total{reason=pending}"]; !ok {
		t.Fatal("no checkpoint skip series")
	}

	// The checkpoint composed the pages the writes replaced, without a
	// forfeit: the change log is off.
	if after["spacewave_world_deaths_total"] <= flows["spacewave_world_deaths_total"] {
		t.Fatalf("composed no deaths, forfeited %v", after["spacewave_world_death_forfeits_total"]-flows["spacewave_world_death_forfeits_total"])
	}
	if got := after["spacewave_world_death_forfeits_total"] - flows["spacewave_world_death_forfeits_total"]; got != 0 {
		t.Fatalf("forfeited %v intervals", got)
	}

	// The checkpoint named a record of those deaths, and its World reaches
	// none of them.
	checkpoint, err := snap.GetCheckpoint(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state := &sobject_world_engine.InnerState{}
	if err := state.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		t.Fatal(err)
	}

	// Gather the pages the checkpoint World reaches.
	live := make(map[string]struct{})
	err = block.WalkGraph(t.Context(), so.GetBlockStore(), []*block.BlockRef{state.GetHeadRef().GetRootRef()}, 0, func(ref *block.BlockRef, _ *block.StoredBlock) error {
		live[ref.MarshalString()] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check every named record against them.
	records := w.deadlists(t, so.GetBlockStore())
	if len(records) == 0 {
		t.Fatal("named no deadlist record")
	}
	for _, rec := range records {
		for _, ref := range slices.Concat(rec.dead.Replaced, rec.dead.Removed) {
			if _, ok := live[ref.MarshalString()]; ok {
				t.Fatalf("deadlist lists %s, which the checkpoint World reaches", ref.MarshalString())
			}
		}
	}
}

// deadlistRecord is one deadlist record a volume names for a World.
type deadlistRecord struct {
	// name is the record's named root.
	name string
	// head is the record's first block, which names the World it runs to.
	head *block.Deadlist
	// dead is what the record lists, gathered from every part.
	dead *block.Replacements
}

// deadlists reads every deadlist record the volume names for the World, due
// or pending, from store. A sequence that names no record, such as a forfeited
// interval or a released record, is skipped; the scan ends past the ledger's
// bound of pending records with none named.
func (w *localWorld) deadlists(t *testing.T, store block.StoreOps) []deadlistRecord {
	// Find the bucket's named roots in the volume's graph.
	t.Helper()
	graph := w.acc.GetVolume().(kvtx_volume.KvtxVolume).GetRefGraph()
	bucketID := provider_local.BlockStoreBucketID(w.acc.GetProviderID(), w.acc.GetAccountID(), w.soRef.GetBlockStoreId())

	// Read each record by sequence.
	var out []deadlistRecord
	for seq, empty := uint64(1), 0; empty <= 64; seq++ {
		var name string
		var refs []string
		for _, n := range []string{block.DeadlistRootName(seq), block.DeadlistDueRootName(seq)} {
			named, err := graph.GetOutgoingRefs(t.Context(), kvtx_volume.RootOwnerIRI(bucketID, n))
			if err != nil {
				t.Fatal(err)
			}
			if len(named) != 0 && name == "" {
				name = n
			}
			refs = append(refs, named...)
		}
		if len(refs) == 0 {
			empty++
			continue
		}
		empty = 0
		ref, ok := block_gc.ParseBlockIRI(refs[0])
		if !ok {
			t.Fatalf("deadlist root names %s", refs[0])
		}
		head, dead, err := block.ReadDeadlist(t.Context(), store, ref)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, deadlistRecord{name: name, head: head, dead: dead})
	}
	return out
}

// scrapeWriteFlows gathers the process metrics and returns the World write
// counters, keyed by name and any kind or reason label.
func scrapeWriteFlows(t *testing.T) map[string]float64 {
	// Gather the families.
	t.Helper()
	families, err := metrics.Gather()
	if err != nil {
		t.Fatal(err)
	}

	// Keep the World series.
	values := make(map[string]float64)
	for _, f := range families {
		if !strings.HasPrefix(f.GetName(), "spacewave_world_") {
			continue
		}
		for _, m := range f.GetMetric() {
			key := f.GetName()
			for _, l := range m.GetLabel() {
				key += "{" + l.GetName() + "=" + l.GetValue() + "}"
			}
			values[key] = m.GetCounter().GetValue()
		}
	}
	return values
}

// waitSeqIdle waits until the database makes no commit for 300 ms and returns
// its last sequence.
func waitSeqIdle(t *testing.T, w *localWorld) uint64 {
	// Poll the sequence until it holds still.
	t.Helper()
	last, since := w.seq(), time.Now()
	deadline := since.Add(10 * time.Second)
	for time.Since(since) < 300*time.Millisecond {
		if time.Now().After(deadline) {
			t.Fatal("database kept committing")
		}
		time.Sleep(10 * time.Millisecond)
		if seq := w.seq(); seq != last {
			last, since = seq, time.Now()
		}
	}
	return last
}
