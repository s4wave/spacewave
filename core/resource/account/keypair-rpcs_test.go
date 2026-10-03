package resource_account

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unsafe"

	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// TestEntityKeypairsWatchStateCoalescesNearSimultaneousChanges asserts that
// an account-state update and a key-store unlock landing while the loop is
// mid-emission coalesce into a single follow-on emission. The unified
// broadcast pattern reads both inputs under the same HoldLock that obtains
// the wait channel, so updates that race with the loop's send fold into
// the next read instead of producing one extra emission per source.
func TestEntityKeypairsWatchStateCoalescesNearSimultaneousChanges(t *testing.T) {
	// Bind the keypair watch loop to a cancelable test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Prepare a watch snapshot with one locked entity keypair.
	_, pid1, _ := generateEntityKey(t)
	_, pid2, _ := generateEntityKey(t)
	state := &entityKeypairsWatchState{
		keypairs: []*session.EntityKeypair{
			{PeerId: pid1.String(), AuthMethod: "password"},
		},
		valid:         true,
		unlockedPeers: map[peer.ID]bool{},
	}

	// Record keypair watch emissions and gate the initial snapshot.
	var (
		emissionsMu sync.Mutex
		emissions   []*s4wave_account.WatchEntityKeypairsResponse
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)
	send := func(r *s4wave_account.WatchEntityKeypairsResponse) error {
		// Capture each keypair snapshot and hold its first emission for concurrent updates.
		emissionsMu.Lock()
		idx := len(emissions)
		emissions = append(emissions, r)
		emissionsMu.Unlock()
		emitted <- struct{}{}
		if idx == 0 {
			<-released
		}
		return nil
	}

	// Run the keypair watch loop and retain its terminal result.
	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.runWatchLoop(ctx, send)
	}()

	// Publish keypair and unlock updates while the initial snapshot is blocked.
	<-emitted
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.keypairs = []*session.EntityKeypair{
			{PeerId: pid1.String(), AuthMethod: "password"},
			{PeerId: pid2.String(), AuthMethod: "password"},
		}
		broadcast()
	})
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.unlockedPeers = map[peer.ID]bool{pid1: true}
		broadcast()
	})

	// Release the initial snapshot and await the coalesced keypair update.
	close(released)
	<-emitted

	// Stop the keypair watch loop before inspecting its emissions.
	cancel()
	<-loopErr

	// Verify that the coalesced snapshot contains both keypair and unlock updates.
	emissionsMu.Lock()
	defer emissionsMu.Unlock()
	if got := len(emissions); got != 2 {
		t.Fatalf("expected 2 emissions (initial + coalesced), got %d", got)
	}
	if got := len(emissions[1].GetKeypairs()); got != 2 {
		t.Fatalf("coalesced emission missing keypairs update: got %d, want 2", got)
	}
	if got := emissions[1].GetUnlockedCount(); got != 1 {
		t.Fatalf("coalesced emission missing key-store update: got UnlockedCount=%d, want 1", got)
	}
	var pid1Unlocked bool
	for _, kp := range emissions[1].GetKeypairs() {
		if kp.GetKeypair().GetPeerId() == pid1.String() && kp.GetUnlocked() {
			pid1Unlocked = true
		}
	}
	if !pid1Unlocked {
		t.Fatal("coalesced emission did not mark pid1 as unlocked")
	}
}

