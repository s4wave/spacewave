package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
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
		postedEpoch  *sobject.SOKeyEpoch
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
	if !soGrantSliceHasPeerID(postedEpoch.GetGrants(), localPID.String()) {
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

func decodePostedRootInner(
	t *testing.T,
	soID string,
	localPriv crypto.PrivKey,
	localPeerID string,
	epoch *sobject.SOKeyEpoch,
	root *sobject.SORoot,
) *sobject.SORootInner {
	t.Helper()

	grant := findSOGrantByPeerID(epoch.GetGrants(), localPeerID)
	if grant == nil {
		t.Fatal("expected local grant")
	}
	grantInner, err := grant.DecryptInnerData(localPriv, soID)
	if err != nil {
		t.Fatalf("decrypt grant inner: %v", err)
	}
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: logrus.New().WithField("test", t.Name())},
		buildStandaloneSpaceInitStepFactorySet(),
		grantInner.GetTransformConf(),
	)
	if err != nil {
		t.Fatalf("build transformer: %v", err)
	}
	innerData, err := xfrm.DecodeBlock(root.GetInner())
	if err != nil {
		t.Fatalf("decode root inner: %v", err)
	}
	inner := &sobject.SORootInner{}
	if err := inner.UnmarshalVT(innerData); err != nil {
		t.Fatalf("unmarshal root inner: %v", err)
	}
	return inner
}
