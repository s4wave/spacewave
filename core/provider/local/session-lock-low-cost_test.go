package provider_local

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/routine"
	"github.com/aperturerobotics/util/scrub"
	"github.com/s4wave/spacewave/core/provider"
	core_session "github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/util/blockenc"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/scrypt"
)

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.observed)
	})
	return c.Context.Done()
}

type sessionTransportFollowHook struct {
	observed chan struct{}
	once     sync.Once
}

type sessionMetadataControllerStub struct {
	core_session.SessionController
	listErr error
	updated *core_session.SessionMetadata
}

func (s *sessionMetadataControllerStub) ListSessions(context.Context) ([]*core_session.SessionListEntry, error) {
	return nil, s.listErr
}

func (s *sessionMetadataControllerStub) UpdateSessionMetadata(
	_ context.Context,
	_ *core_session.SessionRef,
	metadata *core_session.SessionMetadata,
) error {
	s.updated = metadata
	return nil
}

func (h *sessionTransportFollowHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *sessionTransportFollowHook) Fire(entry *logrus.Entry) error {
	if entry.Message == "session transport already exists, skipping creation" {
		h.once.Do(func() {
			close(h.observed)
		})
	}
	return nil
}

func TestSessionLockModeMetadataPreservesAttachmentIdentity(t *testing.T) {
	ref := &core_session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id:                "session",
		ProviderId:        "local",
		ProviderAccountId: "account",
	}}

	t.Run("lookup failure", func(t *testing.T) {
		lookupErr := errors.New("metadata unavailable")
		ctrl := &sessionMetadataControllerStub{listErr: lookupErr}
		err := updateSessionLockModeMetadata(
			t.Context(),
			ctrl,
			ref,
			core_session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED,
		)
		if !errors.Is(err, lookupErr) {
			t.Fatalf("update error = %v, want %v", err, lookupErr)
		}
		if ctrl.updated != nil {
			t.Fatal("metadata update replaced identity after a failed lookup")
		}
	})

	t.Run("missing metadata", func(t *testing.T) {
		ctrl := &sessionMetadataControllerStub{}
		err := updateSessionLockModeMetadata(
			t.Context(),
			ctrl,
			ref,
			core_session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED,
		)
		if err != nil {
			t.Fatal(err)
		}
		if ctrl.updated == nil {
			t.Fatal("metadata was not updated")
		}
		if ctrl.updated.GetProviderId() != "local" || ctrl.updated.GetProviderAccountId() != "account" {
			t.Fatalf("updated identity = %s/%s, want local/account", ctrl.updated.GetProviderId(), ctrl.updated.GetProviderAccountId())
		}
	})
}

func TestMountedPINUnlockRestoresLocalSessionStateLowCost(t *testing.T) {
	ctx := t.Context()
	_, sessRef, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	pin := []byte("2468")
	configureLowCostPINLock(ctx, t, sess, pin)
	if err := sess.LockSession(ctx); err != nil {
		t.Fatalf("lock session: %v", err)
	}
	if sess.GetPrivKey() != nil {
		t.Fatal("expected locked session private key to be cleared")
	}

	if err := sess.UnlockSession(ctx, []byte("wrong")); err == nil {
		t.Fatal("expected wrong PIN to fail mounted unlock")
	}
	if sess.GetPrivKey() != nil {
		t.Fatal("wrong PIN restored the session private key")
	}

	if err := sess.UnlockSession(ctx, pin); err != nil {
		t.Fatalf("unlock mounted session: %v", err)
	}
	if sess.GetPrivKey() == nil {
		t.Fatal("expected mounted unlock to restore the session private key")
	}

	mounted, mountedRelease, err := acc.MountSession(ctx, sessRef, nil)
	if err != nil {
		t.Fatalf("mount session after mounted unlock: %v", err)
	}
	defer mountedRelease()
	if mounted.GetPrivKey() == nil {
		t.Fatal("expected future mount to receive unlocked session")
	}
}

