package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

func TestSessionClientEnrollSpaceMember(t *testing.T) {
	const (
		soID      = "so-standalone-enroll"
		accountID = "test-account"
	)

	entityPriv, _ := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	_, targetPID := generateTestKeypair(t)

	state, chainResp, _, keypairResp := buildRejoinTestFixtures(
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
	keypairJSON := mustMarshalVT(t, keypairResp)

	var posted *api.PostConfigStateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/enroll-member":
			req := &api.EnrollMemberRequest{}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read enroll-member body: %v", err)
			}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal enroll-member: %v", err)
			}
			if req.GetAccountId() != accountID {
				t.Fatalf("account id = %q", req.GetAccountId())
			}
			if !req.GetIgnoreExclusion() {
				t.Fatal("expected ignore_exclusion=true")
			}
			resp := &api.EnrollMemberResponse{
				Peers: []*api.EnrollMemberPeer{{PeerId: targetPID.String()}},
			}
			data, err := resp.MarshalVT()
			if err != nil {
				t.Fatalf("marshal enroll-member response: %v", err)
			}
			_, _ = w.Write(data)
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairJSON)
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
		ownerPriv,
		ownerPID.String(),
	)
	resp, err := cli.EnrollSpaceMember(
		context.Background(),
		logrus.New().WithField("test", t.Name()),
		accountID,
		soID,
		accountID,
		sobject.SOParticipantRole_SOParticipantRole_WRITER,
	)
	if err != nil {
		t.Fatalf("EnrollSpaceMember: %v", err)
	}
	if len(resp.GetResults()) != 1 {
		t.Fatalf("results = %d", len(resp.GetResults()))
	}
	result := resp.GetResults()[0]
	if result.GetPeerId() != targetPID.String() {
		t.Fatalf("peer id = %q", result.GetPeerId())
	}
	if !result.GetEnrolled() {
		t.Fatalf("expected enrolled result, got %+v", result)
	}
	if posted == nil {
		t.Fatal("expected config-state write")
	}
	change := &sobject.SOConfigChange{}
	if err := change.UnmarshalVT(posted.GetConfigChange()); err != nil {
		t.Fatalf("unmarshal posted config change: %v", err)
	}
	if change.GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT {
		t.Fatalf("change type = %v", change.GetChangeType())
	}
	if got := participantConfigForPeer(change.GetConfig(), targetPID.String()); got == nil {
		t.Fatal("expected target peer in posted config")
	}
	if posted.GetKeyEpoch() == nil {
		t.Fatal("expected posted key epoch")
	}
	if !soGrantSliceHasPeerID(posted.GetKeyEpoch().GetGrants(), targetPID.String()) {
		t.Fatal("expected target grant in posted key epoch")
	}
}
