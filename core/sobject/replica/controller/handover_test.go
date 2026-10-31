//go:build !tinygo

package sobject_replica_controller_test

import (
	"context"
	"crypto/sha256"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// The shape of the large World: rounds of writes, each followed by a block
// pass, with payloads large enough to chunk, then a churn of one-object
// commits long enough that the payloads the rounds replaced become garbage.
const (
	// fixtureRounds is the number of write rounds, one block pass after each.
	fixtureRounds = 12
	// fixtureCreates is the number of objects each round creates.
	fixtureCreates = 24
	// fixtureMaxPayload is the largest round payload, in bytes.
	fixtureMaxPayload = 128 << 10
	// fixtureChurn is the number of one-object commits after the rounds. The
	// World's changelog keeps up to two segments of 1024 commits, each change
	// naming the object it replaced, so a replaced payload stays reachable
	// until more commits than that follow it.
	fixtureChurn = 2200
	// fixtureChurnPayload is the largest churn payload, in bytes.
	fixtureChurnPayload = 4 << 10
	// fixtureChurnPass is the number of churn commits between block passes.
	fixtureChurnPass = 100
)

// TestReplicaHandsLargeWorldOverAndBack hands a World written across many
// block passes over to a second Device and back, with block passes running
// throughout. Each side restarts its Session transport in each ownership
// state. No read finds a missing block, and no departed owner's write reaches
// the successor.
func TestReplicaHandsLargeWorldOverAndBack(t *testing.T) {
	// Bound the run and count every logged missing block.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	tb := startTestbed(ctx, t)
	missing := countMissingBlocks(tb.Logger.Logger)

	// A owns a Space holding the large World.
	a, aSession := setupAccount(ctx, t, tb)
	b, bSession := setupAccount(ctx, t, tb)
	aPeer, bPeer := aSession.GetPeerId().String(), bSession.GetPeerId().String()
	ref, err := a.CreateSharedObject(ctx, "replica-space", mustSpaceMeta(t), "", "")
	if err != nil {
		t.Fatal(err)
	}
	id := ref.GetProviderResourceRef().GetId()
	fix := buildLargeWorld(ctx, t, tb.Bus, a, ref)

	// Move to B, restarting both sides while A owns and while A hands over,
	// with block passes on both volumes throughout.
	graph := startFlowgraph(ctx, t, tb, aPeer, bPeer)
	stopGC := collectLoop(ctx, t, a.GetVolume(), b.GetVolume())
	placePair(ctx, t, graph, id, "b")
	waitListed(ctx, t, b, id)
	restartBoth(ctx, t, a, aSession, b, bSession, "settled, A owns")
	waitOwnership(ctx, t, a, id, func(own *sobject.SOOwnership) bool { return own.Departing(aPeer) })
	restartBoth(ctx, t, b, bSession, a, aSession, "A handing over")
	waitOwner(ctx, t, b, id, bPeer, aPeer)

	// Stop the passes and check B's copy of the World.
	stopGC()
	checkWorld(ctx, t, tb.Bus, b, id, fix)

	// B writes as the owner, and a write on A's departed copy never reaches
	// B's state or World.
	stopWatch := watchDeparted(ctx, t, b, id, aPeer)
	fix.write(ctx, t, tb.Bus, listedRef(b.GetSOListCtr().GetValue(), id), "owner-b")
	tryDepartedWrite(ctx, t, tb.Bus, a, id)
	checkDeparted(t, stopWatch())
	checkWorld(ctx, t, tb.Bus, b, id, fix)

	// Move back, restarting both sides while B owns and while B hands over.
	restartBoth(ctx, t, a, aSession, b, bSession, "settled, B owns")
	stopGC = collectLoop(ctx, t, a.GetVolume(), b.GetVolume())
	placePair(ctx, t, graph, id, "a")
	waitOwnership(ctx, t, b, id, func(own *sobject.SOOwnership) bool { return own.Departing(bPeer) })
	restartBoth(ctx, t, a, aSession, b, bSession, "B handing over")
	waitOwner(ctx, t, a, id, aPeer, bPeer)
	stopGC()

	// A holds the whole World, B's writes included, and nothing read found a
	// missing block.
	checkWorld(ctx, t, tb.Bus, a, id, fix)
	stopWatch = watchDeparted(ctx, t, a, id, bPeer)
	fix.write(ctx, t, tb.Bus, listedRef(a.GetSOListCtr().GetValue(), id), "owner-a")
	checkDeparted(t, stopWatch())
	checkWorld(ctx, t, tb.Bus, a, id, fix)
	if n := missing.Load(); n != 0 {
		t.Fatalf("%d log entries report a missing block", n)
	}
}

// largeWorld is the expected content of the large World: the payload digest
// of each live object and the keys deleted, with the payload roots replaced.
type largeWorld struct {
	// rng draws payload sizes and bytes.
	rng *rand.ChaCha8
	// next is the number of the next object created.
	next int
	// live maps each live object key to its payload digest.
	live map[string][sha256.Size]byte
	// roots maps each live object key to its payload root.
	roots map[string]*block.BlockRef
	// replaced are the payload roots an overwrite replaced.
	replaced []*block.BlockRef
	// deleted are the keys deleted.
	deleted []string
}

// buildLargeWorld writes the large World into the Space ref on acc's volume.
// Each round creates objects, overwrites a quarter of the live ones and
// deletes an eighth, then runs a block pass. The churn that follows overwrites
// one object a commit, with a block pass every fixtureChurnPass commits, until
// the passes sweep the payloads the rounds replaced.
func buildLargeWorld(ctx context.Context, t *testing.T, b bus.Bus, acc *provider_local.ProviderAccount, ref *sobject.SharedObjectRef) *largeWorld {
	// Write in the Space body for the whole fixture.
	t.Helper()
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()
	engine := body.GetSharedObjectBody().GetWorldEngine()

	// Each block pass counts the blocks it removed.
	fix := &largeWorld{
		rng:   rand.NewChaCha8([32]byte{8}),
		live:  make(map[string][sha256.Size]byte),
		roots: make(map[string]*block.BlockRef),
	}
	collector := newCollector(t, acc.GetVolume())
	start := time.Now()
	var passes, removed int
	pass := func() {
		stats, err := collector.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		passes++
		removed += stats.RemoveBlockCount
	}

	// Run the rounds, then the churn.
	for round := range fixtureRounds {
		fix.round(ctx, t, engine, "round-"+strconv.Itoa(round))
		pass()
	}
	keys := fix.liveKeys()
	for i := range fixtureChurn {
		key := keys[i%len(keys)]
		commit(ctx, t, engine, func(tx world.Tx) error { return fix.put(ctx, tx, key, fixtureChurnPayload) })
		if (i+1)%fixtureChurnPass == 0 {
			pass()
		}
	}

	// The passes must have swept replaced payloads, or the store spans no
	// block pass that mattered.
	swept := fix.sweptReplaced(ctx, t, acc.GetVolume())
	t.Logf("large World: %d live objects, %d deleted, %d commits, %d block passes removed %d blocks, %d of %d replaced payload roots swept, in %v",
		len(fix.live), len(fix.deleted), fixtureRounds+fixtureChurn, passes, removed, swept, len(fix.replaced), time.Since(start))
	if swept == 0 {
		t.Fatal("no block pass swept a replaced payload")
	}
	return fix
}

// write runs one round in the Space ref, mounting its body only while
// writing.
func (f *largeWorld) write(ctx context.Context, t *testing.T, b bus.Bus, ref *sobject.SharedObjectRef, label string) {
	// Mount the body and run the round in its World.
	t.Helper()
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()
	f.round(ctx, t, body.GetSharedObjectBody().GetWorldEngine(), label)
}

// round creates objects, overwrites a quarter of the live ones and deletes an
// eighth, in one commit of engine.
func (f *largeWorld) round(ctx context.Context, t *testing.T, engine world.Engine, label string) {
	// Pick the overwrites and the deletes from the live keys.
	t.Helper()
	var overwrite, del []string
	for i, key := range f.liveKeys() {
		switch {
		case i%8 == 3:
			del = append(del, key)
		case i%4 == 1:
			overwrite = append(overwrite, key)
		}
	}

	// Create, overwrite and delete in one commit.
	commit(ctx, t, engine, func(tx world.Tx) error {
		for range fixtureCreates {
			if err := f.put(ctx, tx, "obj/"+strconv.Itoa(f.next), fixtureMaxPayload); err != nil {
				return err
			}
			f.next++
		}
		for _, key := range overwrite {
			if err := f.put(ctx, tx, key, fixtureMaxPayload); err != nil {
				return err
			}
		}
		for _, key := range del {
			if _, err := tx.DeleteObject(ctx, key); err != nil {
				return err
			}
		}
		return nil
	})

	// Record the deletes once committed.
	for _, key := range del {
		delete(f.live, key)
		delete(f.roots, key)
		f.deleted = append(f.deleted, key)
	}
	t.Logf("%s: %d created, %d overwritten, %d deleted", label, fixtureCreates, len(overwrite), len(del))
}

// liveKeys returns the live object keys in order.
func (f *largeWorld) liveKeys() []string {
	keys := make([]string, 0, len(f.live))
	for key := range f.live {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// put writes a fresh payload of at most maxSize bytes as the root of key,
// creating it when absent.
func (f *largeWorld) put(ctx context.Context, tx world.Tx, key string, maxSize uint64) error {
	// Draw the payload.
	data := make([]byte, 1+int(f.rng.Uint64()%maxSize))
	_, _ = f.rng.Read(data)

	// Build it as the object's blob.
	objRef, _, err := world.AccessWorldObject(ctx, tx, key, true, func(bcs *block.Cursor) error {
		_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
		return err
	})
	if err != nil {
		return err
	}

	// Record it, and the root it replaced.
	if prev := f.roots[key]; prev != nil {
		f.replaced = append(f.replaced, prev)
	}
	f.roots[key] = objRef.GetRootRef()
	f.live[key] = sha256.Sum256(data)
	return nil
}

// sweptReplaced returns how many replaced payload roots vol no longer holds.
func (f *largeWorld) sweptReplaced(ctx context.Context, t *testing.T, vol volume.Volume) int {
	t.Helper()
	var swept int
	for _, ref := range f.replaced {
		found, err := vol.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			swept++
		}
	}
	return swept
}

// checkWorld checks that the World of id, as acc lists it, holds every live
// object's payload and none of the deleted keys. Readers see the durable
// World, which may trail the admitted one after a handover, so a World that
// differs is checked again as its seqno moves, until a deadline.
func checkWorld(ctx context.Context, t *testing.T, b bus.Bus, acc *provider_local.ProviderAccount, id string, fix *largeWorld) {
	// Mount the Space body acc lists.
	t.Helper()
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, listedRef(acc.GetSOListCtr().GetValue(), id), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()
	ws := world.NewEngineWorldState(body.GetSharedObjectBody().GetWorldEngine(), false)

	// Check the World until it matches or stops moving toward the fixture.
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		diff := worldDiff(ctx, t, ws, fix)
		if diff == "" {
			break
		}
		if _, err := ws.WaitSeqno(waitCtx, seqno+1); err != nil {
			t.Fatalf("%s at seqno %d: %v", diff, seqno, err)
		}
	}
	t.Logf("World holds all %d live objects", len(fix.live))
}

// worldDiff returns how ws differs from fix: a live object holding another
// payload or a deleted key present, or "" when it matches.
func worldDiff(ctx context.Context, t *testing.T, ws world.WorldState, fix *largeWorld) string {
	// Read every live payload in full.
	t.Helper()
	for key, want := range fix.live {
		var data []byte
		_, _, err := world.AccessWorldObject(ctx, ws, key, false, func(bcs *block.Cursor) error {
			var ferr error
			data, ferr = blob.FetchToBytes(ctx, bcs)
			return ferr
		})
		if err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		if sha256.Sum256(data) != want {
			return key + " holds another payload"
		}
	}

	// No deleted key came back.
	for _, key := range fix.deleted {
		obj, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			return "deleted " + key + " is present"
		}
	}
	return ""
}