// TestEntityKeypairsWatchStateEqualityGateSuppressesDuplicates asserts the
// EqualVT suppression at the emit boundary catches the case where a source
// broadcasts an update whose computed snapshot is byte-identical to the
// previous emission. Two no-op writes (same value as the current snapshot)
// must not produce any extra emission; a subsequent real change must still
// emit so the test cannot pass via a stuck loop.
func TestEntityKeypairsWatchStateEqualityGateSuppressesDuplicates(t *testing.T) {
	// Bind the duplicate-suppression watch loop to a cancelable test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Prepare a stable locked-keypair snapshot for repeated writes.
	_, pid1, _ := generateEntityKey(t)
	makeKeypairs := func() []*session.EntityKeypair {
		return []*session.EntityKeypair{
			{PeerId: pid1.String(), AuthMethod: "password"},
		}
	}
	state := &entityKeypairsWatchState{
		keypairs:      makeKeypairs(),
		valid:         true,
		unlockedPeers: map[peer.ID]bool{},
	}

	// Count keypair watch emissions and gate the initial snapshot.
	var (
		emissionsMu sync.Mutex
		emissions   int
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)
	send := func(r *s4wave_account.WatchEntityKeypairsResponse) error {
		// Count each keypair snapshot and hold its first emission for duplicate writes.
		emissionsMu.Lock()
		idx := emissions
		emissions++
		emissionsMu.Unlock()
		emitted <- struct{}{}
		if idx == 0 {
			<-released
		}
		return nil
	}

	// Run the keypair watch loop and retain its terminal result.
	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.runWatchLoop(ctx, send)
	}()

	// Publish unchanged keypair snapshots while the first emission is blocked.
	<-emitted
	for range 2 {
		state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			state.keypairs = makeKeypairs()
			state.unlockedPeers = map[peer.ID]bool{}
			broadcast()
		})
	}

	// Resume the keypair watch and publish a real unlock change.
	close(released)
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.unlockedPeers = map[peer.ID]bool{pid1: true}
		broadcast()
	})
	<-emitted

	// Stop the keypair watch loop before inspecting its emission count.
	cancel()
	<-loopErr

	// Verify that duplicate snapshots produce no extra watch emission.
	emissionsMu.Lock()
	defer emissionsMu.Unlock()
	if emissions != 2 {
		t.Fatalf("expected 2 emissions (initial + one real change after duplicate writes), got %d", emissions)
	}
}

func TestSignWithEntityKeypairUsesUnlockedStoreAndRejectsLockedOrMissingKey(t *testing.T) {
	// Prepare an account Resource with one unlocked entity signing key.
	ctx := context.Background()
	priv, pid, stdPriv := generateEntityKey(t)
	_, missingPID, _ := generateEntityKey(t)
	store := provider_spacewave.NewEntityKeyStore()
	store.Unlock(pid, priv)
	r := &AccountResource{account: providerAccountWithEntityKeyStore(t, store)}
	payload := []byte("cloud admin signed payload")

	// Verify that the unlocked entity key signs a valid payload.
	resp, err := r.SignWithEntityKeypair(ctx, &s4wave_account.SignWithEntityKeypairRequest{
		PeerId:  pid.String(),
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("sign unlocked keypair: %v", err)
	}
	if !ed25519.Verify(stdPriv.Public().(ed25519.PublicKey), payload, resp.GetSignature()) {
		t.Fatal("signature did not verify with the unlocked entity public key")
	}

	// Verify that a missing entity key cannot sign the payload.
	_, err = r.SignWithEntityKeypair(ctx, &s4wave_account.SignWithEntityKeypairRequest{
		PeerId:  missingPID.String(),
		Payload: payload,
	})
	assertLockedEntityKeypairError(t, err)

	// Lock the entity key and verify that it can no longer sign.
	if _, err := r.LockEntityKeypair(ctx, &s4wave_account.LockEntityKeypairRequest{PeerId: pid.String()}); err != nil {
		t.Fatalf("lock keypair: %v", err)
	}
	_, err = r.SignWithEntityKeypair(ctx, &s4wave_account.SignWithEntityKeypairRequest{
		PeerId:  pid.String(),
		Payload: payload,
	})
	assertLockedEntityKeypairError(t, err)
}

func providerAccountWithEntityKeyStore(t *testing.T, store *provider_spacewave.EntityKeyStore) *provider_spacewave.ProviderAccount {
	// Attach the supplied entity key store to a test provider account.
	t.Helper()
	acc := &provider_spacewave.ProviderAccount{}
	field := reflect.ValueOf(acc).Elem().FieldByName("entityKeyStore")
	if !field.IsValid() {
		t.Fatal("ProviderAccount entityKeyStore field not found")
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(store))
	return acc
}

func assertLockedEntityKeypairError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected locked entity keypair error")
	}
	if !strings.Contains(err.Error(), "entity keypair is locked") {
		t.Fatalf("locked entity keypair error: got %q", err.Error())
	}
}

func generateEntityKey(t *testing.T) (bifrost_crypto.PrivKey, peer.ID, ed25519.PrivateKey) {
	// Generate an entity signing key and its peer identity for the test.
	t.Helper()
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("deriving peer ID: %v", err)
	}
	std := priv.(interface{ GetStdKey() ed25519.PrivateKey }).GetStdKey()
	return priv, pid, std
}
