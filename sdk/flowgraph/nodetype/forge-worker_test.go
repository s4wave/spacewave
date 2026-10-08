package flowgraph_nodetype

import (
	"maps"
	"strings"
	"testing"

	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// forgeWorkerNode places a Forge Worker node with the given parameters.
func forgeWorkerNode(parameters map[string]string) *s4wave_flowgraph.PlacedFlowgraphNode {
	return &s4wave_flowgraph.PlacedFlowgraphNode{
		FlowgraphKey: "flowgraph/main",
		NodeID:       "forge",
		Node:         &s4wave_flowgraph.FlowgraphNode{TypeId: s4wave_flowgraph.ForgeWorkerNodeTypeID, Parameters: parameters},
	}
}

// TestForgeWorkerCapability shows a Worker in a Cluster as a forge-worker
// capability that carries the declaration, and rejects a Worker that is missing
// or outside every Cluster.
func TestForgeWorkerCapability(t *testing.T) {
	// Create a Worker in a Cluster and a Worker outside every Cluster.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	p, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	sender := p.GetPeerID()
	if _, _, err := forge_cluster.CreateCluster(ctx, tb.WorldState, "cluster/main", "cluster", sender, sender); err != nil {
		t.Fatal(err)
	}
	for _, workerKey := range []string{"worker/in", "worker/out"} {
		if _, _, err := tb.WorldState.ApplyWorldOp(ctx, forge_worker.NewWorkerCreateOp(workerKey, "worker", nil), sender); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := forge_cluster.AssignWorkerToCluster(ctx, tb.WorldState, "cluster/main", "worker/in", sender); err != nil {
		t.Fatal(err)
	}

	// Show the Worker in the Cluster with its declared capacity.
	valid := map[string]string{
		"worker":       " worker/in ",
		"milli_cpu":    "2000",
		"memory_bytes": "4294967296",
		"backends":     "docker, fuse",
	}
	node := forgeWorkerNode(valid)
	got, err := forgeWorker{}.GetCapability(ctx, tb.WorldState, node)
	if err != nil {
		t.Fatal(err)
	}

	// Check the kind, the policy ref and the declaration the capability carries.
	if got.GetKind() != s4wave_device.DeviceCapabilityKindForgeWorker {
		t.Fatalf("kind = %q, want %q", got.GetKind(), s4wave_device.DeviceCapabilityKindForgeWorker)
	}
	if got.GetPolicy().GetLocalPolicyRef() != node.CapabilityID() {
		t.Fatalf("local policy ref = %q, want %q", got.GetPolicy().GetLocalPolicyRef(), node.CapabilityID())
	}
	declaration := got.GetWorkerDeclaration()
	if declaration.GetWorkerObjectKey() != "worker/in" || declaration.GetMilliCpu() != 2000 || declaration.GetMemoryBytes() != 4294967296 {
		t.Fatalf("declaration = %v", declaration)
	}
	if backends := declaration.GetBackends(); len(backends) != 2 || backends[0] != "docker" || backends[1] != "fuse" {
		t.Fatalf("backends = %v, want [docker fuse]", backends)
	}

	// Check the capability links to the Worker object.
	if got.GetLink().GetObjectKey() != "worker/in" || got.GetLink().GetTypeId() != forge_worker.WorkerTypeID {
		t.Fatalf("link = %v", got.GetLink())
	}

	// Reject a Worker outside every Cluster and a Worker that does not exist.
	for name, tc := range map[string]struct {
		worker string
		want   string
	}{
		"unassigned": {"worker/out", "not assigned to a Cluster"},
		"missing":    {"worker/missing", "verify forge worker"},
	} {
		parameters := maps.Clone(valid)
		parameters["worker"] = tc.worker
		if _, err := (forgeWorker{}).GetCapability(ctx, tb.WorldState, forgeWorkerNode(parameters)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: GetCapability error = %v, want %q", name, err, tc.want)
		}
	}
}

// TestForgeWorkerRejectsMalformedParameters rejects each declaration whose
// parameters are missing or malformed in Compile and in GetCapability.
func TestForgeWorkerRejectsMalformedParameters(t *testing.T) {
	valid := map[string]string{"worker": "worker/in", "milli_cpu": "2000", "memory_bytes": "1073741824", "backends": "docker"}
	for name, mutate := range map[string]func(map[string]string){
		"missing worker":     func(p map[string]string) { delete(p, "worker") },
		"blank worker":       func(p map[string]string) { p["worker"] = " " },
		"missing milli_cpu":  func(p map[string]string) { delete(p, "milli_cpu") },
		"zero milli_cpu":     func(p map[string]string) { p["milli_cpu"] = "0" },
		"bad memory_bytes":   func(p map[string]string) { p["memory_bytes"] = "4GiB" },
		"missing backends":   func(p map[string]string) { delete(p, "backends") },
		"blank backend":      func(p map[string]string) { p["backends"] = "docker,," },
		"spaced backend":     func(p map[string]string) { p["backends"] = "docker host" },
		"duplicate backends": func(p map[string]string) { p["backends"] = "docker,docker" },
	} {
		parameters := maps.Clone(valid)
		mutate(parameters)
		bad := forgeWorkerNode(parameters)
		if _, err := (forgeWorker{}).Compile(bad); err == nil {
			t.Errorf("%s: Compile accepted the declaration", name)
		}
		if _, err := (forgeWorker{}).GetCapability(t.Context(), nil, bad); err == nil {
			t.Errorf("%s: GetCapability accepted the declaration", name)
		}
	}
}