func TestEnsureSessionTransportReleasesAccountLockWhileWaitingReady(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer ctxCancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)

	waitCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(waitCtx, sess.GetPrivKey(), "ws://127.0.0.1:1", "")
		done <- err
	}()

	running, ch := acc.GetTransportSnapshotWithWait()
	for !running {
		select {
		case <-ch:
			running, ch = acc.GetTransportSnapshotWithWait()
		case err := <-done:
			cancel()
			if err != nil {
				t.Fatalf("transport exited before wait state: %v", err)
			}
			t.Fatal("transport exited before wait state")
		case <-ctx.Done():
			cancel()
			t.Fatal(ctx.Err())
		}
	}

	lockCtx, lockCancel := context.WithTimeout(ctx, time.Second)
	defer lockCancel()
	rel, err := acc.mtx.Lock(lockCtx)
	if err != nil {
		cancel()
		t.Fatalf("account mutex stayed locked while transport was waiting: %v", err)
	}
	rel()

	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("transport wait did not return after cancellation: %v", ctx.Err())
	}
}

func TestSessionTransportReplacementContextDoesNotInventDeadline(t *testing.T) {
	callerCtx, callerCancel := context.WithCancel(t.Context())
	replacementCtx, replacementCancel := sessionTransportReplacementContext(callerCtx)

	if deadline, ok := replacementCtx.Deadline(); ok {
		t.Fatalf("replacement context added private deadline %v", deadline)
	}
	callerCancel()
	select {
	case <-replacementCtx.Done():
		t.Fatal("caller cancellation interrupted mandatory transport cleanup")
	default:
	}
	replacementCancel()
	select {
	case <-replacementCtx.Done():
	default:
		t.Fatal("replacement cleanup context did not cancel")
	}
}

func TestSessionTransportReadyErrorCleanupHonorsCallerDeadline(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer ctxCancel()
	acc, _, release := newPairingTransportAccount(ctx, t)
	defer release()

	rel, err := acc.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cleanupCancel()
	done := make(chan error, 1)
	go func() {
		done <- acc.cleanupSessionTransportReadyError(
			cleanupCtx,
			&sessionTransportState{},
			errors.New("startup timeout"),
		)
	}()

	select {
	case err := <-done:
		rel()
		if err == nil || !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cleanup returned %v, want bounded context cancellation", err)
		}
	case <-time.After(time.Second):
		rel()
		t.Fatal("cleanup remained blocked on the account owner")
	}
}

func TestSessionTransportReadyErrorCleanupUsesFreshBudgetAfterCallerExpiry(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer ctxCancel()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	gate := newStartupGate()
	st, err := transport.NewSessionTransport(
		acc.le,
		acc.t.p.b,
		sessionKey,
		"",
		"",
		gate.option(),
	)
	if err != nil {
		t.Fatal(err)
	}
	rc := routine.NewRoutineContainer()
	rc.SetRoutine(st.Execute)
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	rc.SetContext(runCtx, false)
	peerID, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		t.Fatal(err)
	}
	sts := &sessionTransportState{
		transport: st,
		rc:        rc,
		config:    sessionTransportConfig{peerID: peerID},
	}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = sts
		bcast()
	})

	ownerRelease, err := acc.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	callerCtx, callerCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer callerCancel()
	done := make(chan error, 1)
	go func() {
		done <- acc.waitSessionTransportReady(callerCtx, sts)
	}()

	select {
	case <-gate.started:
	case <-callerCtx.Done():
		ownerRelease()
		t.Fatal("transport startup did not reach the gate")
	}
	select {
	case <-callerCtx.Done():
	case <-ctx.Done():
		ownerRelease()
		t.Fatalf("caller context did not expire: %v", ctx.Err())
	}
	ownerRelease()

	var cleanupErr error
	select {
	case cleanupErr = <-done:
	case <-ctx.Done():
		t.Fatalf("cleanup did not return after releasing the account owner: %v", ctx.Err())
	}
	if cleanupErr == nil || !errors.Is(cleanupErr, context.Canceled) && !errors.Is(cleanupErr, context.DeadlineExceeded) {
		t.Fatalf("expired caller returned %v, want caller cancellation", cleanupErr)
	}

	var current *sessionTransportState
	acc.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = acc.sessionTransport
	})
	if current != nil {
		t.Fatal("expired caller left dead transport current")
	}
	var exited bool
	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		exited = sts.exited
	})
	if !exited {
		t.Fatal("expired caller did not publish dead transport exit")
	}

	replacementCtx, replacementCancel := context.WithTimeout(ctx, time.Second)
	defer replacementCancel()
	if err := acc.EnsureSessionTransport(replacementCtx, sessionKey, ""); err != nil {
		t.Fatalf("same-configuration replacement: %v", err)
	}
	replacement := acc.GetSessionTransport()
	if replacement == nil || replacement == sts.transport {
		t.Fatal("same-configuration ensure did not create a replacement")
	}
}

