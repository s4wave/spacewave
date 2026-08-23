package provider_spacewave

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
)

func TestMarshalFriendDmChannelWorldOp(t *testing.T) {
	_, sender := generateTestKeypair(t)
	members := []string{"peer-a", "peer-b"}
	data, err := marshalFriendDmChannelWorldOp(sender, members)
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
	if got := chatOp.GetMemberPeerIds(); !reflect.DeepEqual(got, members) {
		t.Fatalf("member peer ids = %v, want %v", got, members)
	}
}

func TestOpenFriendDMHiddenTargetDoesNotMutate(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/account/friends/acct-b/dm" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"found":false}`))
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
	local := FriendDmAccount{
		AccountID:  "test-account",
		EntityUUID: "entity-a",
		Epoch:      2,
	}
	target := FriendDmAccount{
		AccountID:  "acct-b",
		EntityUUID: "entity-b",
		Epoch:      2,
		Sessions:   []FriendDmSession{{PeerID: targetPeer.String()}},
	}
	sharedObjectID := deriveFriendDmSharedObjectID(local, target)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"sharedObjectId":"` + sharedObjectID + `","ready":false,"ownerAccountId":"acct-b","ownerType":"account","accounts":[{"accountId":"test-account","entityUuid":"entity-a","epoch":2,"sessions":[{"peerId":"` + localPeer + `"}],"recoveryKeypairs":[{"peerId":"` + localPeer + `"}]},{"accountId":"acct-b","entityUuid":"entity-b","epoch":2,"sessions":[{"peerId":"` + targetPeer.String() + `"}],"recoveryKeypairs":[{"peerId":"` + targetPeer.String() + `"}]}]}`))
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
	var response string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	localPeerID := acc.GetCurrentSessionPeerID().String()
	local := FriendDmAccount{
		AccountID:  acc.GetAccountID(),
		EntityUUID: "entity-a",
		Epoch:      2,
		Sessions:   []FriendDmSession{{PeerID: localPeerID}},
	}
	target := FriendDmAccount{
		AccountID:  "acct-b",
		EntityUUID: "entity-b",
		Epoch:      2,
		Sessions:   []FriendDmSession{{PeerID: "invalid-peer"}},
	}
	response = `{"sharedObjectId":"` + deriveFriendDmSharedObjectID(local, target) +
		`","ready":false,"ownerAccountId":"` + local.AccountID +
		`","ownerType":"account","accounts":[{"accountId":"` + local.AccountID +
		`","entityUuid":"entity-a","epoch":2,"sessions":[{"peerId":"` + localPeerID +
		`"}],"recoveryKeypairs":[{"peerId":"` + localPeerID +
		`"}]},{"accountId":"acct-b","entityUuid":"entity-b","epoch":2,"sessions":[{"peerId":"invalid-peer"}],"recoveryKeypairs":[{"peerId":"` + localPeerID + `"}]}]}`
	if _, err := acc.OpenFriendDM(context.Background(), target.AccountID); err == nil {
		t.Fatal("expected invalid peer error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one Cloud request and no initialization", requests)
	}
}

func TestOpenFriendDMRejectsNoncanonicalIDBeforeInitialization(t *testing.T) {
	var requests int
	var response string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(response))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	_, targetPeer := generateTestKeypair(t)
	local := FriendDmAccount{
		AccountID:  acc.GetAccountID(),
		EntityUUID: "entity-a",
		Epoch:      2,
		Sessions:   []FriendDmSession{{PeerID: acc.GetCurrentSessionPeerID().String()}},
	}
	target := FriendDmAccount{
		AccountID:  "acct-b",
		EntityUUID: "entity-b",
		Epoch:      2,
		Sessions:   []FriendDmSession{{PeerID: targetPeer.String()}},
	}
	response = `{"sharedObjectId":"wrong-id","ready":false,"ownerAccountId":"` +
		local.AccountID + `","ownerType":"account","accounts":[{"accountId":"` +
		local.AccountID + `","entityUuid":"entity-a","epoch":2,"sessions":[{"peerId":"` +
		local.Sessions[0].PeerID + `"}],"recoveryKeypairs":[{"peerId":"` +
		local.Sessions[0].PeerID + `"}]},{"accountId":"acct-b","entityUuid":"entity-b","epoch":2,"sessions":[{"peerId":"` +
		target.Sessions[0].PeerID + `"}],"recoveryKeypairs":[{"peerId":"` +
		target.Sessions[0].PeerID + `"}]}]}`
	if _, err := acc.OpenFriendDM(context.Background(), target.AccountID); err == nil {
		t.Fatal("expected noncanonical ID error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one Cloud request and no initialization", requests)
	}
}

func TestValidateFriendDmBootstrapAllowsEpochZero(t *testing.T) {
	_, localPeer := generateTestKeypair(t)
	_, targetPeer := generateTestKeypair(t)
	local := FriendDmAccount{
		AccountID:  "acct-a",
		EntityUUID: "entity-a",
		Epoch:      0,
		Sessions:   []FriendDmSession{{PeerID: localPeer.String()}},
		RecoveryKeypairs: []FriendDmRecoveryKeypair{{
			PeerID: localPeer.String(),
		}},
	}
	target := FriendDmAccount{
		AccountID:  "acct-b",
		EntityUUID: "entity-b",
		Epoch:      0,
		Sessions:   []FriendDmSession{{PeerID: targetPeer.String()}},
		RecoveryKeypairs: []FriendDmRecoveryKeypair{{
			PeerID: targetPeer.String(),
		}},
	}
	bootstrap := &FriendDmBootstrap{
		SharedObjectID: deriveFriendDmSharedObjectID(local, target),
		Ready:          false,
		OwnerAccountID: local.AccountID,
		OwnerType:      "account",
		Accounts:       []FriendDmAccount{local, target},
	}
	if err := validateFriendDmBootstrap(
		bootstrap,
		local.AccountID,
		target.AccountID,
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
	accounts := []FriendDmAccount{
		{AccountID: "acct-a", Sessions: []FriendDmSession{{PeerID: "peer-owner"}, {PeerID: "peer-a-new"}}},
		{AccountID: "acct-b", Sessions: []FriendDmSession{{PeerID: "peer-b"}}},
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
		[]FriendDmAccount{
			{AccountID: "acct-a", Sessions: []FriendDmSession{{PeerID: "peer-owner"}, {PeerID: "peer-a-new"}}},
			{AccountID: "acct-b", Sessions: []FriendDmSession{{PeerID: "peer-b"}}},
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
		[]FriendDmAccount{{AccountID: "acct-a", Sessions: []FriendDmSession{{PeerID: "peer-other"}}}},
		"peer-owner",
	)
	if err == nil {
		t.Fatal("expected missing local owner error")
	}
}