// departedKey is the object a departed owner tries to write.
const departedKey = "departed-write"

// tryDepartedWrite tries to write departedKey into acc's departed copy of id.
// The write may fail or land only in that copy; either way it must not reach
// the owner.
func tryDepartedWrite(ctx context.Context, t *testing.T, b bus.Bus, acc *provider_local.ProviderAccount, id string) {
	// Bound the attempt, since a departed copy may never confirm it.
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Write through the copy acc still lists.
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, listedRef(acc.GetSOListCtr().GetValue(), id), false, nil)
	if err != nil {
		t.Logf("departed copy did not mount: %v", err)
		return
	}
	defer bodyRef.Release()
	ws := world.NewEngineWorldState(body.GetSharedObjectBody().GetWorldEngine(), true)
	obj, err := ws.CreateObject(ctx, departedKey, nil)
	world.ReleaseObjectState(obj)
	t.Logf("departed write: %v", err)
}

// departure is the successor's state when it first no longer names the
// departed owner: the checkpoint height and the departed owner's ops still
// pending. Nothing above it may come from the departed owner.
type departure struct {
	// height is the checkpoint height.
	height uint64
	// ops are the keys of the departed owner's pending ops.
	ops []string
}

// watchDeparted watches acc's state of id and records each write from
// departed after the state no longer names it: a checkpoint it signed above
// the departure, or an op it submitted after. The returned func stops the
// watch and returns what it recorded.
func watchDeparted(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, id, departed string) func() []string {
	// Mount the SharedObject and its state.
	t.Helper()
	so, release := mountListed(ctx, t, acc, id)
	states, releaseStates, err := so.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		release()
		t.Fatal(err)
	}

	// Check each state until stopped. Only the goroutine touches the record
	// until done closes.
	watchCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	var base *departure
	var found []string
	go func() {
		// Record each departed write until the watch stops.
		defer close(done)
		_, _ = states.WaitValueWithValidator(watchCtx, func(state *sobject.SOState) (bool, error) {
			found = append(found, departedWrites(id, departed, state, &base)...)
			return false, nil
		}, nil)
	}()

	// Stop, release and return the record.
	return func() []string {
		// Stop the watch and release what it holds.
		stop()
		<-done
		releaseStates()
		release()

		// Report a state that never dropped the departed owner.
		if base == nil {
			found = append(found, "the state never stopped naming the departed owner")
		}
		return found
	}
}