func TestCanceledSessionTransportCreatorStopsPendingState(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	gate := newStartupGate()
	acc.t.p.localNetwork = gate.option()

	creatorCtx, creatorCancel := context.WithCancel(ctx)
	type ensureResult struct {
		sts *sessionTransportState
		err error
	}
	creatorDone := make(chan ensureResult, 1)
	go func() {
		sts, _, err := acc.ensureSessionTransport(creatorCtx, sessionKey, unreachableSignalingURL, "")
		creatorDone <- ensureResult{sts: sts, err: err}
	}()

	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatalf("creator did not reach the startup gate: %v", ctx.Err())
	}
	creatorCancel()
	select {
	case <-gate.canceled:
	case <-ctx.Done():
		t.Fatalf("startup did not observe creator cancellation: %v", ctx.Err())
	}

	var result ensureResult
	select {
	case result = <-creatorDone:
	case <-ctx.Done():
		t.Fatalf("creator did not return after cancellation: %v", ctx.Err())
	}
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("creator returned %v, want context cancellation", result.err)
	}
	if result.sts == nil {
		t.Fatal("creator did not return its pending transport state")
	}
	var exited bool
	var exitErr error
	result.sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		exited = result.sts.exited
		exitErr = result.sts.err
	})
	if !exited {
		t.Fatal("canceled creator did not publish transport exit")
	}
	if !errors.Is(exitErr, context.Canceled) {
		t.Fatalf("canceled creator exit error = %v, want context cancellation", exitErr)
	}
	if acc.GetSessionTransport() != nil {
		t.Fatal("canceled creator left its transport current")
	}

	replacement, created, err := acc.ensureSessionTransport(ctx, sessionKey, unreachableSignalingURL, "")
	if err != nil {
		t.Fatalf("same-configuration ensure after cancellation failed: %v", err)
	}
	if !created {
		t.Fatal("same-configuration ensure reused the abandoned transport")
	}
	if replacement == result.sts {
		t.Fatal("same-configuration ensure returned the abandoned transport")
	}
	if replacement.transport.GetChildBus() == nil {
		t.Fatal("replacement transport was not usable after readiness")
	}
}

func TestEnsureSessionTransportRetriesWhenExistingTransportClearsBeforeReady(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer ctxCancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)

	fakeTransport, err := transport.NewSessionTransport(acc.le, acc.t.p.b, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	fakeState := &sessionTransportState{
		transport: fakeTransport,
		config: sessionTransportConfig{
			peerID: sess.GetPeerId(),
		},
	}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = fakeState
		bcast()
	})
	clearFakeState := func() {
		acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			if acc.sessionTransport == fakeState {
				acc.sessionTransport = nil
				bcast()
			}
		})
	}
	defer clearFakeState()

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(waitCtx, sess.GetPrivKey(), "", "")
		done <- err
	}()

	lockCtx, lockCancel := context.WithTimeout(ctx, time.Second)
	rel, err := acc.mtx.Lock(lockCtx)
	lockCancel()
	if err != nil {
		cancel()
		t.Fatalf("account mutex stayed locked while waiting on existing transport: %v", err)
	}
	rel()

	clearFakeState()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ensure session transport after cleared state: %v", err)
		}
	case <-ctx.Done():
		cancel()
		t.Fatalf("ensure session transport remained blocked after cleared state: %v", ctx.Err())
	}
}

func TestEnsureSessionTransportCoalescesSameConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)

	first, created, err := acc.ensureSessionTransport(ctx, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatalf("first ensure failed: %v", err)
	}
	if !created {
		t.Fatal("first ensure did not create the transport")
	}
	second, created, err := acc.ensureSessionTransport(ctx, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatalf("same-configuration ensure failed: %v", err)
	}
	if created {
		t.Fatal("same-configuration ensure created a second transport")
	}
	if second != first {
		t.Fatal("same-configuration ensure did not coalesce onto the existing transport")
	}
}

func TestEnsureSessionTransportPostUnlockStartDoesNotSupersedeExplicitCallers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	gate := newStartupGate()
	acc.t.p.localNetwork = gate.option()
	defer gate.release()

	firstDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(ctx, sessionKey, unreachableSignalingURL, "")
		firstDone <- err
	}()
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatalf("first explicit transport did not begin startup: %v", ctx.Err())
	}

	backgroundObserved := make(chan struct{})
	backgroundCtx := &observedDoneContext{Context: ctx, observed: backgroundObserved}
	backgroundDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransportWithoutReplacement(backgroundCtx, sessionKey, "", "")
		backgroundDone <- err
	}()
	select {
	case <-backgroundObserved:
	case <-ctx.Done():
		t.Fatalf("post-unlock transport did not reach its readiness wait: %v", ctx.Err())
	}

	secondObserved := make(chan struct{})
	secondCtx := &observedDoneContext{Context: ctx, observed: secondObserved}
	secondDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(secondCtx, sessionKey, unreachableSignalingURL, "")
		secondDone <- err
	}()
	select {
	case <-secondObserved:
	case <-ctx.Done():
		t.Fatalf("second explicit transport did not reach its readiness wait: %v", ctx.Err())
	}
	gate.release()

	for name, done := range map[string]<-chan error{
		"first explicit":  firstDone,
		"post-unlock":     backgroundDone,
		"second explicit": secondDone,
	} {
		select {
		case err := <-done:
			if errors.Is(err, errSessionTransportSuperseded) {
				t.Fatalf("%s transport was superseded: %v", name, err)
			}
			if err != nil {
				t.Fatalf("%s transport failed: %v", name, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s transport did not finish: %v", name, ctx.Err())
		}
	}
}

func TestUnlockSessionDoesNotSupersedeExplicitSessionTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	pin := []byte("2468")
	sessionKey := sess.GetPrivKey()
	configureLowCostPINLock(ctx, t, sess, pin)
	if err := sess.LockSession(ctx); err != nil {
		t.Fatalf("lock session: %v", err)
	}
	acc.StopSessionTransport()

	gate := newStartupGate()
	acc.t.p.localNetwork = gate.option()
	defer gate.release()

	firstDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(ctx, sessionKey, unreachableSignalingURL, "")
		firstDone <- err
	}()
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatalf("explicit transport did not begin startup: %v", ctx.Err())
	}

	followObserved := make(chan struct{})
	acc.le.Logger.AddHook(&sessionTransportFollowHook{observed: followObserved})
	unlockDone := make(chan error, 1)
	go func() {
		unlockDone <- sess.UnlockSession(ctx, pin)
	}()

	firstPending := true
	select {
	case <-followObserved:
	case err := <-firstDone:
		firstPending = false
		gate.release()
		if errors.Is(err, errSessionTransportSuperseded) {
			t.Fatalf("explicit transport was superseded by unlock startup: %v", err)
		}
		if err != nil {
			t.Fatalf("explicit transport failed before unlock followed it: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("unlock startup did not select a transport policy: %v", ctx.Err())
	}
	gate.release()

	if firstPending {
		select {
		case err := <-firstDone:
			if errors.Is(err, errSessionTransportSuperseded) {
				t.Fatalf("explicit transport was superseded by unlock startup: %v", err)
			}
			if err != nil {
				t.Fatalf("explicit transport failed: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("explicit transport did not finish: %v", ctx.Err())
		}
	}
	select {
	case err := <-unlockDone:
		if err != nil {
			t.Fatalf("unlock session: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("unlock session did not finish: %v", ctx.Err())
	}
}

func TestEnsureSessionTransportWithoutReplacementRetriesAfterFollowedTransportIsSuperseded(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	gate := newStartupGate()
	acc.t.p.localNetwork = gate.option()
	defer gate.release()

	firstDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(ctx, sessionKey, unreachableSignalingURL, "")
		firstDone <- err
	}()
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatalf("first transport did not begin startup: %v", ctx.Err())
	}

	followerObserved := make(chan struct{})
	followerCtx := &observedDoneContext{Context: ctx, observed: followerObserved}
	followerDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransportWithoutReplacement(followerCtx, sessionKey, "", "")
		followerDone <- err
	}()
	select {
	case <-followerObserved:
	case <-ctx.Done():
		t.Fatalf("non-replacing caller did not follow the first transport: %v", ctx.Err())
	}

	if _, _, err := acc.ensureSessionTransport(ctx, sessionKey, "", ""); err != nil {
		t.Fatalf("replacement transport failed: %v", err)
	}
	select {
	case err := <-followerDone:
		if err != nil {
			t.Fatalf("non-replacing follower failed after supersession: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("non-replacing follower did not retry: %v", ctx.Err())
	}
	select {
	case err := <-firstDone:
		if !errors.Is(err, errSessionTransportSuperseded) {
			t.Fatalf("first transport returned %v, want superseded error", err)
		}
	case <-ctx.Done():
		t.Fatalf("first transport did not report supersession: %v", ctx.Err())
	}
}

func TestEnsureSessionTransportWithoutReplacementRetriesAfterStartedTransportIsSuperseded(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	acc, sessionKey, release := newPairingTransportAccount(ctx, t)
	defer release()

	gate := newStartupGate()
	acc.t.p.localNetwork = gate.option()
	defer gate.release()

	starterDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransportWithoutReplacement(ctx, sessionKey, unreachableSignalingURL, "")
		starterDone <- err
	}()
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatalf("non-replacing transport did not begin startup: %v", ctx.Err())
	}

	if _, _, err := acc.ensureSessionTransport(ctx, sessionKey, "", ""); err != nil {
		t.Fatalf("replacement transport failed: %v", err)
	}
	select {
	case err := <-starterDone:
		if err != nil {
			t.Fatalf("non-replacing starter failed after supersession: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("non-replacing starter did not retry: %v", ctx.Err())
	}
}

func TestExistingSessionTransportWaitReturnsStateExit(t *testing.T) {
	_, _, acc, sess, release := setupProviderAndSessionInternal(t.Context(), t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)
	baseCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	fakeTransport, err := transport.NewSessionTransport(acc.le, acc.t.p.b, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	fakeState := &sessionTransportState{
		transport: fakeTransport,
		rc:        routine.NewRoutineContainer(),
	}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = fakeState
		bcast()
	})
	defer func() {
		acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			if acc.sessionTransport == fakeState {
				acc.sessionTransport = nil
				bcast()
			}
		})
	}()

	observed := make(chan struct{})
	waitCtx := &observedDoneContext{
		Context:  baseCtx,
		observed: observed,
	}
	done := make(chan error, 1)
	go func() {
		done <- acc.waitExistingSessionTransportReady(waitCtx, fakeState)
	}()
	select {
	case <-observed:
	case <-baseCtx.Done():
		t.Fatalf("waiter did not enter its wait path: %v", baseCtx.Err())
	}
	fakeState.setExited(context.Canceled)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait returned %v, want state cancellation", err)
		}
	case <-baseCtx.Done():
		t.Fatalf("state exit did not wake existing wait: %v", baseCtx.Err())
	}
}

