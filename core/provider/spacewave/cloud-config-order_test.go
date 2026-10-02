package provider_spacewave

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// TestCloudConfigParticipantOrder accepts reordered membership without accepting
// changes to signed authority, and retains that state across cache hydration.
func TestCloudConfigParticipantOrder(t *testing.T) {
	// Build a real signed root.
	owner, ownerID := generateTestKeypair(t)
	entity, _ := generateTestKeypair(t)
	_, readerID := generateTestKeypair(t)
	state, chain, _, _ := buildRejoinTestFixtures(t, testSharedObjectID, "owner-account", owner, ownerID, entity, 1)

	// Add a reader through a signed change.
	next := state.GetConfig().CloneVT()
	next.Participants = append(next.Participants, &sobject.SOParticipantConfig{
		PeerId: readerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_READER, EntityId: "reader-account",
	})
	change, err := sobject.BuildSOConfigChange(testSharedObjectID, state.GetConfig(), next, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = sobject.VerifyConfigChange(testSharedObjectID, state.GetConfig(), change)
	if err != nil {
		t.Fatal(err)
	}
	chain.ConfigChanges = append(chain.ConfigChanges, change)
	encodedChain := mustMarshalVT(t, chain)

	// Only the HTTP response is substituted; verification and publication are real.
	for _, tc := range []struct {
		// name identifies the cloud response.
		name string
		// mutate changes a reordered response beyond its participant order.
		mutate func(*sobject.SOState)
	}{
		{name: "reordered"},
		{name: "role", mutate: func(s *sobject.SOState) {
			s.Config.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_OWNER
		}},
		{name: "entity", mutate: func(s *sobject.SOState) { s.Config.Participants[0].EntityId = "different-account" }},
		{name: "missing", mutate: func(s *sobject.SOState) { s.Config.Participants = s.Config.Participants[1:] }},
		{name: "duplicate", mutate: func(s *sobject.SOState) { s.Config.Participants[0] = s.Config.Participants[1].CloneVT() }},
		{name: "consensus", mutate: func(s *sobject.SOState) { s.Config.ConsensusMode++ }},
		{name: "sequence", mutate: func(s *sobject.SOState) { s.Config.ConfigChainSeqno++ }},
		{name: "root signature", mutate: func(s *sobject.SOState) { s.Root.ValidatorSignatures = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Preserve the signed chain while the response projects participants differently.
			candidate := state.CloneVT()
			slices.Reverse(candidate.Config.Participants)
			if tc.mutate != nil {
				tc.mutate(candidate)
			}
			before := candidate.CloneVT()
			response := mustMarshalVT(t, &api.SOStateMessage{
				Seqno: 1, ConfigChain: chain,
				Content: &api.SOStateMessage_Snapshot{Snapshot: candidate},
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/sobject/"+testSharedObjectID+"/state" {
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if _, err := w.Write(response); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			host := &cloudSOHost{
				le:     logrus.New().WithField("test", t.Name()),
				client: NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, owner, ownerID.String()),
				soID:   testSharedObjectID, privKey: owner, peerID: ownerID,
				stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil),
			}

			// A failed check must leave the watched root unavailable.
			if err := host.pullState(t.Context(), SeedReasonColdSeed); err != nil {
				t.Fatal(err)
			}
			accepted := host.stateCtr.GetValue()
			if tc.mutate != nil {
				if accepted != nil || host.initialStateErr == nil {
					t.Fatalf("invalid state was not rejected: state=%v error=%v", accepted != nil, host.initialStateErr)
				}
				return
			}
			if accepted == nil {
				t.Fatalf("reordered state was rejected: %v", host.initialStateErr)
			}
			if !accepted.EqualVT(before) {
				t.Fatal("verification changed the received state")
			}
			if !bytes.Equal(encodedChain, mustMarshalVT(t, chain)) {
				t.Fatal("verification changed the signed chain")
			}

			// The persisted snapshot can differ in order from its verified history.
			cache := host.buildVerifiedStateCache()
			cache.PeerState = accepted.CloneVT()
			reopened := &cloudSOHost{soID: testSharedObjectID, stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil)}
			reopened.hydrateVerifiedStateCache(cache)
			if !accepted.EqualVT(reopened.stateCtr.GetValue()) {
				t.Fatal("reordered verified state was lost on cache hydration")
			}
		})
	}
}
