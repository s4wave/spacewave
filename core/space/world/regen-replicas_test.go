package space_world_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	bquad "github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/testbed"
)

// TestRegenerateWorldReplicasWhileWriting regenerates a two-replica Space's
// World and switches it back while a writer on each device keeps writing.
// Every object and quad either device wrote, and the quad the regeneration
// carried, survive on both replicas, and the replicas reach the same root.
func TestRegenerateWorldReplicasWhileWriting(t *testing.T) {
	// Start a testbed with the local provider and the Space World engine.
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_world.NewEngineFactory(tb.Bus, space_world.OpTypes{}))
	_, providerController, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local", PeerId: tb.Volume.GetPeerID().String(), StorageId: tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerController.Release)
	rawProvider, providerRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	local := rawProvider.(*provider_local.Provider)

	// The owner creates the Space and mounts its replica.
	owner, ownerSession := newReplicaSession(ctx, t, local)
	writer, writerSession := newReplicaSession(ctx, t, local)
	ownerRef, err := owner.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ownerObject, releaseOwnerObject, err := owner.MountSharedObject(ctx, ownerRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOwnerObject)
	ownerEngine, ownerServed := startReplicaEngine(ctx, t, tb, ownerRef, "regen-owner")

	// Seed the kv World before the second device joins.
	carried := world.NewGraphQuadWithKeys("seed-0", "<carried>", "seed-1", "")
	var keys []string
	var quads []world.GraphQuad
	for i := range 4 {
		key := fmt.Sprintf("seed-%d", i)
		quad := world.NewGraphQuadWithKeys(key, "<seed>", "seed-0", "")
		if err := writeObjectQuad(ctx, ownerEngine, key, quad); err != nil {
			t.Fatal(err)
		}
		keys, quads = append(keys, key), append(quads, quad)
	}

	// Invite the second device as a writer and mount its replica, with the
	// seeded World's blocks.
	host := ownerObject.(sobject.InviteHost)
	invite, err := host.CreateSOInviteOp(ctx, host.GetPrivKey(), "local", &sobject.SOInvite{
		Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, TargetPeerId: writerSession.GetPeerId().String(), MaxUses: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PrepareDirectInvite(ctx, ownerSession.GetPrivKey(), host.GetPrivKey(), invite); err != nil {
		t.Fatal(err)
	}
	joined, err := writer.JoinViaInvite(ctx, writerSession.GetPrivKey(), invite, "")
	if err != nil {
		t.Fatal(err)
	}
	writerRef := sobject.NewSharedObjectRef("local", writer.GetAccountID(), joined.SharedObjectID, provider_local.SobjectBlockStoreID(joined.SharedObjectID))
	writerObject, releaseWriterObject, err := writer.MountSharedObject(ctx, writerRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseWriterObject)
	snapshot, err := ownerObject.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seeded, releaseSeeded, err := space_world.ReplayWorld(ctx, tb.Logger, tb.Bus, tb.StepFactorySet, ownerObject, "", space_world.OpTypes{}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSeeded()
	if err := block.CopyGraph(ctx, ownerObject.GetBlockStore(), writerObject.GetBlockStore(), seeded.GetHeadRef().GetRootRef(), nil); err != nil {
		t.Fatal(err)
	}
	writerEngine, writerServed := startReplicaEngine(ctx, t, tb, writerRef, "regen-writer")
	waitReplicas(ctx, t, []world.Engine{writerEngine}, keys, quads)

	// Write the carried quad's list on the owner, under a stage that holds it
	// until the regeneration returns, and copy it to the writer's store.
	list := writeHeldQuadList(ctx, t, ownerObject.GetBlockStore(), ownerEngine, carried)
	if err := block.CopyGraph(ctx, ownerObject.GetBlockStore(), writerObject.GetBlockStore(), list, nil); err != nil {
		t.Fatal(err)
	}

	// A writer on each device writes an object and a quad at a time until
	// stopped, recording each write its replica accepted. A write a
	// checkpoint covered before replay placed it ends with its durability
	// unknown, and the SharedObject holds it, so it counts as written.
	var mu sync.Mutex
	var written []string
	var writtenQuads []world.GraphQuad
	counts := map[string]int{}
	stop := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, w := range []struct {
		name string
		eng  world.Engine
	}{{"owner", ownerEngine}, {"writer", writerEngine}} {
		wg.Go(func() {
			for i := 0; ; i++ {
				select {
				case <-stop:
					errs <- nil
					return
				default:
				}
				key := fmt.Sprintf("%s-%d", w.name, i)
				quad := world.NewGraphQuadWithKeys(key, "<written>", "seed-1", "")
				if err := writeObjectQuad(ctx, w.eng, key, quad); err != nil && !errors.Is(err, world.ErrDurabilityUnknown) {
					errs <- fmt.Errorf("%s write %d: %w", w.name, i, err)
					return
				}
				mu.Lock()
				written, writtenQuads = append(written, key), append(writtenQuads, quad)
				counts[w.name]++
				mu.Unlock()
			}
		})
	}
	stopWriters := func() {
		close(stop)
		wg.Wait()
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
	}

	// Regenerate, then switch back, each after both writers have written
	// more and while they keep writing.
	waitWrites := func(n int) {
		t.Helper()
		for {
			mu.Lock()
			done := counts["owner"] >= n && counts["writer"] >= n
			mu.Unlock()
			if done {
				return
			}
			select {
			case <-ctx.Done():
				stopWriters()
				t.Fatal(ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	// Regenerate, switch back, and regenerate again, each step after both
	// writers have written more. A step's device asks once its replica shows
	// the step before, as a caller that reads its World before asking would:
	// a mark the device before writes after its step returns follows the step
	// in the order. A device that asked before its replica applied the step
	// before would be concurrent with it, and refused when ordered first.
	type step struct {
		device string
		target world_block.WorldFormat
		quads  *block.BlockRef
	}
	devices := map[string]struct {
		eng    world.Engine
		served *space_world.Engine
		object sobject.SharedObject
	}{
		"owner":  {ownerEngine, ownerServed, ownerObject},
		"writer": {writerEngine, writerServed, writerObject},
	}
	covers := map[string]uint64{}
	var stepsAt []int
	prev := ""
	for i, st := range []step{
		{"owner", world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH, list},
		{"writer", world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV, nil},
		{"owner", world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH, nil},
	} {
		mu.Lock()
		at := counts["owner"] + counts["writer"]
		mu.Unlock()
		waitWrites(at/2 + 3)
		if prev != "" {
			key := fmt.Sprintf("step-%d", i-1)
			mark := world.NewGraphQuadWithKeys(key, "<mark>", "seed-0", "")
			if err := writeObjectQuad(ctx, devices[prev].eng, key, mark); err != nil {
				stopWriters()
				t.Fatal(err)
			}
			keys, quads = append(keys, key), append(quads, mark)
			waitReplicas(ctx, t, []world.Engine{devices[st.device].eng}, []string{key}, nil)
		}
		dev := devices[st.device]
		if err := dev.served.RegenerateWorld(ctx, st.target, st.quads); err != nil {
			stopWriters()
			t.Fatalf("step %d: %v", i, err)
		}
		covers[dev.object.GetPeerID().String()] = authorHead(ctx, t, dev.object)
		mu.Lock()
		stepsAt = append(stepsAt, counts["owner"]+counts["writer"])
		mu.Unlock()
		prev = st.device
	}

	// Write enough after the last step for a checkpoint past it.
	waitWrites(stepsAt[len(stepsAt)-1]/2 + sobject.MinCheckpointOperations)
	stopWriters()

	// Both replicas hold every object and quad, and the carried quad.
	keys, quads = append(keys, written...), append(append(quads, writtenQuads...), carried)
	waitReplicas(ctx, t, []world.Engine{ownerEngine, writerEngine}, keys, quads)
	t.Logf("writes: owner %d, writer %d; the steps returned after %v", counts["owner"], counts["writer"], stepsAt)

	// Both replicas hold one checkpoint past every step.
	var agreed *sobject.SOCheckpointInner
	for {
		owned, written := coveringCheckpoint(ctx, t, ownerObject, covers), coveringCheckpoint(ctx, t, writerObject, covers)
		if owned != nil && owned.EqualVT(written) {
			agreed = owned
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no checkpoint past every step on both replicas: %v, %v", owned.GetHeight(), written.GetHeight())
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Logf("both replicas hold checkpoint %d", agreed.GetHeight())

	// The replicas reach the same root at the same seqno.
	for {
		ownerRoot, ownerSeqno, err := ownerServed.GetHead(ctx)
		if err != nil {
			t.Fatal(err)
		}
		writerRoot, writerSeqno, err := writerServed.GetHead(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ownerSeqno == writerSeqno && ownerRoot.EqualVT(writerRoot) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("the replicas hold %s at %d and %s at %d", ownerRoot.MarshalString(), ownerSeqno, writerRoot.MarshalString(), writerSeqno)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// newReplicaSession mounts an account of its own on local and its Session.
func newReplicaSession(ctx context.Context, t *testing.T, local *provider_local.Provider) (*provider_local.ProviderAccount, session.Session) {
	t.Helper()
	ref, err := local.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, releaseAccount, err := local.AccessProviderAccount(ctx, ref.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAccount)
	account := raw.(*provider_local.ProviderAccount)
	sess, releaseSession, err := account.MountSession(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSession)
	t.Cleanup(account.StopSessionTransport)
	t.Cleanup(account.StopP2PSync)
	return account, sess
}

// startReplicaEngine starts the Space World engine id on the replica ref and
// returns it with the engine it serves.
func startReplicaEngine(ctx context.Context, t *testing.T, tb *testbed.Testbed, ref *sobject.SharedObjectRef, id string) (world.Engine, *space_world.Engine) {
	t.Helper()
	eng, release, err := world.StartEngine(ctx, tb.Bus, space_world.NewConfig(id, ref), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release.Release)
	var served *space_world.Engine
	if err := world.AccessEngine(ctx, eng, func(_ context.Context, eng world.Engine) error {
		served = eng.(*space_world.Engine)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return eng, served
}

// writeObjectQuad creates key and sets quad in one transaction.
func writeObjectQuad(ctx context.Context, eng world.Engine, key string, quad world.GraphQuad) error {
	return world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return ws.SetGraphQuad(ctx, quad)
	})
}

// writeHeldQuadList writes quad as a quad list in store, in the World's
// encoding, under a stage held until the test ends, and returns its root.
func writeHeldQuadList(ctx context.Context, t *testing.T, store block.StoreOps, eng world.Engine, quad world.GraphQuad) *block.BlockRef {
	t.Helper()
	stage, release, err := block.OpenStage(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	storage, err := eng.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Release()
	btx, bcs := storage.BuildTransactionWithStore(nil, stage)
	if err := space_world.WriteQuadList(ctx, bcs, func(cb func(*bquad.Quad) error) error {
		return cb(world.GraphQuadToQuad(quad))
	}); err != nil {
		t.Fatal(err)
	}
	list, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// waitReplicas waits until every engine holds every key and quad.
func waitReplicas(ctx context.Context, t *testing.T, engines []world.Engine, keys []string, quads []world.GraphQuad) {
	t.Helper()
	for _, eng := range engines {
		for {
			err := hasObjects(ctx, eng, keys...)
			if err == nil {
				err = hasQuads(ctx, eng, quads)
			}
			if err == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(err)
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
}

// hasQuads reports an error unless eng holds every quad.
func hasQuads(ctx context.Context, eng world.Engine, quads []world.GraphQuad) error {
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	for _, q := range quads {
		found, err := tx.LookupGraphQuads(ctx, q, 1)
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return fmt.Errorf("quad %s %s %s is missing", q.GetSubject(), q.GetPredicate(), q.GetObj())
		}
	}
	return nil
}

// authorHead returns the nonce of the latest operation object's own device
// wrote in its operation set.
func authorHead(ctx context.Context, t *testing.T, object sobject.SharedObject) uint64 {
	t.Helper()
	snap, err := object.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nonce, _ := set.AuthorHead(object.GetPeerID().String())
	return nonce
}

// coveringCheckpoint returns object's checkpoint when it covers each
// author's operation at nonce, or nil.
func coveringCheckpoint(ctx context.Context, t *testing.T, object sobject.SharedObject, covers map[string]uint64) *sobject.SOCheckpointInner {
	t.Helper()
	snap, err := object.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for author, nonce := range covers {
		covered := false
		for _, pos := range checkpoint.GetAuthors() {
			covered = covered || pos.GetPeerId() == author && pos.GetNonce() >= nonce
		}
		if !covered {
			return nil
		}
	}
	return checkpoint
}
