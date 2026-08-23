package provider_spacewave

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
)

func TestMarshalFriendDmChannelWorldOp(t *testing.T) {
	_, sender := generateTestKeypair(t)
	data, err := marshalFriendDmChannelWorldOp(sender)
	if err != nil {
		t.Fatalf("marshalFriendDmChannelWorldOp: %v", err)
	}
	worldOp := &sobject_world_engine.SOWorldOp{}
	if err := worldOp.UnmarshalVT(data); err != nil {
		t.Fatalf("unmarshal world op: %v", err)
	}
	apply := worldOp.GetApplyTxOp()
	if apply == nil || apply.GetTx() == nil {
		t.Fatal("expected ApplyTxOp envelope")
	}
	tx := apply.GetTx()
	if tx.GetTxType() != world_block_tx.TxType_TxType_APPLY_WORLD_OP {
		t.Fatalf("tx type = %v", tx.GetTxType())
	}
	chatOp := &spacewave_chat.CreateChatChannelOp{}
	if err := chatOp.UnmarshalBlock(tx.GetTxApplyWorldOp().GetOperationBody()); err != nil {
		t.Fatalf("unmarshal chat operation: %v", err)
	}
	if chatOp.GetObjectKey() != FriendDmChannelObjectKey {
		t.Fatalf("object key = %q", chatOp.GetObjectKey())
	}
	if tx.GetTxApplyWorldOp().GetOpSender() != sender.String() {
		t.Fatalf("op sender = %q", tx.GetTxApplyWorldOp().GetOpSender())
	}
}

func TestOpenFriendDMHiddenTargetDoesNotMutate(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/account/friends/acct-b/dm" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeFriendDmResponse(t, w, &api.GetFriendDmResponse{})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	_, err := acc.OpenFriendDM(context.Background(), "acct-b")
	if !stderrors.Is(err, ErrFriendDmNotFound) {
		t.Fatalf("error = %v, want ErrFriendDmNotFound", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one Cloud authorization request", requests)
	}
}

func TestOpenFriendDMProposedOtherOwnerRefusesBeforeCreate(t *testing.T) {
	var requests int
	var localPeer string
	_, targetPeer := generateTestKeypair(t)
	local := &api.FriendDmAccount{AccountId: "test-account", EntityUuid: "entity-a"}
	target := &api.FriendDmAccount{
		AccountId:  "acct-b",
		EntityUuid: "entity-b",
		Sessions:   []*api.FriendDmSessionPeer{{PeerId: targetPeer.String()}},
	}
	sharedObjectID := deriveFriendDmSharedObjectID(local, target)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeFriendDmResponse(t, w, &api.GetFriendDmResponse{
			SharedObjectId: sharedObjectID,
			OwnerAccountId: "acct-b",
			OwnerType:      "account",
			Accounts: []*api.FriendDmAccount{
				{
					AccountId:        local.AccountId,
					EntityUuid:       local.EntityUuid,
					Sessions:         []*api.FriendDmSessionPeer{{PeerId: localPeer}},
					RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: localPeer}},
				},
				target,
			},
		})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	localPeer = acc.GetCurrentSessionPeerID().String()
	_, err := acc.OpenFriendDM(context.Background(), "acct-b")
	if err == nil {
		t.Fatal("expected proposed-other-owner refusal")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one GET and no create", requests)
	}
}

func TestOpenFriendDMInvalidResponseDoesNotMutate(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"sharedObjectId":"01frienddm","ready":false,"ownerAccountId":"acct-a","ownerType":"account","accounts":[{"accountId":"acct-a","entityUuid":"entity-a","epoch":1,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-a"}]},{"accountId":"acct-b","entityUuid":"entity-b","epoch":1,"sessions":[],"recoveryKeypairs":[{"peerId":"recovery-b"}]}]}`))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	_, err := acc.OpenFriendDM(context.Background(), "acct-b")
	if err == nil {
		t.Fatal("expected malformed Cloud response error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one Cloud authorization request", requests)
	}
}

func TestOpenFriendDMRejectsInvalidPeerBeforeInitialization(t *testing.T) {
	var requests int
	var response *api.GetFriendDmResponse
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if response != nil {
			writeFriendDmResponse(t, w, response)
			return
		}
		writeFriendDmResponse(t, w, &api.GetFriendDmResponse{})
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	localPeerID := acc.GetCurrentSessionPeerID().String()
	local := &api.FriendDmAccount{
		AccountId:        acc.GetAccountID(),
		EntityUuid:       "entity-a",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: localPeerID}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: localPeerID}},
	}
	target := &api.FriendDmAccount{
		AccountId:        "acct-b",
		EntityUuid:       "entity-b",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: "invalid-peer"}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: localPeerID}},
	}
	response = &api.GetFriendDmResponse{
		SharedObjectId: deriveFriendDmSharedObjectID(local, target),
		OwnerAccountId: local.AccountId,
		OwnerType:      "account",
		Accounts:       []*api.FriendDmAccount{local, target},
	}
	if _, err := acc.OpenFriendDM(context.Background(), target.AccountId); err == nil {
		t.Fatal("expected invalid peer error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one Cloud request and no initialization", requests)
	}
}

