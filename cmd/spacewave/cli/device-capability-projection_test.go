package spacewave_cli

import (
	"context"
	"strings"
	"testing"

	device_policy "github.com/s4wave/spacewave/core/device/policy"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

// declaredPolicy builds a one-revision policy declaring the given worker key.
func declaredPolicy(workerObjectKey string) *device_policy.DevicePolicy {
	return &device_policy.DevicePolicy{
		Revision: 1,
		ForgeWorker: &device_policy.ForgeWorkerPolicy{
			WorkerObjectKey: workerObjectKey,
			MilliCpu:        1_000,
			MemoryBytes:     1 << 30,
			Backends:        []string{"docker"},
		},
	}
}

// existingCaps returns the non-policy capabilities the projection must
// preserve across every cycle. The neutral custom id proves pass-through
// without colliding with any policy-owned capability family.
func existingCaps() []*s4wave_device.DeviceCapability {
	return []*s4wave_device.DeviceCapability{{
		Id:   "custom-capability",
		Kind: "custom",
	}}
}

func capsByID(caps []*s4wave_device.DeviceCapability) map[string]*s4wave_device.DeviceCapability {
	out := make(map[string]*s4wave_device.DeviceCapability, len(caps))
	for _, cap := range caps {
		out[cap.GetId()] = cap
	}
	return out
}

// TestComputeDevicePolicyCapabilitiesAuthorsAndRemovesForgeWorker pins the
// authoring and removal rules for the declared envelope.
func TestComputeDevicePolicyCapabilitiesAuthorsAndRemovesForgeWorker(t *testing.T) {
	existing := append(existingCaps(), &s4wave_device.DeviceCapability{
		Id:   devicePolicyForgeWorkerCapabilityID,
		Kind: s4wave_device.DeviceCapabilityKindForgeWorker,
		Link: &s4wave_device.DeviceCapabilityLink{ObjectKey: "worker/old", TypeId: forge_worker.WorkerTypeID},
	})

	// Declared: authored with both link fields set to the declared key.
	next := computeDevicePolicyCapabilities(declaredPolicy("worker/new"), existing)
	byID := capsByID(next)
	fw, ok := byID[devicePolicyForgeWorkerCapabilityID]
	if !ok {
		t.Fatal("declared envelope must author the forge-worker capability")
	}
	if fw.GetLink().GetObjectKey() != "worker/new" || fw.GetLink().GetTypeId() != forge_worker.WorkerTypeID {
		t.Fatalf("link must carry object key and type id: %+v", fw)
	}
	if fw.GetKind() != s4wave_device.DeviceCapabilityKindForgeWorker {
		t.Fatalf("unexpected kind: %+v", fw)
	}
	if _, ok := byID["custom-capability"]; !ok {
		t.Fatal("non-policy capability must survive the cycle")
	}

	// Removed: an absent section drops the capability and preserves the rest.
	next = computeDevicePolicyCapabilities(&device_policy.DevicePolicy{Revision: 2}, existing)
	byID = capsByID(next)
	if _, ok := byID[devicePolicyForgeWorkerCapabilityID]; ok {
		t.Fatal("absent section must remove the forge-worker capability")
	}
	if _, ok := byID["custom-capability"]; !ok {
		t.Fatal("non-policy capability must survive removal")
	}
}

// newCapacityTestbed builds a local world engine for observer tests.
func newCapacityTestbed(t *testing.T) (context.Context, *world_testbed.Testbed) {
	ctx := context.Background()
	btb, err := hydra_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { btb.Release() })
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wtb.Release() })
	return ctx, wtb
}

// TestVerifyForgeWorkerLinkMissingObject pins the in-transaction fail-before:
// a declared key without a Worker object fails verification, which aborts the
// projection transaction and leaves the Device block unchanged.
func TestVerifyForgeWorkerLinkMissingObject(t *testing.T) {
	ctx, wtb := newCapacityTestbed(t)
	err := world.ExecTransaction(ctx, wtb.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		return verifyForgeWorkerLink(ctx, ws, "worker/missing")
	})
	if err == nil || !strings.Contains(err.Error(), "verify forge worker") {
		t.Fatalf("expected verification failure for missing worker, got %v", err)
	}
}

// TestVerifyForgeWorkerLinkRequiresClusterAssignment rejects a typed Worker
// that exists but cannot receive work from any Cluster.
func TestVerifyForgeWorkerLinkRequiresClusterAssignment(t *testing.T) {
	ctx, wtb := newCapacityTestbed(t)
	err := world.ExecTransaction(ctx, wtb.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		if _, _, err := ws.ApplyWorldOp(
			ctx,
			forge_worker.NewWorkerCreateOp("worker/unassigned", "unassigned", nil),
			peer.ID("test-peer"),
		); err != nil {
			return err
		}
		return verifyForgeWorkerLink(ctx, ws, "worker/unassigned")
	})
	if err == nil || !strings.Contains(err.Error(), "not assigned to a Cluster") {
		t.Fatalf("expected cluster-assignment failure, got %v", err)
	}
}
