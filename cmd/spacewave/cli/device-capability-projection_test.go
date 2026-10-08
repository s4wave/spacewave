package spacewave_cli

import (
	"slices"
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestComputeDevicePolicyCapabilitiesDropsPolicyOwnedAndKeepsNodesLast pins the
// order the policy projection and the Flowgraph reconciler share: node
// capabilities stay after every other capability, so neither writer rewrites
// the other's order. It also pins the ownership rule: a capability the policy
// wrote earlier, which carries the policy ref prefix, is dropped.
func TestComputeDevicePolicyCapabilitiesDropsPolicyOwnedAndKeepsNodesLast(t *testing.T) {
	// Seed a node capability ahead of a custom one and a policy-owned one.
	node := &s4wave_device.DeviceCapability{
		Id:   s4wave_device.DeviceCapabilityKindFlowgraphNode + "/flowgraph/main/tcp",
		Kind: s4wave_device.DeviceCapabilityKindFlowgraphNode,
	}
	existing := []*s4wave_device.DeviceCapability{
		node,
		{Id: "custom-capability", Kind: "custom"},
		{
			Id:   "forge-worker",
			Kind: s4wave_device.DeviceCapabilityKindForgeWorker,
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "device-policy/12/forge-worker",
			},
		},
	}

	// Require the custom capability first, the stale one gone, and the node last.
	next := computeDevicePolicyCapabilities(existing)
	var ids []string
	for _, cap := range next {
		ids = append(ids, cap.GetId())
	}
	want := []string{"custom-capability", node.GetId()}
	if !slices.Equal(ids, want) {
		t.Fatalf("capability order %v, want %v", ids, want)
	}
}