func TestExistingSessionTransportWaitReturnsSuperseded(t *testing.T) {
	baseCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(baseCtx, t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)

	fakeTransport, err := transport.NewSessionTransport(acc.le, acc.t.p.b, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	fakeState := &sessionTransportState{
		transport: fakeTransport,
		rc:        routine.NewRoutineContainer(),
	}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = fakeState
		bcast()
	})
	defer func() {
		acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
			if acc.sessionTransport == fakeState {
				acc.sessionTransport = nil
				bcast()
			}
		})
	}()

	observed := make(chan struct{})
	waitCtx := &observedDoneContext{
		Context:  baseCtx,
		observed: observed,
	}
	done := make(chan error, 1)
	go func() {
		done <- acc.waitExistingSessionTransportReady(waitCtx, fakeState)
	}()
	select {
	case <-observed:
	case <-baseCtx.Done():
		t.Fatalf("waiter did not enter its wait path: %v", baseCtx.Err())
	}
	fakeState.setReplaced()

	select {
	case err := <-done:
		if !errors.Is(err, errSessionTransportSuperseded) {
			t.Fatalf("wait returned %v, want superseded signal", err)
		}
	case <-baseCtx.Done():
		t.Fatalf("replacement did not wake existing wait: %v", baseCtx.Err())
	}
}

