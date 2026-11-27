package space_world

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// testInstallHeadSignedReplay replays an earlier local operation while a new
// retained-root write has signed the same bytes, with or without a queue return.
func testInstallHeadSignedReplay(t *testing.T) {
	// Exercise both queue return orders, then expire a caller already in flight.
	for _, tc := range []struct {
		name        string
		returnFirst bool
		refused     bool
		expire      bool
	}{
		{name: "apply before queue return"},
		{name: "apply after queue return", returnFirst: true},
		{name: "refuse before queue return", refused: true},
		{name: "refuse after queue return", returnFirst: true, refused: true},
		{name: "apply after durability timeout", expire: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Generate IDs before entering the clock bubble, whose epoch
			// predates the earliest accepted ULID timestamp.
			t.Setenv("SPACEWAVE_WORLD_DURABILITY_TIMEOUT", "20ms")
			ids := [2]string{sobject.NewSOOperationLocalID(), sobject.NewSOOperationLocalID()}
			synctest.Test(t, func(t *testing.T) {
				runInstallHeadSignedReplay(t, ids, tc.returnFirst, tc.refused, tc.expire)
			})
		})
	}
}

// runInstallHeadSignedReplay drives the adapter as the queue and body driver
// would. It controls signing, callback and reorder separately.
func runInstallHeadSignedReplay(t *testing.T, ids [2]string, returnFirst, refused, expire bool) {
	// Set up a caught-up adapter with a replay head and a local author.
	t.Helper()
	ctx := t.Context()
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	so := &advancedSpace{peerID: p.GetPeerID()}
	a := &Adapter{
		so: so, opened: openedNow(), queue: &opQueue{},
		le: logrus.NewEntry(logrus.New()), pages: block.NewPendingStore(block_mock.NewMockStore(0), 1<<20),
		missing: make(map[string]struct{}), legacyReplayMatcher: os.Getenv(ReplayIdentityEnv) == "off",
		replayState: &sobject.SOReplayState{State: &sobject.SOReplayState_CaughtUp{CaughtUp: &sobject.SOReplayCaughtUp{}}},
	}
	a.replay = &spaceReplay{a: a, base: &InnerState{}, stage: newReplayStage(a.pages)}

	// Sign real SharedObject positions over a retained-root release, and
	// apply the older operation once with no waiter.
	wop := &WorldOp{Body: &WorldOp_SetRetainedRoot{SetRetainedRoot: &SetRetainedRootOp{Name: "kept"}}}
	data, err := wop.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	sign := func(data []byte, localID string, nonce uint64, prev []byte, config byte) *sobject.OrderedOperation {
		// Sign for the chosen chain and configuration.
		link := &sobject.SOOperationLink{Nonce: nonce, PrevOpHash: prev, ConfigHash: bytes.Repeat([]byte{config}, 32)}
		signed, err := sobject.BuildSOOperation("space", priv, data, link, localID)
		if err != nil {
			t.Fatal(err)
		}

		// Use the signed body's identity rather than injecting a separate ID.
		inner, err := signed.UnmarshalInner()
		if err != nil {
			t.Fatal(err)
		}
		return &sobject.OrderedOperation{Data: data, Hash: signed.Hash(), Inner: inner}
	}
	apply := func(index int, op *sobject.OrderedOperation) {
		if reason, err := a.Apply(ctx, index, op); err != nil || reason != op.Refused {
			t.Fatalf("apply at %d = %q, %v; want %q", index, reason, err, op.Refused)
		}
	}
	older := sign(data, ids[0], 1, nil, 3)
	apply(0, older)

	// Dequeue the next write and publish what its signer produced, without
	// completing the queue call. The caller is in its real durability wait.
	result := make(chan error, 1)
	go func() { result <- a.writeOp(ctx, wop) }()
	synctest.Wait()
	waiting := a.queue.drain()
	if len(waiting) != 1 || !bytes.Equal(waiting[0].op, older.Data) {
		t.Fatal("the new write did not queue the older operation's bytes")
	}
	adm := waiting[0]
	id := ids[1]
	current := sign(adm.op, id, 2, older.Hash, 3)
	position := &sobject.SOOperationPosition{PeerId: so.GetPeerID().String(), Nonce: 2, OpHash: current.Hash}
	if !a.legacyReplayMatcher && !adm.receipt.Publish(position) {
		t.Fatal("the signer did not publish the new position")
	}

	// Neither signing nor a queue return can decide a World write. Install
	// the older operation again at the same index as after a driver rewind.
	if returnFirst {
		a.queued(adm, id, position, nil)
	}
	if adm.state != admissionQueued {
		t.Fatal("the new write was decided before replay")
	}
	apply(0, older)
	if adm.state != admissionQueued {
		t.Fatalf("older replay decided the newer write: %v, %v", adm.decision, adm.err)
	}

	// A body-less refusal from a fork with the same local ID and nonce is
	// not the new write, and exact mode must not retain it in a cache.
	fork := sign(adm.op, id, 2, older.Hash, 4)
	fork.Data, fork.Refused = nil, "refused fork"
	apply(1, fork)
	if adm.state != admissionQueued || a.unmatched != nil {
		t.Fatal("the fork refused the new write or left a cached refusal")
	}

	// Let the actual wait clock expire without surrendering replay ownership.
	if expire {
		if err := <-result; !errors.Is(err, world.ErrDurabilityUnknown) || world.IsSuperseded(err) {
			t.Fatalf("expired write = %v; want durability unknown, not a retry", err)
		}
		if len(a.pending) != 1 || a.pending[0] != adm || len(a.queue.drain()) != 0 || adm.state != admissionQueued {
			t.Fatal("durability timeout removed or requeued the in-flight write")
		}
	}

	// Only the new operation can decide the admission; a later queue return
	// cannot change that result or create a cached refusal.
	want := world.Accepted
	if refused {
		current.Data, current.Refused = nil, "refused exact operation"
		want = world.Rejected
	}
	apply(2, current)
	if adm.state != admissionDecided || adm.decision != want || adm.reason != current.Refused ||
		!bytes.Equal(adm.hash, current.Hash) || len(a.pending) != 0 || a.unmatched != nil || a.headPositions != 3 {
		t.Fatalf("exact replay = state %v, decision %v, reason %q, position %d, pending %d, cached %d",
			adm.state, adm.decision, adm.reason, a.headPositions, len(a.pending), len(a.unmatched))
	}

	// Queue completion cannot overwrite replay's earlier decision.
	if !returnFirst {
		a.queued(adm, id, position, nil)
		if adm.decision != want || adm.reason != current.Refused || len(a.pending) != 0 || a.unmatched != nil {
			t.Fatal("queue completion changed the replay outcome")
		}
	}
	if !expire {
		if err := <-result; (!refused && err != nil) || (refused && !errors.Is(err, sobject.ErrRejectedOp)) {
			t.Fatalf("write after exact replay = %v, refused = %t", err, refused)
		}
	}
}

