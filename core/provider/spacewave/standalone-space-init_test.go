package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestBuildFriendDmInitialStateIncludesBilateralRecovery(t *testing.T) {
	// Describe two friend DM accounts with sessions and recovery keypairs.
	localPriv, localPID := generateTestKeypair(t)
	localSecondPriv, localSecondPID := generateTestKeypair(t)
	localRecoveryPriv, localRecoveryPID := generateTestKeypair(t)
	targetPriv, targetPID := generateTestKeypair(t)
	targetRecoveryPriv, targetRecoveryPID := generateTestKeypair(t)
	accounts := []*api.FriendDmAccount{
		{
			AccountId: "acct-a",
			EntityId:  "alice",
			Sessions: []*api.FriendDmSessionPeer{
				{PeerId: localPID.String()},
				{PeerId: localSecondPID.String()},
			},
			RecoveryKeypairs: []*api.FriendDmRecoveryPeer{
				{PeerId: localRecoveryPID.String()},
			},
		},
		{
			AccountId:        "acct-b",
			EntityId:         "bob",
			Sessions:         []*api.FriendDmSessionPeer{{PeerId: targetPID.String()}},
			RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: targetRecoveryPID.String()}},
		},
	}

	// Build the friend DM initial state owned by alice.
	state, err := buildStandaloneSpaceInitState(
		context.Background(),
		nil,
		logrus.New().WithField("test", "friend-dm"),
		"acct-a",
		"",
		"friend-dm-so",
		localPriv,
		buildStandaloneSpaceInitStepFactorySet(),
		true,
		accounts,
	)
	if err != nil {
		t.Fatalf("build friend dm state: %v", err)
	}

	// Decode the genesis participants.
	change := &sobject.SOConfigChange{}
	if err := change.UnmarshalVT(state.configData); err != nil {
		t.Fatalf("unmarshal genesis config: %v", err)
	}
	participants := change.GetConfig().GetParticipants()
	if len(participants) != 3 {
		t.Fatalf("participants = %d, want 3", len(participants))
	}

	// Check each participant's role and username.
	rolesByPeer := make(map[string]sobject.SOParticipantRole, len(participants))
	wantUsernames := map[string]string{"acct-a": "alice", "acct-b": "bob"}
	for _, participant := range participants {
		rolesByPeer[participant.GetPeerId()] = participant.GetRole()
		if want := wantUsernames[participant.GetEntityId()]; participant.GetUsername() != want {
			t.Fatalf("participant %s username = %q, want %q", participant.GetPeerId(), participant.GetUsername(), want)
		}
	}
	if rolesByPeer[localPID.String()] !=
		sobject.SOParticipantRole_SOParticipantRole_OWNER ||
		rolesByPeer[localSecondPID.String()] !=
			sobject.SOParticipantRole_SOParticipantRole_OWNER ||
		rolesByPeer[targetPID.String()] !=
			sobject.SOParticipantRole_SOParticipantRole_WRITER {
		t.Fatalf("unexpected participant roles: %#v", rolesByPeer)
	}

	// Check that every participant has a grant.
	if len(state.keyEpoch.GetGrants()) != len(participants) {
		t.Fatalf("grants = %d, want %d", len(state.keyEpoch.GetGrants()), len(participants))
	}
	for _, participant := range participants {
		if state.keyEpoch.FindGrant(participant.GetPeerId()) == nil {
			t.Fatalf("missing grant for %s", participant.GetPeerId())
		}
	}

	// Check that only each account's recovery key unlocks its envelope.
	if len(state.recoveryEnvelopes) != 2 {
		t.Fatalf("recovery envelopes = %d, want 2", len(state.recoveryEnvelopes))
	}
	recoveryPrivsByAccount := map[string][]crypto.PrivKey{
		"acct-a": {localRecoveryPriv},
		"acct-b": {targetRecoveryPriv},
	}
	sessionPrivsByAccount := map[string][]crypto.PrivKey{
		"acct-a": {localPriv, localSecondPriv},
		"acct-b": {targetPriv},
	}
	for _, env := range state.recoveryEnvelopes {
		if _, err := sobject.UnlockSOEntityRecoveryEnvelope(
			sessionPrivsByAccount[env.GetEntityId()],
			env,
		); err == nil {
			t.Fatalf("session key unexpectedly unlocked recovery envelope %s", env.GetEntityId())
		}
		material, err := sobject.UnlockSOEntityRecoveryEnvelope(
			recoveryPrivsByAccount[env.GetEntityId()],
			env,
		)
		if err != nil {
			t.Fatalf("unlock recovery envelope %s: %v", env.GetEntityId(), err)
		}
		if material.GetEntityId() != env.GetEntityId() {
			t.Fatalf("recovery entity = %q, want %q", material.GetEntityId(), env.GetEntityId())
		}
	}

	// Check the config and checkpoint request wrappers.
	configData, checkpointData, err := marshalFriendDmInitialState(state)
	if err != nil {
		t.Fatalf("marshal friend dm wrappers: %v", err)
	}
	configRequest := &api.PostConfigStateRequest{}
	if err := configRequest.UnmarshalVT(configData); err != nil {
		t.Fatalf("unmarshal config wrapper: %v", err)
	}
	if len(configRequest.GetRecoveryEnvelopes()) != 2 ||
		configRequest.GetKeyEpoch() == nil ||
		len(configRequest.GetKeyEpoch().GetGrants()) != 3 {
		t.Fatalf("incomplete config wrapper: %+v", configRequest)
	}

	// The checkpoint wrapper holds the genesis checkpoint.
	checkpointRequest := &api.PostCheckpointRequest{}
	if err := checkpointRequest.UnmarshalVT(checkpointData); err != nil {
		t.Fatalf("unmarshal checkpoint wrapper: %v", err)
	}
	inner, err := checkpointRequest.GetCheckpoint().UnmarshalInner()
	if err != nil || inner.GetHeight() != 0 {
		t.Fatalf("incomplete checkpoint wrapper: %+v: %v", checkpointRequest, err)
	}
}