// departedWrites returns the writes from departed in state above the
// departure, setting base from the first state that no longer names departed.
func departedWrites(id, departed string, state *sobject.SOState, base **departure) []string {
	// Only a state that no longer names departed counts.
	if slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
		return p.GetPeerId() == departed
	}) || state.GetCheckpoint() == nil {
		return nil
	}
	inner, signers, err := state.GetCheckpoint().Verify(id)
	if err != nil {
		return []string{"checkpoint does not verify: " + err.Error()}
	}

	// Collect the departed owner's pending ops.
	var ops []string
	for _, op := range state.GetOps() {
		opInner, err := op.UnmarshalInner()
		if err != nil {
			return []string{"op does not decode: " + err.Error()}
		}
		if opInner.GetPeerId() == departed {
			ops = append(ops, opInner.GetLocalId()+"/"+strconv.FormatUint(opInner.GetNonce(), 10))
		}
	}

	// The first such state is the departure.
	if *base == nil {
		*base = &departure{height: inner.GetHeight(), ops: ops}
		return nil
	}

	// Report a later checkpoint it signed and an op it submitted after.
	var found []string
	if inner.GetHeight() > (*base).height && slices.Contains(signers, departed) {
		found = append(found, "checkpoint "+strconv.FormatUint(inner.GetHeight(), 10)+" signed by the departed owner")
	}
	for _, op := range ops {
		if !slices.Contains((*base).ops, op) {
			found = append(found, "op "+op+" from the departed owner")
		}
	}
	return found
}