// TestInstallDecidesCoveredAdmissions checks that an installed checkpoint
// decides every admission of this device it covers, so none waits queued for
// a replay that will never place it: a refused one is Rejected, an applied one
// is placed for installHead to accept, and one covered by a checkpoint that
// records no outcomes, skips an installed height, or that this device cannot
// prove is on the covered chain is Undecided with durability unknown. It
// covers admissions queued before the install and those whose QueueOperation
// returns after it.
func TestInstallDecidesCoveredAdmissions(t *testing.T) {
	testRefusalPeer := peer.ID("local-peer")
	const peer = "local-peer"
	hash := func(nonce uint64) []byte { return []byte("op-" + strconv.FormatUint(nonce, 10)) }
	pos := func(nonce uint64) *sobject.SOOperationPosition {
		return &sobject.SOOperationPosition{PeerId: peer, Nonce: nonce, OpHash: hash(nonce)}
	}

	// held is the operations this device holds: one chain op-1 to op-9, each
	// linking the one below it, unless a case adds a fork.
	held := make(map[string]*sobject.SOOperationInner)
	for n := uint64(1); n <= 9; n++ {
		held[string(hash(n))] = &sobject.SOOperationInner{PeerId: peer, Nonce: n, PrevOpHash: hash(n - 1)}
	}
	get := func(h []byte) *sobject.SOOperationInner { return held[string(h)] }
	checkpoint := func(height uint64, head *sobject.SOOperationPosition, outcomes *sobject.SOCheckpointOutcomes) *sobject.SOCheckpointInner {
		other := &sobject.SOOperationPosition{PeerId: "other-peer", Nonce: 9, OpHash: []byte("other")}
		return &sobject.SOCheckpointInner{Height: height, Authors: []*sobject.SOOperationPosition{other, head}, Outcomes: outcomes}
	}
	admit := func(a *Adapter, nonce uint64) *spaceAdmission {
		adm := &spaceAdmission{a: a, op: []byte("op"), pos: pos(nonce), done: make(chan struct{})}
		a.pending = append(a.pending, adm)
		return adm
	}
	install := func(a *Adapter, c *sobject.SOCheckpointInner) {
		var refused, unknown []*spaceAdmission
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			refused, unknown = a.installCoverLocked(c, nil, nil, peer, get)
		})
		a.decideCovered(refused, unknown)
		a.headHeight, a.acceptHeight = c.GetHeight(), c.GetHeight()
	}
	expect := func(t *testing.T, name string, adm *spaceAdmission, state spaceAdmissionState, decision world.Decision, sentinel error) {
		t.Helper()
		if adm.state != state || (state == admissionDecided && adm.decision != decision) {
			t.Fatalf("%s: state %v decision %v, want state %v decision %v", name, adm.state, adm.decision, state, decision)
		}
		if sentinel != nil && !errors.Is(adm.err, sentinel) {
			t.Fatalf("%s: error %v, want %v", name, adm.err, sentinel)
		}
	}
	newAdapter := func() *Adapter {
		a := &Adapter{}
		install(a, checkpoint(1, pos(1), nil))
		return a
	}

	// A checkpoint that records outcomes and follows the installed one
	// decides each operation it newly covers, and leaves the one above it.
	a := newAdapter()
	applied, refused, head, above := admit(a, 2), admit(a, 3), admit(a, 4), admit(a, 5)
	install(a, checkpoint(2, pos(4), &sobject.SOCheckpointOutcomes{Refused: []*sobject.SOOperationPosition{pos(3)}}))
	expect(t, "applied", applied, admissionCovered, 0, nil)
	expect(t, "refused", refused, admissionDecided, world.Rejected, sobject.ErrRejectedOp)
	expect(t, "covered head", head, admissionCovered, 0, nil)
	expect(t, "above the cover", above, admissionQueued, 0, nil)

	// An admission whose QueueOperation returns after that install is decided
	// as the install would have: refused, or accepted at the installed head.
	late := &spaceAdmission{a: a, op: []byte("op"), done: make(chan struct{})}
	a.pending = append(a.pending, late)
	a.queued(late, "late-refused", pos(3), nil)
	expect(t, "refused after install", late, admissionDecided, world.Rejected, sobject.ErrRejectedOp)
	late = &spaceAdmission{a: a, op: []byte("op"), done: make(chan struct{})}
	a.pending = append(a.pending, late)
	a.queued(late, "late-applied", pos(2), nil)
	expect(t, "applied after install", late, admissionDecided, world.Accepted, nil)

	// A checkpoint signed before outcomes existed covers its operations
	// without deciding them, so they end as durability unknown.
	a = newAdapter()
	first, second := admit(a, 2), admit(a, 3)
	install(a, checkpoint(2, pos(3), nil))
	expect(t, "pre-change checkpoint", first, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	expect(t, "pre-change checkpoint head", second, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)

	// A checkpoint that skips an installed height may cover refusals the
	// skipped one listed, so it proves nothing applied. A refusal it lists
	// names its operation by hash and decides it.
	a = newAdapter()
	skipped, skippedRefused := admit(a, 2), admit(a, 3)
	install(a, checkpoint(3, pos(3), &sobject.SOCheckpointOutcomes{Refused: []*sobject.SOOperationPosition{pos(3)}}))
	expect(t, "skipped height", skipped, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	expect(t, "refused at a skipped height", skippedRefused, admissionDecided, world.Rejected, sobject.ErrRejectedOp)

	// The first install after a mount knows no previous head. It decides the
	// refusals the checkpoint lists, and an operation not listed stays hidden:
	// an earlier checkpoint may have covered it.
	firstInstall := func() (applied, refused *spaceAdmission) {
		a := &Adapter{}
		applied, refused = admit(a, 2), admit(a, 3)
		install(a, checkpoint(2, pos(3), &sobject.SOCheckpointOutcomes{Refused: []*sobject.SOOperationPosition{pos(3)}}))
		return applied, refused
	}
	first, firstRefused := firstInstall()
	expect(t, "first install applied", first, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	expect(t, "first install refused", firstRefused, admissionDecided, world.Rejected, sobject.ErrRejectedOp)

	// Two checkpoints signed after the unknown operation before the remount:
	// C1 covered op-2 and op-3 and refused op-3, then C2 covered op-4 and
	// op-5 and refused op-5. The first install is C2. It answers its own
	// refusal, and neither proves op-2 applied nor op-3 refused, since only
	// C1 recorded them.
	a = &Adapter{}
	behindApplied, behindRefused, newRefused := admit(a, 2), admit(a, 3), admit(a, 5)
	install(a, checkpoint(3, pos(5), &sobject.SOCheckpointOutcomes{Refused: []*sobject.SOOperationPosition{pos(5)}}))
	expect(t, "applied before the first install's checkpoint", behindApplied, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	expect(t, "refused before the first install's checkpoint", behindRefused, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	expect(t, "refused by the first install's checkpoint", newRefused, admissionDecided, world.Rejected, sobject.ErrRejectedOp)

	// The switch restores the old rule: the first install decides nothing.
	t.Setenv(checkpointStandaloneOutcomeEnv, "off")
	_, firstRefused = firstInstall()
	expect(t, "first install refused, switch off", firstRefused, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	t.Setenv(checkpointStandaloneOutcomeEnv, "")

	// A covered head whose hash is not this operation's names another chain.
	a = newAdapter()
	forked := admit(a, 2)
	other := pos(2)
	other.OpHash = []byte("forked")
	install(a, checkpoint(2, other, &sobject.SOCheckpointOutcomes{}))
	expect(t, "forked head", forked, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)

	// A restore of the same identity signs a second chain from nonce 2. A
	// checkpoint covering that fork's tip covers neither this chain's
	// operations below it nor one queued late on this chain, even at a nonce
	// the cover passes: none is on the walk from the fork's tip.
	a = newAdapter()
	held["fork-2"] = &sobject.SOOperationInner{PeerId: peer, Nonce: 2, PrevOpHash: hash(1)}
	held["fork-3"] = &sobject.SOOperationInner{PeerId: peer, Nonce: 3, PrevOpHash: []byte("fork-2")}
	nonTip := admit(a, 2)
	forkTip := &sobject.SOOperationPosition{PeerId: peer, Nonce: 3, OpHash: []byte("fork-3")}
	install(a, checkpoint(2, forkTip, &sobject.SOCheckpointOutcomes{}))
	expect(t, "non-tip after a restore", nonTip, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	late = &spaceAdmission{a: a, op: []byte("op"), done: make(chan struct{})}
	a.pending = append(a.pending, late)
	a.queued(late, "late-non-tip", pos(3), nil)
	expect(t, "non-tip queued after a restore", late, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)

	// A link the device does not hold ends the proof: the operations below
	// it are Undecided, and those on the walk still decide.
	a = newAdapter()
	gap, proven := admit(a, 2), admit(a, 4)
	delete(held, string(hash(3)))
	install(a, checkpoint(2, pos(4), &sobject.SOCheckpointOutcomes{}))
	expect(t, "below a missing link", gap, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	expect(t, "on the walk above a missing link", proven, admissionCovered, 0, nil)

	// Replay of a data-less refusal and the queue callback may arrive in
	// either order. Force the split between Apply's match and noteUnmatched
	// without sleeping or letting a checkpoint decide the admission.
	for _, order := range []string{"callback between match and note", "replay before callback", "fork after callback", "fork before callback"} {
		t.Run(order, func(t *testing.T) {
			// Admit two writes with identical payloads but distinct identities.
			so := &advancedSpace{peerID: testRefusalPeer}
			a := &Adapter{so: so, legacyReplayMatcher: true}
			decoy, target := admit(a, 7), admit(a, 7)
			decoy.pos, target.pos = nil, nil
			position := func(hash string) *sobject.SOOperationPosition {
				return &sobject.SOOperationPosition{PeerId: so.GetPeerID().String(), Nonce: 7, OpHash: []byte(hash)}
			}
			a.queued(decoy, "same-payload", position("decoy"), nil)
			queue := func() { a.queued(target, "signed-local-id", position("exact"), nil) }

			// Run the two refusal steps Apply takes for a body-less entry.
			refuse := func(hash string, between func()) {
				op := &sobject.OrderedOperation{
					Hash: []byte(hash), Refused: hash,
					Inner: &sobject.SOOperationInner{PeerId: so.GetPeerID().String(), LocalId: "signed-local-id", Nonce: 7},
				}
				matched := a.match(op)
				if between != nil {
					between()
				}

				// Record the final replay position with the same dispatch as Apply.
				pos := spacePosition{op: op, reason: op.Refused}
				if matched != nil {
					a.place(matched, pos, true)
				} else {
					a.noteUnmatched(op, pos)
				}
			}

			// Exercise both arrival orders and both sides of a same-ID fork.
			switch order {
			case "callback between match and note":
				refuse("exact", queue)
			case "replay before callback":
				refuse("exact", nil)
				queue()
			case "fork after callback":
				queue()
				refuse("fork", nil)
				expect(t, "fork must not refuse target", target, admissionQueued, world.Undecided, nil)
				refuse("exact", nil)
			case "fork before callback":
				refuse("fork", nil)
				queue()
				expect(t, "cached fork must not refuse target", target, admissionQueued, world.Undecided, nil)
				refuse("exact", nil)
			}

			// WaitDecided must be ready without any other replay or install.
			select {
			case <-target.done:
				expect(t, "exact refusal", target, admissionDecided, world.Rejected, sobject.ErrRejectedOp)
				if target.reason != "exact" {
					t.Fatalf("target refused by %q, want exact operation", target.reason)
				}
			default:
				t.Fatal("exact refusal did not wake the admission")
			}
			// The rejection must survive both public waits without a success seqno.
			if decision, err := target.WaitDecided(t.Context()); decision != world.Rejected || !errors.Is(err, sobject.ErrRejectedOp) {
				t.Fatalf("WaitDecided returned %v, %v for a refused operation", decision, err)
			}
			if seqno, err := target.WaitDurable(t.Context()); seqno != 0 || !errors.Is(err, sobject.ErrRejectedOp) {
				t.Fatalf("WaitDurable returned %d, %v for a refused operation", seqno, err)
			}
			expect(t, "same-payload admission", decoy, admissionQueued, world.Undecided, nil)
		})
	}

	// An adjacent checkpoint proves the exact tip even if replay never held
	// its body. It does not prove a fork or turn a refusal into success.
	{
		// Select the signed-tip path regardless of the test process's override.
		t.Setenv(checkpointTipOutcomeEnv, "")
		// Give replay neither the accepted nor the refused tip body.
		delete(held, string(hash(4)))
		a := newAdapter()
		exact, forked := admit(a, 4), admit(a, 4)
		forked.pos.OpHash = []byte("same nonce on another chain")
		install(a, checkpoint(2, pos(4), &sobject.SOCheckpointOutcomes{}))
		expect(t, "exact signed tip", exact, admissionCovered, 0, nil)
		expect(t, "fork at signed tip", forked, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)

		// Check the late callback and the explicit refusal from the same proof.
		late := &spaceAdmission{a: a, op: []byte("op"), done: make(chan struct{})}
		a.pending = append(a.pending, late)
		a.queued(late, "late-tip", pos(4), nil)
		expect(t, "late exact tip", late, admissionDecided, world.Accepted, nil)
		a = newAdapter()
		refused := admit(a, 4)
		install(a, checkpoint(2, pos(4), &sobject.SOCheckpointOutcomes{Refused: []*sobject.SOOperationPosition{pos(4)}}))
		expect(t, "refused tip", refused, admissionDecided, world.Rejected, sobject.ErrRejectedOp)

		// The rollout override keeps the old conservative result selectable.
		t.Setenv(checkpointTipOutcomeEnv, "off")
		a = newAdapter()
		legacy := admit(a, 4)
		install(a, checkpoint(2, pos(4), &sobject.SOCheckpointOutcomes{}))
		expect(t, "legacy exact tip", legacy, admissionDecided, world.Undecided, world.ErrDurabilityUnknown)
	}

	// An admission queued before any install waits for replay.
	a = &Adapter{}
	early := &spaceAdmission{a: a, op: []byte("op"), done: make(chan struct{})}
	a.pending = append(a.pending, early)
	a.queued(early, "early", pos(1), nil)
	expect(t, "before any install", early, admissionQueued, 0, nil)
}

// TestSettleAdoptsOnlyWhatTheMountShows forces the order a restart inside the
// unknown's reload allows: the old adapter sends its writes, the next adapter
// opens, and only then does the old adapter tell them unknown. It tries to
// make the settle set lose a write told after the next adapter opened, or hand
// an adapter a write its mount cannot answer. The next adapter must learn of
// each tell, settle a told write by absence only when it was sent before it
// opened, and on the old mount only once its QueueOperation returned there
// with a result the mount shows: a failure after the run's ctx ended counts
// only on a SharedObject that writes nothing after its call returns.
// Its replay settles a write it applies whether or not the old adapter told it
// yet. A decided write leaves no entry, and a resolved one drops its mount and
// answers one WaitSettled.
func TestSettleAdoptsOnlyWhatTheMountShows(t *testing.T) {
	ctx := t.Context()
	set := newSettleSet()
	localPeer := peer.ID("local-peer")
	oldMount, newMount := &advancedSpace{peerID: localPeer}, &advancedSpace{peerID: localPeer}
	oldA, newA := &Adapter{so: oldMount, settles: set}, &Adapter{so: newMount, settles: set}
	peerID := localPeer.String()

	// send admits a write on the old adapter and passes it to the signer, as
	// the queue does.
	send := func(nonce uint64, hash string) *spaceAdmission {
		adm := oldA.newAdmission(world.Candidate{}, []byte(hash), nil, nil)
		oldA.admit(adm)
		oldA.registerSent(adm)
		adm.receipt.Publish(&sobject.SOOperationPosition{PeerId: peerID, Nonce: nonce, OpHash: []byte(hash)})
		return adm
	}
	inFlight, stored, replayed, accepted := send(1, "in-flight"), send(2, "stored"), send(3, "replayed"), send(4, "accepted")

	// The next adapter opens before any tell, and one more write is sent.
	newA.settleSeq = set.last()
	late := send(5, "late")
	var rev uint64
	set.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { rev = set.rev })
	woke := make(chan []*settleEntry, 1)
	go func() {
		ents, _, _ := set.waitTold(ctx, newMount, newA.settleSeq, rev)
		woke <- ents
	}()

	// The next adapter's replay applies one write before the old adapter told
	// it, and settles it once.
	op := &sobject.OrderedOperation{Hash: []byte("replayed"), Inner: &sobject.SOOperationInner{PeerId: peerID, Nonce: 3}}
	adm := newA.match(op)
	if adm == nil || adm.settle != replayed.sent {
		t.Fatalf("the next adapter's replay matched %v, want the entry of the write in flight on the old mount", adm)
	}
	if again := newA.match(op); again != adm {
		t.Fatal("replaying the write again adopted it twice")
	}
	newA.decide([]*spaceAdmission{adm}, world.Accepted, "", nil)
	if got := set.unresolved([]*settleEntry{replayed.sent, inFlight.sent}); !slices.Equal(got, []*settleEntry{inFlight.sent}) {
		t.Fatal("an adapter would adopt again a write another replay resolved after it was told")
	}

	// The old adapter decides one write and tells the rest unknown, one of
	// them after its QueueOperation returned.
	oldA.decide([]*spaceAdmission{accepted}, world.Accepted, "", nil)
	oldA.sentReturned(stored, nil)
	for _, adm := range []*spaceAdmission{inFlight, stored, replayed, late} {
		oldA.fail(adm, durabilityUnknown(adm.receipt.OpHash(), errAdapterStopped))
	}
	if ents := <-woke; len(ents) == 0 {
		t.Fatal("the next adapter did not learn of a tell after it opened")
	}
	if ent, _ := set.replayed([]byte("accepted")); ent != nil {
		t.Fatal("a decided write kept its entry")
	}

	// hashes returns the writes an adapter on so that opened after send order
	// upto settles by absence.
	hashes := func(so sobject.SharedObject, upto uint64) []string {
		ents, _, err := set.waitTold(ctx, so, upto, ^uint64(0))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, ent := range ents {
			out = append(out, string(ent.pos.GetOpHash()))
		}
		return out
	}
	if got := hashes(newMount, newA.settleSeq); !slices.Equal(got, []string{"in-flight", "stored"}) {
		t.Fatalf("the next adapter settles %q by absence, want the two sent before it opened, in nonce order", got)
	}
	if got := hashes(newMount, set.last()); !slices.Equal(got, []string{"in-flight", "stored", "late"}) {
		t.Fatalf("a later adapter settles %q by absence, want every unresolved told write", got)
	}
	if got := hashes(oldMount, set.last()); !slices.Equal(got, []string{"stored"}) {
		t.Fatalf("an adapter on the old mount settles %q by absence, want only the stored write", got)
	}
	oldA.sentReturned(inFlight, context.Canceled)
	if got := hashes(oldMount, set.last()); !slices.Equal(got, []string{"stored"}) {
		t.Fatalf("a failure after the run ended on a mount that may still commit made an adapter on it settle %q", got)
	}
	set.returned(inFlight.sent)
	if got := hashes(oldMount, set.last()); !slices.Equal(got, []string{"in-flight", "stored"}) {
		t.Fatalf("after its call returned an adapter on the old mount settles %q", got)
	}

	// The replayed write's answer drops its mount, is read once, then is gone.
	if replayed.sent.so != nil {
		t.Fatal("a resolved entry still pins its mount")
	}
	if _, applied, err := set.wait(ctx, []byte("replayed")); err != nil || !applied {
		t.Fatalf("WaitSettled answered applied=%t, %v; want applied", applied, err)
	}
	if _, _, err := set.wait(ctx, []byte("replayed")); !errors.Is(err, ErrNoSettle) {
		t.Fatalf("a second WaitSettled returned %v, want ErrNoSettle", err)
	}
}
