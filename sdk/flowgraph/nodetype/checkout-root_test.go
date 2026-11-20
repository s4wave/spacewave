package flowgraph_nodetype

import (
	"maps"
	"testing"

	"github.com/s4wave/spacewave/core/checkoutroot"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// TestCheckoutRootCapability shows a valid root as a filesystem capability and
// rejects a root whose name, path or access is missing or malformed.
func TestCheckoutRootCapability(t *testing.T) {
	// Show a read-write root as a filesystem capability that selectors match.
	place := func(parameters map[string]string) *s4wave_flowgraph.PlacedFlowgraphNode {
		return &s4wave_flowgraph.PlacedFlowgraphNode{
			FlowgraphKey: "flowgraph/main",
			NodeID:       "root",
			Node:         &s4wave_flowgraph.FlowgraphNode{TypeId: s4wave_flowgraph.CheckoutRootNodeTypeID, Parameters: parameters},
		}
	}

	// Read the capability of a valid root.
	valid := map[string]string{"name": " skiffos ", "path": "/srv/skiffos/", "access": "read-write"}
	node := place(valid)
	got, err := checkoutRoot{}.GetCapability(t.Context(), nil, node)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetKind() != s4wave_device.DeviceCapabilityKindFilesystem {
		t.Fatalf("kind = %q, want %q", got.GetKind(), s4wave_device.DeviceCapabilityKindFilesystem)
	}

	// Require the root's name, display path, selection ref and write access.
	root := got.GetCheckoutRoot()
	if root.GetName() != "skiffos" || root.GetDisplayPath() != "/srv/skiffos" || root.GetSelectionRef() != node.CapabilityID() {
		t.Fatalf("checkout root = %v", root)
	}
	if !s4wave_device.DeviceCheckoutRootCanWrite(root) {
		t.Fatalf("read-write root cannot write: %v", root)
	}

	// Require the advertised protocol to be the one the controller serves.
	configs, err := (checkoutRoot{}).Compile(t.Context(), node)
	if err != nil {
		t.Fatal(err)
	}
	conf := configs["checkout-root"].(*checkoutroot.Config)
	if got.GetLink().GetProtocolId() != string(conf.ProtocolID()) {
		t.Fatalf("advertised protocol = %q, serving %q", got.GetLink().GetProtocolId(), conf.ProtocolID())
	}

	// Reject each malformed declaration in Compile and in GetCapability.
	for name, mutate := range map[string]func(map[string]string){
		"missing name":   func(p map[string]string) { delete(p, "name") },
		"blank name":     func(p map[string]string) { p["name"] = "  " },
		"relative path":  func(p map[string]string) { p["path"] = "srv/skiffos" },
		"missing path":   func(p map[string]string) { delete(p, "path") },
		"missing access": func(p map[string]string) { delete(p, "access") },
		"unknown access": func(p map[string]string) { p["access"] = "write" },
	} {
		parameters := map[string]string{}
		maps.Copy(parameters, valid)
		mutate(parameters)
		bad := place(parameters)
		if _, err := (checkoutRoot{}).Compile(t.Context(), bad); err == nil {
			t.Errorf("%s: Compile accepted the root", name)
		}
		if _, err := (checkoutRoot{}).GetCapability(t.Context(), nil, bad); err == nil {
			t.Errorf("%s: GetCapability accepted the root", name)
		}
	}
}