// checkDeparted fails the test with each departed write found.
func checkDeparted(t *testing.T, found []string) {
	t.Helper()
	if len(found) != 0 {
		t.Fatalf("departed owner wrote: %s", strings.Join(found, "; "))
	}
}

// restartBoth restarts first's Session transport, then second's, logging the
// ownership state they restart in.
func restartBoth(
	ctx context.Context,
	t *testing.T,
	first *provider_local.ProviderAccount,
	firstSession *provider_local.Session,
	second *provider_local.ProviderAccount,
	secondSession *provider_local.Session,
	state string,
) {
	t.Helper()
	restartSession(ctx, t, first, firstSession, state)
	restartSession(ctx, t, second, secondSession, state)
}

// restartSession stops acc's P2P sync and Session transport, then starts
// both as a Session mount does, which restarts the replica controller on the
// new bus.
func restartSession(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, session *provider_local.Session, state string) {
	// Stop the transport and its sync.
	t.Helper()
	start := time.Now()
	acc.StopP2PSync()
	acc.StopSessionTransport()

	// Start them again.
	if err := acc.EnsureConfiguredSessionTransport(ctx, session.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	if err := acc.AutoStartP2PSyncIfNeeded(ctx, acc.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	t.Logf("restarted %s (%s) in %v", session.GetPeerId(), state, time.Since(start))
}

// waitListed waits until acc lists id.
func waitListed(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, id string) {
	t.Helper()
	_, release := mountListed(ctx, t, acc, id)
	release()
}

// waitOwnership waits until acc's state of id has an ownership ok accepts.
func waitOwnership(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, id string, ok func(*sobject.SOOwnership) bool) {
	// Mount the listed SharedObject and its state.
	t.Helper()
	so, release := mountListed(ctx, t, acc, id)
	defer release()
	host := so.GetSOHost()
	states, releaseStates, err := host.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()

	// Read the ownership of each state until ok accepts it.
	if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		own, err := sobject.ReadSOOwnership(ctx, state.GetConfig(), host.ReadConfigEntry)
		if err != nil {
			return false, err
		}
		return ok(own), nil
	}, nil); err != nil {
		t.Fatalf("ownership did not reach the awaited state: %v", err)
	}
}