func TestSessionTransportReadyErrorClassifiesReplacementFromFreshState(t *testing.T) {
	ctx := t.Context()
	sts := &sessionTransportState{}
	readyResult := make(chan error, 1)
	readyResult <- context.Canceled

	var stateWaitCh <-chan struct{}
	sts.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		stateWaitCh = getWaitCh()
	})
	sts.setReplaced()

	select {
	case <-readyResult:
	default:
		t.Fatal("readiness result was not eligible")
	}
	select {
	case <-stateWaitCh:
	default:
		t.Fatal("replacement notification was not eligible")
	}

	err := classifySessionTransportReadyError(ctx, sts, context.Canceled, func(err error) error {
		t.Fatalf("cleanup called for superseded transport: %v", err)
		return err
	})
	if !errors.Is(err, errSessionTransportSuperseded) {
		t.Fatalf("ready error classification = %v, want superseded signal", err)
	}
}

func TestSessionTransportReadyErrorCleanupRechecksSupersession(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	acc, oldPriv, release := newPairingTransportAccount(ctx, t)
	defer release()

	oldTransport, err := transport.NewSessionTransport(acc.le, acc.t.p.b, oldPriv, "", "")
	if err != nil {
		t.Fatal(err)
	}
	oldRC := routine.NewRoutineContainer()
	oldRC.SetRoutine(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	oldRC.SetContext(ctx, false)
	oldPeerID, err := peer.IDFromPrivateKey(oldPriv)
	if err != nil {
		t.Fatal(err)
	}
	oldState := &sessionTransportState{
		transport: oldTransport,
		rc:        oldRC,
		config:    sessionTransportConfig{peerID: oldPeerID},
	}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = oldState
		bcast()
	})

	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	oldDone := make(chan error, 1)
	go func() {
		oldDone <- classifySessionTransportReadyError(
			ctx,
			oldState,
			context.DeadlineExceeded,
			func(err error) error {
				close(cleanupStarted)
				<-releaseCleanup
				return acc.cleanupSessionTransportReadyError(ctx, oldState, err)
			},
		)
	}()
	select {
	case <-cleanupStarted:
	case <-ctx.Done():
		t.Fatalf("old startup did not reach cleanup overlap: %v", ctx.Err())
	}

	newPriv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	newDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(ctx, newPriv, "", "")
		newDone <- err
	}()
	select {
	case err := <-newDone:
		if err != nil {
			t.Fatalf("new configuration failed to start: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("new configuration did not become ready: %v", ctx.Err())
	}
	close(releaseCleanup)

	select {
	case err := <-oldDone:
		if !errors.Is(err, errSessionTransportSuperseded) {
			t.Fatalf("superseded startup returned %v, want superseded error", err)
		}
	case <-ctx.Done():
		t.Fatalf("superseded startup did not return: %v", ctx.Err())
	}
}

func TestSessionTransportReplacementReturnsSupersededSignal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)

	st, err := transport.NewSessionTransport(acc.le, acc.t.p.b, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	rc := routine.NewRoutineContainer()
	sts := &sessionTransportState{
		transport: st,
		rc:        rc,
	}
	runCtx := t.Context()
	rc.SetRoutine(st.Execute)
	rc.SetContext(runCtx, false)

	rel, err := acc.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = sts
		bcast()
	})
	if err := acc.stopSessionTransportForReplacementLocked(ctx); err != nil {
		rel()
		t.Fatalf("stop session transport for replacement: %v", err)
	}
	rel()

	err = acc.waitSessionTransportReady(ctx, sts)
	if !errors.Is(err, errSessionTransportSuperseded) {
		t.Fatalf("replacement wait returned %v, want superseded signal", err)
	}
}

