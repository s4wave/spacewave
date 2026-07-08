package provider_local

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/scrub"
	"github.com/s4wave/spacewave/core/provider"
	core_session "github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto/blake3"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/testbed"
	"golang.org/x/crypto/scrypt"
)

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
	ctx := t.Context()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	acc.StopSessionTransport()

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
	<-done
}

func TestEnsureSessionTransportRetriesWhenExistingTransportClearsBeforeReady(t *testing.T) {
	ctx := t.Context()
	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()
	acc.StopSessionTransport()

	fakeTransport, err := transport.NewSessionTransport(acc.le, acc.t.p.b, sess.GetPrivKey(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	fakeState := &sessionTransportState{transport: fakeTransport}
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

	completeCtx, completeCancel := context.WithTimeout(ctx, time.Second)
	defer completeCancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ensure session transport after cleared state: %v", err)
		}
	case <-completeCtx.Done():
		cancel()
		err := <-done
		t.Fatalf("ensure session transport remained blocked after cleared state: %v", err)
	}
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

func setupProviderAndSessionInternal(ctx context.Context, t *testing.T) (
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