func TestSessionClientInitEmptyStandaloneSpace(t *testing.T) {
	// Describe an owner config without a checkpoint.
	const (
		soID      = "so-standalone-init"
		accountID = "test-account"
	)
	localPriv, localPID := generateTestKeypair(t)
	otherPriv, _ := generateTestKeypair(t)
	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId:   localPID.String(),
				Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
				EntityId: accountID,
			}},
		},
	}
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainData := mustMarshalVT(t, &sobject.SOConfigChainResponse{})
	keypairResp := buildRecoveryKeypairResponse(t, accountID, otherPriv)
	keypairData := mustMarshalVT(t, keypairResp)

	// Serve the object, recording the config, key epoch and checkpoint writes.
	var (
		postedConfig     *api.PostConfigStateRequest
		postedCheckpoint *sobject.SOCheckpoint
		postedEpoch      *sobject.SOKeyEpoch
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainData)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairData)
		case "/api/sobject/" + soID + "/config-state":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read config-state body: %v", err)
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal config-state request: %v", err)
			}
			postedConfig = req
			postedEpoch = req.GetKeyEpoch()
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/checkpoint":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read checkpoint body: %v", err)
			}
			req := &api.PostCheckpointRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal post checkpoint request: %v", err)
			}
			postedCheckpoint = req.GetCheckpoint()
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	// Initialize the Space through a session client.
	cli := newStandaloneInitTestClient(srv.URL, localPriv, localPID)
	changed, err := cli.InitEmptyStandaloneSpace(
		context.Background(),
		nil,
		accountID,
		"",
		soID,
	)
	if err != nil {
		t.Fatalf("InitEmptyStandaloneSpace: %v", err)
	}
	if !changed {
		t.Fatal("expected init mutation")
	}

	// Check that the config, checkpoint and key epoch were written.
	if postedConfig == nil {
		t.Fatal("expected config-state write")
	}
	if postedCheckpoint == nil {
		t.Fatal("expected checkpoint write")
	}
	if postedEpoch == nil {
		t.Fatal("expected key-epoch write")
	}

	// Check the genesis owner, the genesis checkpoint and the owner grant.
	change := &sobject.SOConfigChange{}
	if err := change.UnmarshalVT(postedConfig.GetConfigChange()); err != nil {
		t.Fatalf("unmarshal posted config change: %v", err)
	}
	if change.GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS {
		t.Fatalf("change type = %v", change.GetChangeType())
	}
	got := participantConfigForPeer(change.GetConfig(), localPID.String())
	if got == nil {
		t.Fatal("expected local owner participant in genesis config")
	}
	if got.GetEntityId() != accountID {
		t.Fatalf("entity id = %q", got.GetEntityId())
	}

	// The posted checkpoint is the genesis.
	inner, err := postedCheckpoint.UnmarshalInner()
	if err != nil || inner.GetHeight() != 0 {
		t.Fatalf("posted checkpoint is not genesis: %v", err)
	}
	if postedEpoch.FindGrant(localPID.String()) == nil {
		t.Fatal("expected local owner grant in posted key epoch")
	}
}

func TestSessionClientInitEmptyStandaloneSpaceRejectsGrantlessCheckpoint(t *testing.T) {
	// Describe an initialized Space whose key epoch has no grants.
	const (
		soID      = "so-standalone-grantless"
		accountID = "test-account"
	)
	localPriv, localPID := generateTestKeypair(t)
	state, _, err := sobject.BuildGenesisSOState(logrus.NewEntry(logrus.New()), testStepFactorySet(), soID, localPriv, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.KeyEpochs = []*sobject.SOKeyEpoch{{}}
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainData := mustMarshalVT(t, &sobject.SOConfigChainResponse{KeyEpochs: state.KeyEpochs})

	// Serve only reads; any write would replace the initialized Space.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainData)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	// Initialization refuses to replace the Space.
	cli := newStandaloneInitTestClient(srv.URL, localPriv, localPID)
	changed, err := cli.InitEmptyStandaloneSpace(context.Background(), nil, accountID, "", soID)
	if err == nil || changed {
		t.Fatalf("InitEmptyStandaloneSpace = %t, %v; want a missing grant error", changed, err)
	}
}

// newStandaloneInitTestClient returns a session client for endpoint that writes
// with a fixed write ticket.
func newStandaloneInitTestClient(endpoint string, priv crypto.PrivKey, pid peer.ID) *SessionClient {
	cli := NewSessionClient(http.DefaultClient, endpoint, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-init-root")
	}
	return cli
}

func buildRecoveryKeypairResponse(
	t *testing.T,
	entityID string,
	entityPriv crypto.PrivKey,
) *api.ListSORecoveryEntityKeypairsResponse {
	t.Helper()

	entityPID, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		t.Fatalf("derive recovery peer id: %v", err)
	}
	return &api.ListSORecoveryEntityKeypairsResponse{
		Entities: []*api.SORecoveryEntityKeypairs{{
			EntityId: entityID,
			Keypairs: []*session.EntityKeypair{{
				PeerId: entityPID.String(),
			}},
		}},
	}
}