func TestOpenFriendDMRejectsNoncanonicalIDBeforeInitialization(t *testing.T) {
	var requests int
	var response *api.GetFriendDmResponse
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if response != nil {
			writeFriendDmResponse(t, w, response)
			return
		}
		writeFriendDmResponse(t, w, &api.GetFriendDmResponse{})
	}))
	defer srv.Close()

	_, targetPeer := generateTestKeypair(t)
	acc := NewTestProviderAccount(t, srv.URL)
	localPeerID := acc.GetCurrentSessionPeerID().String()
	local := &api.FriendDmAccount{
		AccountId:        acc.GetAccountID(),
		EntityUuid:       "entity-a",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: localPeerID}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: localPeerID}},
	}
	target := &api.FriendDmAccount{
		AccountId:        "acct-b",
		EntityUuid:       "entity-b",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: targetPeer.String()}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: targetPeer.String()}},
	}
	response = &api.GetFriendDmResponse{
		SharedObjectId: "wrong-id",
		OwnerAccountId: local.AccountId,
		OwnerType:      "account",
		Accounts:       []*api.FriendDmAccount{local, target},
	}
	if _, err := acc.OpenFriendDM(context.Background(), target.AccountId); err == nil {
		t.Fatal("expected noncanonical ID error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one Cloud request and no initialization", requests)
	}
}

func TestValidateFriendDmBootstrapAllowsEpochZero(t *testing.T) {
	_, localPeer := generateTestKeypair(t)
	_, targetPeer := generateTestKeypair(t)
	local := &api.FriendDmAccount{
		AccountId:        "acct-a",
		EntityUuid:       "entity-a",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: localPeer.String()}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: localPeer.String()}},
	}
	target := &api.FriendDmAccount{
		AccountId:        "acct-b",
		EntityUuid:       "entity-b",
		Sessions:         []*api.FriendDmSessionPeer{{PeerId: targetPeer.String()}},
		RecoveryKeypairs: []*api.FriendDmRecoveryPeer{{PeerId: targetPeer.String()}},
	}
	bootstrap := &api.GetFriendDmResponse{
		SharedObjectId: deriveFriendDmSharedObjectID(local, target),
		OwnerAccountId: local.AccountId,
		OwnerType:      "account",
		Accounts:       []*api.FriendDmAccount{local, target},
	}
	if err := validateFriendDmBootstrap(
		bootstrap,
		local.AccountId,
		target.AccountId,
		localPeer.String(),
	); err != nil {
		t.Fatalf("validate epoch-zero bootstrap: %v", err)
	}
}

func TestBuildFriendDmParticipantPlanReconcilesActivePeers(t *testing.T) {
	current := []*sobject.SOParticipantConfig{
		{PeerId: "peer-owner", EntityId: "acct-a", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: "peer-stale", EntityId: "acct-old", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: "peer-b", EntityId: "acct-b", Role: sobject.SOParticipantRole_SOParticipantRole_READER},
	}
	accounts := []*api.FriendDmAccount{
		{AccountId: "acct-a", Sessions: []*api.FriendDmSessionPeer{{PeerId: "peer-owner"}, {PeerId: "peer-a-new"}}},
		{AccountId: "acct-b", Sessions: []*api.FriendDmSessionPeer{{PeerId: "peer-b"}}},
	}
	want := friendDmParticipantPlan{
		removals: []string{"peer-b", "peer-stale"},
		additions: []friendDmParticipant{
			{peerID: "peer-a-new", accountID: "acct-a", role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
			{peerID: "peer-b", accountID: "acct-b", role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
		},
	}
	got, err := buildFriendDmParticipantPlan(current, accounts, "peer-owner")
	if err != nil {
		t.Fatalf("buildFriendDmParticipantPlan: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %#v, want %#v", got, want)
	}

	converged := append([]*sobject.SOParticipantConfig(nil), current[:1]...)
	converged = append(converged,
		&sobject.SOParticipantConfig{PeerId: "peer-a-new", EntityId: "acct-a", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
		&sobject.SOParticipantConfig{PeerId: "peer-b", EntityId: "acct-b", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
	)
	got, err = buildFriendDmParticipantPlan(converged, accounts, "peer-owner")
	if err != nil {
		t.Fatalf("build converged plan: %v", err)
	}
	if len(got.removals) != 0 || len(got.additions) != 0 {
		t.Fatalf("converged plan = %#v, want no mutations", got)
	}
}

func TestBuildFriendDmParticipantPlanCorrectsRoles(t *testing.T) {
	plan, err := buildFriendDmParticipantPlan(
		[]*sobject.SOParticipantConfig{
			{PeerId: "peer-owner", EntityId: "acct-a", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
			{PeerId: "peer-a-new", EntityId: "acct-a", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
			{PeerId: "peer-b", EntityId: "acct-b", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
		},
		[]*api.FriendDmAccount{
			{AccountId: "acct-a", Sessions: []*api.FriendDmSessionPeer{{PeerId: "peer-owner"}, {PeerId: "peer-a-new"}}},
			{AccountId: "acct-b", Sessions: []*api.FriendDmSessionPeer{{PeerId: "peer-b"}}},
		},
		"peer-owner",
	)
	if err != nil {
		t.Fatalf("build role correction plan: %v", err)
	}
	want := friendDmParticipantPlan{
		removals: []string{"peer-a-new", "peer-b"},
		additions: []friendDmParticipant{
			{peerID: "peer-a-new", accountID: "acct-a", role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
			{peerID: "peer-b", accountID: "acct-b", role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
		},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("plan = %#v, want %#v", plan, want)
	}
}

func TestBuildFriendDmParticipantPlanRejectsMissingLocalOwner(t *testing.T) {
	_, err := buildFriendDmParticipantPlan(
		[]*sobject.SOParticipantConfig{{
			PeerId:   "peer-owner",
			EntityId: "acct-a",
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
		}},
		[]*api.FriendDmAccount{{AccountId: "acct-a", Sessions: []*api.FriendDmSessionPeer{{PeerId: "peer-other"}}}},
		"peer-owner",
	)
	if err == nil {
		t.Fatal("expected missing local owner error")
	}
}
