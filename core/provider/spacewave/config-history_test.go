package provider_spacewave

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

// TestCloudHostRejectsConfigChangesThroughRootWrite preserves the server's
// configuration publication boundary when shared host writes carry lineage.
func TestCloudHostRejectsConfigChangesThroughRootWrite(t *testing.T) {
	// Count real HTTP requests so an accidental root publication cannot pass unnoticed.
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	priv, pid := generateTestKeypair(t)
	client := NewSessionClient(srv.Client(), srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	host := newCloudSOHost(nil, client, testSharedObjectID, "", nil, priv, pid, nil, nil, nil, nil)
	initial := &sobject.SOState{Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{{
		PeerId: pid.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER,
	}}}}
	host.stateCtr.SetValue(initial)

	// A valid signed transition still requires the cloud config-state operation.
	entry, err := sobject.BuildSOConfigChange(testSharedObjectID, initial.GetConfig(), initial.GetConfig(), sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.soHost.ApplyConfigChange(t.Context(), entry, nil); err == nil {
		t.Fatal("cloud root write accepted a configuration transition")
	}
	if requests.Load() != 0 {
		t.Fatalf("unexpected HTTP requests = %d", requests.Load())
	}
	if !host.stateCtr.GetValue().EqualVT(initial) {
		t.Fatal("rejected configuration transition changed the cloud cache")
	}
}
