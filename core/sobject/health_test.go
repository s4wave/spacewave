package sobject

import (
	"slices"
	"testing"
)

// TestSyncPeerAdmissionPreservesMountHealth keeps source availability separate
// from locally held mount health and clears only the recovering participant.
func TestSyncPeerAdmissionPreservesMountHealth(t *testing.T) {
	health := NewSharedObjectReadyHealth(SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
	first := health.WithSyncPeerAdmission("peer-b", false)
	second := first.WithSyncPeerAdmission("peer-a", false)
	if !slices.Equal(second.GetSyncDeniedPeerIds(), []string{"peer-a", "peer-b"}) {
		t.Fatalf("denied sources = %v", second.GetSyncDeniedPeerIds())
	}
	if len(health.GetSyncDeniedPeerIds()) != 0 || !slices.Equal(first.GetSyncDeniedPeerIds(), []string{"peer-b"}) {
		t.Fatal("updating source health mutated an earlier snapshot")
	}
	if second.GetStatus() != health.GetStatus() || second.GetCommonReason() != health.GetCommonReason() {
		t.Fatal("remote admission changed local mount authority")
	}
	if unchanged := second.WithSyncPeerAdmission("peer-a", false); unchanged != second {
		t.Fatal("unchanged denial produced another watch update")
	}

	// Another source becoming available cannot clear the denied peer.
	if unchanged := second.WithSyncPeerAdmission("peer-c", true); unchanged != second {
		t.Fatal("unrelated admission changed the denied sources")
	}
	recovered := second.WithSyncPeerAdmission("peer-a", true)
	if !slices.Equal(recovered.GetSyncDeniedPeerIds(), []string{"peer-b"}) {
		t.Fatalf("remaining denied sources = %v", recovered.GetSyncDeniedPeerIds())
	}
	if !slices.Equal(second.GetSyncDeniedPeerIds(), []string{"peer-a", "peer-b"}) {
		t.Fatal("recovery mutated the previous health snapshot")
	}
}

// TestSyncRecoveryRequiresConvergence preserves local access and independent source denials.
func TestSyncRecoveryRequiresConvergence(t *testing.T) {
	initial := NewSharedObjectReadyHealth(SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
	recovery := initial.WithSyncPeerRecovery("peer-a", true)
	admitted := recovery.WithSyncPeerAdmission("peer-a", true)
	if len(admitted.GetSyncRecoveryPeerIds()) != 1 || admitted.GetStatus() != SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatal("admission cleared recovery or closed local access")
	}
	denied := admitted.WithSyncPeerAdmission("peer-b", false)
	converged := denied.WithSyncPeerRecovery("peer-a", false)
	if len(converged.GetSyncRecoveryPeerIds()) != 0 || len(converged.GetSyncDeniedPeerIds()) != 1 {
		t.Fatal("convergence did not clear only its own recovery requirement")
	}
	if len(recovery.GetSyncRecoveryPeerIds()) != 1 || len(initial.GetSyncRecoveryPeerIds()) != 0 {
		t.Fatal("recovery update mutated an earlier health snapshot")
	}
}

// TestWithoutSyncDenialsKeepsRecovery forgets admission denials for a replaced
// grant while keeping recovery requirements and earlier snapshots.
func TestWithoutSyncDenialsKeepsRecovery(t *testing.T) {
	denied := NewSharedObjectReadyHealth(SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT).
		WithSyncPeerAdmission("owner", false).
		WithSyncPeerRecovery("peer-a", true)
	cleared := denied.WithoutSyncDenials()
	if len(cleared.GetSyncDeniedPeerIds()) != 0 || len(cleared.GetSyncRecoveryPeerIds()) != 1 {
		t.Fatalf("denied = %v, recovery = %v", cleared.GetSyncDeniedPeerIds(), cleared.GetSyncRecoveryPeerIds())
	}
	if len(denied.GetSyncDeniedPeerIds()) != 1 {
		t.Fatal("clearing denials mutated the earlier snapshot")
	}
	if unchanged := cleared.WithoutSyncDenials(); unchanged != cleared {
		t.Fatal("clearing no denials produced another watch update")
	}
}