func TestSessionTransportReplacementReportsUncooperativeRoutine(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	stopMountedSessionTransportOwner(t, acc, sess)

	st, err := transport.NewSessionTransport(acc.le, acc.t.p.b, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	rc := routine.NewRoutineContainer()
	started := make(chan struct{})
	releaseRoutine := make(chan struct{})
	routineExited := make(chan struct{})
	rc.SetRoutine(func(context.Context) error {
		close(started)
		<-releaseRoutine
		close(routineExited)
		return nil
	})
	rc.SetContext(ctx, false)
	sts := &sessionTransportState{transport: st, rc: rc}
	acc.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.sessionTransport = sts
		bcast()
	})

	rel, err := acc.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		rel()
		t.Fatalf("old routine did not start: %v", ctx.Err())
	}
	stopCtx, stopCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	err = acc.stopSessionTransportForReplacementLocked(stopCtx)
	stopCancel()
	rel()
	if err == nil {
		t.Fatal("replacement returned nil while old routine ignored cancellation")
	}
	var current *sessionTransportState
	acc.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = acc.sessionTransport
	})
	if current != sts {
		t.Fatal("replacement removed the current state after an unconfirmed stop")
	}

	close(releaseRoutine)
	select {
	case <-routineExited:
	case <-ctx.Done():
		t.Fatalf("old routine did not exit after release: %v", ctx.Err())
	}
}

func TestSupersededSessionTransportCreatorKeepsNewConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	if _, _, err := acc.ensureSessionTransport(ctx, sess.GetPrivKey(), "", ""); err != nil {
		t.Fatalf("settle session transport startup: %v", err)
	}

	gate := newStartupGate()
	acc.t.p.localNetwork = gate.option()
	defer gate.release()

	oldPriv := sess.GetPrivKey()
	newPriv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	oldDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(ctx, oldPriv, unreachableSignalingURL, "old")
		oldDone <- err
	}()

	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatalf("old transport did not reach the startup gate: %v", ctx.Err())
	}

	newDone := make(chan error, 1)
	go func() {
		_, _, err := acc.ensureSessionTransport(ctx, newPriv, "", "")
		newDone <- err
	}()
	select {
	case err := <-newDone:
		if err != nil {
			t.Fatalf("new transport failed to start: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("new transport did not become ready: %v", ctx.Err())
	}

	select {
	case err := <-oldDone:
		if !errors.Is(err, errSessionTransportSuperseded) {
			t.Fatalf("superseded creator returned %v, want superseded error", err)
		}
	case <-ctx.Done():
		t.Fatalf("superseded creator did not return: %v", ctx.Err())
	}

	var current *sessionTransportState
	acc.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = acc.sessionTransport
	})
	if current == nil {
		t.Fatal("superseded creator removed the newer transport")
	}
	newPeerID, err := peer.IDFromPrivateKey(newPriv)
	if err != nil {
		t.Fatal(err)
	}
	if !current.config.matches(newPeerID, "", "") {
		t.Fatalf("current transport configuration = %+v, want peer %s with empty relay", current.config, newPeerID)
	}
}

func stopMountedSessionTransportOwner(t *testing.T, acc *ProviderAccount, sess *Session) {
	t.Helper()
	_, sessionOwnerExited := sess.tkr.sessionProm.GetPromise()
	if !acc.sessions.RemoveKey(sess.tkr.id) {
		t.Fatal("mounted session owner was not running")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-sessionOwnerExited:
	case <-ctx.Done():
		t.Fatalf("mounted session owner did not stop: %v", ctx.Err())
	}
	acc.StopSessionTransport()
}

// unreachableSignalingURL refuses connections. Tests use it to give a
// transport a distinct signaling configuration; signaling retries in the
// background without affecting readiness.
const unreachableSignalingURL = "http://127.0.0.1:1"

// startupGate holds the first session transport that starts through it in
// startup until release. Later transports start without waiting.
type startupGate struct {
	started     chan struct{}
	canceled    chan struct{}
	releaseCh   chan struct{}
	first       atomic.Bool
	releaseOnce sync.Once
}