// newCollector returns a block collector for vol's reference graph.
func newCollector(t *testing.T, vol volume.Volume) *block_gc.Collector {
	t.Helper()
	rg := vol.GetRefGraph()
	if rg == nil {
		t.Fatal("the account volume has no ref graph")
	}
	return block_gc.NewCollector(rg, vol, nil)
}

// collectLoop runs block passes back to back on each distinct volume until
// the returned func stops them. A failed pass fails the test.
func collectLoop(ctx context.Context, t *testing.T, vols ...volume.Volume) func() {
	// Start one loop per distinct volume.
	t.Helper()
	loopCtx, stop := context.WithCancel(ctx)
	errs := make(chan error, len(vols))
	var started []volume.Volume
	var passes atomic.Int64
	for _, vol := range vols {
		if slices.Contains(started, vol) {
			continue
		}
		started = append(started, vol)
		collector := newCollector(t, vol)
		go func() { errs <- runPasses(loopCtx, collector, &passes) }()
	}

	// Stop every loop and report the first failure.
	return func() {
		t.Helper()
		stop()
		for range started {
			if err := <-errs; err != nil {
				t.Fatalf("block pass: %v", err)
			}
		}
		t.Logf("%d block passes ran during the move", passes.Load())
	}
}

// runPasses runs block passes with collector until ctx ends, counting each.
func runPasses(ctx context.Context, collector *block_gc.Collector, passes *atomic.Int64) error {
	for {
		_, err := collector.Collect(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		passes.Add(1)
	}
}

// countMissingBlocks counts the entries log writes that report a missing
// block: a read that found none, or a backfill that found no peer holding it.
func countMissingBlocks(log *logrus.Logger) *atomic.Int64 {
	hook := &missingBlockHook{}
	log.AddHook(hook)
	return &hook.count
}

// missingBlockHook counts log entries whose message or error reports a
// missing block.
type missingBlockHook struct {
	// count is the number of such entries.
	count atomic.Int64
}

// Levels returns every level.
func (h *missingBlockHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire counts the entry when it reports a missing block.
func (h *missingBlockHook) Fire(entry *logrus.Entry) error {
	// Match the message and the error field.
	text := entry.Message
	if err, ok := entry.Data[logrus.ErrorKey].(error); ok {
		text += " " + err.Error()
	}
	if strings.Contains(text, block.ErrNotFound.Error()) || strings.Contains(text, "no peer holds") {
		h.count.Add(1)
	}
	return nil
}
