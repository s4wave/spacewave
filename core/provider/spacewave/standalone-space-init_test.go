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
)

func TestSessionClientInitEmptyStandaloneSpace(t *testing.T) {
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

	var (
		postedConfig *api.PostConfigStateRequest
		postedRoot   *sobject.SORoot
		postedEpoch  *api.PostKeyEpochRequest
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
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/root":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read root body: %v", err)
			}
			req := &api.PostRootRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal post root request: %v", err)
			}
			postedRoot = req.GetRoot()
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/key-epoch":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read key-epoch body: %v", err)
			}
			req := &api.PostKeyEpochRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal key-epoch request: %v", err)
			}
			postedEpoch = req
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cli := NewSessionClient(
		http.DefaultClient,
		srv.URL,
		DefaultSigningEnvPrefix,
		localPriv,
		localPID.String(),
	)
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-init-root")
	}
	changed, err := cli.InitEmptyStandaloneSpace(
		context.Background(),
		nil,
		accountID,
		soID,
	)
	if err != nil {
		t.Fatalf("InitEmptyStandaloneSpace: %v", err)
	}
	if !changed {
		t.Fatal("expected init mutation")
	}
	if postedConfig == nil {
		t.Fatal("expected config-state write")
	}
	if postedRoot == nil {
		t.Fatal("expected root write")
	}
	if postedEpoch == nil {
		t.Fatal("expected key-epoch write")
	}

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
	if postedRoot.GetInnerSeqno() != 1 {
		t.Fatalf("root seqno = %d", postedRoot.GetInnerSeqno())
	}
	if postedEpoch.GetKeyEpoch() == nil {
		t.Fatal("expected posted key epoch")
	}
	if !soGrantSliceHasPeerID(postedEpoch.GetKeyEpoch().GetGrants(), localPID.String()) {
		t.Fatal("expected local owner grant in posted key epoch")
	}
}

func TestSessionClientInitEmptyStandaloneSpaceRepairsGrantlessGenesis(t *testing.T) {
	const (
		soID      = "so-standalone-repair"
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
		Root: &sobject.SORoot{
			InnerSeqno: 1,
			Inner:      []byte("existing-root"),
		},
	}
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainData := mustMarshalVT(t, &sobject.SOConfigChainResponse{
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:      0,
			SeqnoStart: 0,
		}},
	})
	keypairResp := buildRecoveryKeypairResponse(t, accountID, otherPriv)
	keypairData := mustMarshalVT(t, keypairResp)

	var (
		postedRoot  *sobject.SORoot
		postedEpoch *api.PostKeyEpochRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainData)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairData)
		case "/api/sobject/" + soID + "/key-epoch":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read key-epoch body: %v", err)
			}
			req := &api.PostKeyEpochRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal key-epoch request: %v", err)
			}
			postedEpoch = req
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/root":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read root body: %v", err)
			}
			req := &api.PostRootRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal post root request: %v", err)
			}
			postedRoot = req.GetRoot()
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cli := NewSessionClient(
		http.DefaultClient,
		srv.URL,
		DefaultSigningEnvPrefix,
		localPriv,
		localPID.String(),
	)
	cli.executeWriteTicketAudience = func(
		ctx context.Context,
		resourceID string,
		audience writeTicketAudience,
		fn func(ticket string) error,
	) error {
		return fn("ticket-init-root")
	}
	changed, err := cli.InitEmptyStandaloneSpace(
		context.Background(),
		nil,
		accountID,
		soID,
	)
	if err != nil {
		t.Fatalf("InitEmptyStandaloneSpace: %v", err)
	}
	if !changed {
		t.Fatal("expected repair mutation")
	}
	if postedEpoch == nil {
		t.Fatal("expected key-epoch repair write")
	}
	if postedRoot == nil {
		t.Fatal("expected root repair write")
	}
	if postedEpoch.GetKeyEpoch().GetEpoch() != 1 {
		t.Fatalf("epoch = %d", postedEpoch.GetKeyEpoch().GetEpoch())
	}
	if postedEpoch.GetKeyEpoch().GetSeqnoStart() != 2 {
		t.Fatalf("epoch seqno_start = %d", postedEpoch.GetKeyEpoch().GetSeqnoStart())
	}
	if !soGrantSliceHasPeerID(postedEpoch.GetKeyEpoch().GetGrants(), localPID.String()) {
		t.Fatal("expected local owner grant in repaired key epoch")
	}
	if postedRoot.GetInnerSeqno() != 2 {
		t.Fatalf("root seqno = %d", postedRoot.GetInnerSeqno())
	}
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
