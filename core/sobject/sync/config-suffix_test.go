package sobject_sync

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestRefusedParticipantAdoptsConfigSuffix covers a participant whose config
// predates the key of a peer that holds a newer config: the peer offers its
// changes on the refused stream, and the next stream admits it only when an
// owner of the held config signed them.
func TestRefusedParticipantAdoptsConfigSuffix(t *testing.T) {
	for _, test := range []struct {
		// name identifies the offered history.
		name string
		// forged signs the addition with the added key instead of the owner.
		forged bool
		// unrelated starts the offered history at another owner's genesis.
		unrelated bool
	}{
		{name: "owner signed"},
		{name: "forged", forged: true},
		{name: "unrelated", unrelated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Bound the test and make the three keys.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			const soID = "authentication-config-suffix"
			owner, stale, added := mustKeyPair(t), mustKeyPair(t), mustKeyPair(t)

			// The stale participant holds the genesis config.
			staleState, staleGenesis := suffixGenesisState(t, soID, owner, stale)
			staleHost, _ := newMemHostRetaining(soID, staleState.CloneVT(), []*sobject.SOConfigChange{staleGenesis})
			t.Cleanup(staleHost.ClearContext)
			stalePeer := NewSOSync(gateLogger(), nil, soID, mustPeerID(t, stale), stale, staleHost, nopPeerObserver{})

			// The added key holds a config one change later that lists it.
			aheadState, aheadGenesis := staleState.CloneVT(), staleGenesis
			signer := owner
			if test.unrelated {
				signer = mustKeyPair(t)
				aheadState, aheadGenesis = suffixGenesisState(t, soID, signer, stale)
			}
			if test.forged {
				signer = added
			}
			addition := suffixAddReader(t, soID, aheadState, added, signer)
			aheadHost, _ := newMemHostRetaining(soID, aheadState, []*sobject.SOConfigChange{aheadGenesis, addition})
			t.Cleanup(aheadHost.ClearContext)
			aheadPeer := NewSOSync(gateLogger(), nil, soID, mustPeerID(t, added), added, aheadHost, nopPeerObserver{})

			// The first stream is refused by the stale side.
			runSuffixStream(t, ctx, aheadPeer, stalePeer, true)

			// Only an owner-signed extension of the held head is adopted.
			held, err := staleHost.GetHostState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			adopt := !test.forged && !test.unrelated
			if adopt && !sobject.EqualSOConfigs(held.GetConfig(), aheadState.GetConfig()) {
				t.Fatalf("stale config seqno = %d, want the offered seqno %d", held.GetConfig().GetConfigChainSeqno(), aheadState.GetConfig().GetConfigChainSeqno())
			}
			if !adopt && !held.GetConfig().EqualVT(staleState.GetConfig()) {
				t.Fatal("stale participant adopted a config its owners did not sign")
			}

			// The next stream is admitted exactly when the changes were adopted.
			runSuffixStream(t, ctx, aheadPeer, stalePeer, !adopt)
		})
	}
}

// suffixGenesisState builds a held genesis config with an owner and a reader
// and returns the genesis change behind it.
func suffixGenesisState(t *testing.T, soID string, owner, reader crypto.PrivKey) (*sobject.SOState, *sobject.SOConfigChange) {
	// Sign the genesis change for the owner and the reader.
	t.Helper()
	config := &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		participantCfg(mustPeerIDStr(t, owner), sobject.SOParticipantRole_SOParticipantRole_OWNER),
		participantCfg(mustPeerIDStr(t, reader), sobject.SOParticipantRole_SOParticipantRole_READER),
	}}
	genesis, err := sobject.BuildSOConfigChange(soID, config, config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Hold the config it yields and its genesis checkpoint.
	state := &sobject.SOState{}
	state.Config, err = sobject.VerifyConfigChange(soID, config, genesis)
	if err != nil {
		t.Fatal(err)
	}
	state.Checkpoint, err = sobject.BuildGenesisSOCheckpoint(owner, soID, state.Config.GetConfigChainHash(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return state, genesis
}

// suffixAddReader signs, as signer, the addition of reader to the config state
// holds, applies it to state without an authority check, and returns it.
func suffixAddReader(t *testing.T, soID string, state *sobject.SOState, reader, signer crypto.PrivKey) *sobject.SOConfigChange {
	// Sign the config that adds the reader.
	t.Helper()
	next := state.GetConfig().CloneVT()
	next.Participants = append(next.Participants, participantCfg(mustPeerIDStr(t, reader), sobject.SOParticipantRole_SOParticipantRole_READER))
	change, err := sobject.BuildSOConfigChange(soID, state.GetConfig(), next, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, signer, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Hold the change's result, whoever signed it.
	next.ConfigChainHash, err = sobject.HashSOConfigChange(change)
	if err != nil {
		t.Fatal(err)
	}
	next.ConfigChainSeqno = change.GetConfigSeqno()
	state.Config = next
	return change
}

// runSuffixStream runs one real stream between the two peers and checks
// whether the stale side refused it.
func runSuffixStream(t *testing.T, ctx context.Context, ahead, stale *SOSync, refused bool) {
	// Connect the two stream owners over an observed pipe.
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 64)}

	// Run both owners, and end an admitted stream once data flows.
	done := make(chan error, 2)
	go func() { done <- ahead.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
	go func() { done <- stale.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	if !refused {
		waitAuthenticationData(t, ctx, observed.messages)
		cancel()
	}

	// Join both owners and check the refusal.
	for range 2 {
		select {
		case err := <-done:
			if refused && !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("refused stream result = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("stream owner failed to join")
		}
	}

	// A refused stream carries no object data.
	for len(observed.messages) > 0 {
		message := <-observed.messages
		if message.GetSnapshot() != nil || message.GetHead() != nil || message.GetHistoryPage() != nil {
			if refused {
				t.Fatal("refused stream carried object data")
			}
		}
	}
}

// mustPeerID returns the identity belonging to the signing key.
func mustPeerID(t *testing.T, priv crypto.PrivKey) peer.ID {
	t.Helper()
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