func newStartupGate() *startupGate {
	return &startupGate{
		started:   make(chan struct{}),
		canceled:  make(chan struct{}, 1),
		releaseCh: make(chan struct{}),
	}
}

// option returns the local transport option that waits on the gate.
func (g *startupGate) option() transport.SessionTransportOption {
	return transport.WithLocalTransport(func(ctx context.Context, _ *logrus.Entry, _ bus.Bus, _ peer.ID) (*transport_controller.Controller, func(), error) {
		if g.first.Swap(true) {
			return nil, func() {}, nil
		}
		close(g.started)
		select {
		case <-g.releaseCh:
			return nil, func() {}, nil
		case <-ctx.Done():
			g.canceled <- struct{}{}
			return nil, nil, ctx.Err()
		}
	})
}

// release lets the held startup continue.
func (g *startupGate) release() {
	g.releaseOnce.Do(func() {
		close(g.releaseCh)
	})
}

func configureLowCostPINLock(ctx context.Context, t *testing.T, sess *Session, pin []byte) {
	t.Helper()

	privPEM, err := keypem.MarshalPrivKeyPem(sess.sessionPriv)
	if err != nil {
		t.Fatal(err)
	}
	defer scrub.Scrub(privPEM)

	encPriv, encSymKey, config, err := createLowCostPINLock(privPEM, pin)
	if err != nil {
		t.Fatal(err)
	}
	if err := session_lock.WritePINLock(ctx, sess.objStore, sess.tkr.id, encPriv, encSymKey, config); err != nil {
		t.Fatal(err)
	}

	sess.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		sess.lockMode = session_lock.SessionLockMode_PIN_ENCRYPTED
		broadcast()
	})
	sess.updateSessionMetadata(ctx, core_session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED)
}

func createLowCostPINLock(privPEM, pin []byte) (encPriv, encSymKey []byte, config *session_lock.LockConfig, err error) {
	var symKey [32]byte
	if _, err := rand.Read(symKey[:]); err != nil {
		return nil, nil, nil, err
	}
	defer scrub.Scrub(symKey[:])

	symMethod, err := blockenc.NewXChaCha20Poly1305(symKey[:])
	if err != nil {
		return nil, nil, nil, err
	}
	encPriv, err = symMethod.Encrypt(blockenc.DefaultAllocFn(), privPEM)
	if err != nil {
		return nil, nil, nil, err
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, nil, err
	}
	config = &session_lock.LockConfig{ScryptN: 1, Salt: salt}
	pinKey, err := deriveLowCostPINKey(config, pin)
	if err != nil {
		return nil, nil, nil, err
	}
	defer scrub.Scrub(pinKey)

	pinMethod, err := blockenc.NewXChaCha20Poly1305(pinKey)
	if err != nil {
		return nil, nil, nil, err
	}
	encSymKey, err = pinMethod.Encrypt(blockenc.DefaultAllocFn(), symKey[:])
	if err != nil {
		return nil, nil, nil, err
	}

	return encPriv, encSymKey, config, nil
}

func deriveLowCostPINKey(config *session_lock.LockConfig, pin []byte) ([]byte, error) {
	var passKey [32]byte
	blake3.DeriveKey("aperture/alpha 2026-03-16 session-lock pin-kdf v2", pin, passKey[:])
	return scrypt.Key(passKey[:], config.Salt, 1<<config.ScryptN, 8, 1, 32)
}

func setupProviderAndSessionInternal(ctx context.Context, t *testing.T, configure ...func(*Provider)) (
	*testbed.Testbed,
	*core_session.SessionRef,
	*ProviderAccount,
	*Session,
	func(),
) {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	providerID := "local"
	peerID := tb.Volume.GetPeerID()
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	localProv := prov.(*Provider)
	for _, apply := range configure {
		apply(localProv)
	}
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	acc := accIface.(*ProviderAccount)

	sess, sessRelease, err := acc.MountSession(ctx, sessRef, nil)
	if err != nil {
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	localSess := sess.(*Session)

	release := func() {
		sessRelease()
		accRel()
		provRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
	return tb, sessRef, acc, localSess, release
}
