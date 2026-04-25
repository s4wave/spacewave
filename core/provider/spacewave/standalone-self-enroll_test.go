package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

func TestSessionClientSelfEnrollSpacePeer(t *testing.T) {
	const (
		soID      = "so-standalone-self-enroll"
		accountID = "test-account"
	)

	entityPriv, _ := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newSessionPriv, newSessionPID := generateTestKeypair(t)

	state, chainResp, envResp, keypairResp := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		3,
	)

	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envData := mustMarshalVT(t, envResp)
	keypairData := mustMarshalVT(t, keypairResp)

	var posted *api.PostConfigStateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		case "/api/sobject/" + soID + "/recovery-envelope":
			_, _ = w.Write(envData)
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
			posted = req
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
		newSessionPriv,
		newSessionPID.String(),
	)
	changed, err := cli.SelfEnrollSpacePeer(
		context.Background(),
		entityPriv,
		accountID,
		soID,
	)
	if err != nil {
		t.Fatalf("SelfEnrollSpacePeer: %v", err)
	}
	if !changed {
		t.Fatal("expected self-enroll mutation")
	}
	if posted == nil {
		t.Fatal("expected config-state write")
	}

	change := &sobject.SOConfigChange{}
	if err := change.UnmarshalVT(posted.GetConfigChange()); err != nil {
		t.Fatalf("unmarshal posted config change: %v", err)
	}
	if change.GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER {
		t.Fatalf("change type = %v", change.GetChangeType())
	}
	got := participantConfigForPeer(change.GetConfig(), newSessionPID.String())
	if got == nil {
		t.Fatal("expected self-enrolled peer in posted config")
	}
	if got.GetEntityId() != accountID {
		t.Fatalf("entity id = %q", got.GetEntityId())
	}
	if got.GetRole() != sobject.SOParticipantRole_SOParticipantRole_OWNER {
		t.Fatalf("role = %v", got.GetRole())
	}
	if posted.GetKeyEpoch() == nil {
		t.Fatal("expected posted key epoch")
	}
	if !soGrantSliceHasPeerID(posted.GetKeyEpoch().GetGrants(), newSessionPID.String()) {
		t.Fatal("expected self-enrolled peer grant in posted key epoch")
	}
}
