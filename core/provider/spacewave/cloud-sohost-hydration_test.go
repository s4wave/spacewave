package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/sirupsen/logrus"
)

// TestCloudHostHydrationRefreshesState catches up a durable snapshot after startup.
func TestCloudHostHydrationRefreshesState(t *testing.T) {
	// Serve the cloud snapshot and signal the background state request.
	initial, key := newTestGenesisState(t)
	pid := mustPeerID(t, key)
	data := mustMarshalSOStateMessageSnapshotJSON(t, initial)
	pulled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expose only the state endpoint this hydrated host needs.
		if r.URL.Path != "/api/sobject/"+testSharedObjectID+"/state" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		pulled <- struct{}{}
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)

	// Restore a verified checkpoint before starting the host routines.
	le := logrus.New().WithField("test", t.Name())
	host := newCloudSOHost(le,
		NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, pid.String()),
		testSharedObjectID, "", newWSTracker(le, func() *SessionClient { return nil }), key, pid, nil,
		&api.VerifiedSOStateCache{
			PeerState: initial.CloneVT(), CloudState: initial.CloneVT(), CurrentConfig: initial.GetConfig().CloneVT(),
			VerifiedConfigChainHash: initial.GetConfig().GetConfigChainHash(), VerifiedConfigChainSeqno: initial.GetConfig().GetConfigChainSeqno(),
		}, nil, nil,
	)
	if host.stateCtr.GetValue() == nil {
		t.Fatal("verified checkpoint was not hydrated")
	}

	// Execute must refresh the durable snapshot even though initial state is ready.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- host.Execute(ctx) }()
	select {
	case <-pulled:
	case <-ctx.Done():
		t.Fatal("hydrated host did not refresh its cloud state")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
