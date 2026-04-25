package resource_account

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/core/session"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// TestEntityKeypairsWatchStateCoalescesNearSimultaneousChanges asserts that
// an account-state update and a tracker unlock landing while the loop is
// mid-emission coalesce into a single follow-on emission. The unified
// broadcast pattern reads both inputs under the same HoldLock that obtains
// the wait channel, so updates that race with the loop's send fold into
// the next read instead of producing one extra emission per source.
func TestEntityKeypairsWatchStateCoalescesNearSimultaneousChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_, pid1, _ := generateEntityKey(t)
	_, pid2, _ := generateEntityKey(t)

	state := &entityKeypairsWatchState{
		keypairs: []*session.EntityKeypair{
			{PeerId: pid1.String(), AuthMethod: "password"},
		},
		valid:         true,
		unlockedPeers: map[peer.ID]bool{},
	}

	var (
		emissionsMu sync.Mutex
		emissions   []*s4wave_account.WatchEntityKeypairsResponse
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

	send := func(r *s4wave_account.WatchEntityKeypairsResponse) error {
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

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.runWatchLoop(ctx, send)
	}()

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

	close(released)
	<-emitted

	cancel()
	<-loopErr

	emissionsMu.Lock()
	defer emissionsMu.Unlock()

	if got := len(emissions); got != 2 {
		t.Fatalf("expected 2 emissions (initial + coalesced), got %d", got)
	}
	if got := len(emissions[1].GetKeypairs()); got != 2 {
		t.Fatalf("coalesced emission missing keypairs update: got %d, want 2", got)
	}
	if got := emissions[1].GetUnlockedCount(); got != 1 {
		t.Fatalf("coalesced emission missing tracker update: got UnlockedCount=%d, want 1", got)
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

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

	var (
		emissionsMu sync.Mutex
		emissions   int
	)
	released := make(chan struct{})
	emitted := make(chan struct{}, 8)

	send := func(r *s4wave_account.WatchEntityKeypairsResponse) error {
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

	loopErr := make(chan error, 1)
	go func() {
		loopErr <- state.runWatchLoop(ctx, send)
	}()

	<-emitted

	for range 2 {
		state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			state.keypairs = makeKeypairs()
			state.unlockedPeers = map[peer.ID]bool{}
			broadcast()
		})
	}

	close(released)

	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.unlockedPeers = map[peer.ID]bool{pid1: true}
		broadcast()
	})

	<-emitted

	cancel()
	<-loopErr

	emissionsMu.Lock()
	defer emissionsMu.Unlock()

	if emissions != 2 {
		t.Fatalf("expected 2 emissions (initial + one real change after duplicate writes), got %d", emissions)
	}
}

func generateEntityKey(t *testing.T) (bifrost_crypto.PrivKey, peer.ID, ed25519.PrivateKey) {
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
