package flowgraph_nodetype

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

const (
	// forgeWorkerWorkerParameter names the parameter holding the object key of
	// the Forge Worker the Device hosts.
	forgeWorkerWorkerParameter = "worker"
	// forgeWorkerMilliCPUParameter names the parameter holding the declared CPU
	// in milli-cores.
	forgeWorkerMilliCPUParameter = "milli_cpu"
	// forgeWorkerMemoryBytesParameter names the parameter holding the declared
	// memory in bytes.
	forgeWorkerMemoryBytesParameter = "memory_bytes"
	// forgeWorkerBackendsParameter names the parameter holding the comma
	// separated runtime backends the Worker supports.
	forgeWorkerBackendsParameter = "backends"
)

// forgeWorker declares the Forge Worker its Device hosts and the capacity the
// Worker offers. It compiles to no entries: the Worker runs in the plugin
// process, which reads the declaration from the Device's accepted node. A
// Device keeps one Forge Worker.
type forgeWorker struct{}

// GetDisplayName returns the name shown for the node type.
func (forgeWorker) GetDisplayName() string {
	return "Forge Worker"
}

// GetPorts returns no ports, since the Worker takes work from its Cluster.
func (forgeWorker) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns no config IDs, since a Forge Worker compiles to no
// entries.
func (forgeWorker) GetConfigIDs() []string {
	return nil
}

// Compile validates the node's parameters and compiles to no entries.
func (forgeWorker) Compile(node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	_, err := forgeWorkerDeclaration(node.Node)
	return nil, err
}

// GetCapability returns the forge-worker capability that carries the declared
// capacity. It rejects a Worker that is not in the World, cannot receive work
// from a Cluster, or has no keypair for the Device's peer.
func (forgeWorker) GetCapability(
	ctx context.Context,
	ws world.WorldState,
	node *s4wave_flowgraph.PlacedFlowgraphNode,
) (*s4wave_device.DeviceCapability, error) {
	// Read the declaration from the node's parameters.
	declaration, err := forgeWorkerDeclaration(node.Node)
	if err != nil {
		return nil, err
	}

	// Require the Worker to exist in a Cluster and to act as this Device's peer
	// before the plugin hosts it.
	if err := verifyForgeWorkerLink(ctx, ws, declaration.GetWorkerObjectKey()); err != nil {
		return nil, err
	}
	if err := verifyForgeWorkerPeer(ctx, ws, declaration.GetWorkerObjectKey(), node.DevicePeerID); err != nil {
		return nil, err
	}
	return &s4wave_device.DeviceCapability{
		Kind:  s4wave_device.DeviceCapabilityKindForgeWorker,
		Label: "Forge Worker",
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			LocalPolicyRef: node.CapabilityID(),
			LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
		},
		Link: &s4wave_device.DeviceCapabilityLink{
			ObjectKey: declaration.GetWorkerObjectKey(),
			TypeId:    forge_worker.WorkerTypeID,
		},
		WorkerDeclaration: declaration,
	}, nil
}

// forgeWorkerDeclaration returns the declaration the node's parameters make, or
// an error naming the parameter that is missing or malformed.
func forgeWorkerDeclaration(node *s4wave_flowgraph.FlowgraphNode) (*s4wave_device.ForgeWorkerDeclaration, error) {
	// Read the parameters the declaration comes from.
	params := node.GetParameters()

	// Require the Worker the Device hosts.
	workerObjectKey := strings.TrimSpace(params[forgeWorkerWorkerParameter])
	if workerObjectKey == "" {
		return nil, errors.Errorf("parameter %q is required", forgeWorkerWorkerParameter)
	}

	// Require the capacity the Worker offers.
	milliCPU, err := strconv.ParseUint(strings.TrimSpace(params[forgeWorkerMilliCPUParameter]), 10, 64)
	if err != nil || milliCPU == 0 {
		return nil, errors.Errorf("parameter %q must be a positive integer", forgeWorkerMilliCPUParameter)
	}
	memoryBytes, err := strconv.ParseUint(strings.TrimSpace(params[forgeWorkerMemoryBytesParameter]), 10, 64)
	if err != nil || memoryBytes == 0 {
		return nil, errors.Errorf("parameter %q must be a positive integer", forgeWorkerMemoryBytesParameter)
	}

	// Require at least one runtime backend, each named once. Sort them so the
	// order the node lists them in does not change the declaration.
	var backends []string
	for backend := range strings.SplitSeq(params[forgeWorkerBackendsParameter], ",") {
		backend = strings.TrimSpace(backend)
		if backend == "" || strings.ContainsAny(backend, " \t\r\n") {
			return nil, errors.Errorf("parameter %q must list backends separated by commas", forgeWorkerBackendsParameter)
		}
		for _, seen := range backends {
			if seen == backend {
				return nil, errors.Errorf("parameter %q lists backend %q twice", forgeWorkerBackendsParameter, backend)
			}
		}
		backends = append(backends, backend)
	}
	slices.Sort(backends)
	return &s4wave_device.ForgeWorkerDeclaration{
		WorkerObjectKey: workerObjectKey,
		MilliCpu:        milliCPU,
		MemoryBytes:     memoryBytes,
		Backends:        backends,
	}, nil
}

// verifyForgeWorkerLink proves the declared Worker object exists, carries the
// forge/worker type quad, and belongs to a Cluster that can assign it work.
func verifyForgeWorkerLink(ctx context.Context, ws world.WorldState, workerObjectKey string) error {
	// Check the worker type and require it to belong to a Cluster.
	{
		_, objectState, err := forge_worker.LookupWorker(ctx, ws, workerObjectKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			return errors.Wrapf(err, "verify forge worker %q", workerObjectKey)
		}
	}
	if err := forge_worker.CheckWorkerType(ctx, ws, workerObjectKey); err != nil {
		return errors.Wrapf(err, "verify forge worker %q", workerObjectKey)
	}
	clusterKeys, err := forge_cluster.ListWorkerClusters(ctx, ws, workerObjectKey)
	if err != nil {
		return errors.Wrapf(err, "list clusters for Forge Worker %q", workerObjectKey)
	}
	if len(clusterKeys) == 0 {
		return errors.Errorf("Forge Worker %q is not assigned to a Cluster", workerObjectKey)
	}
	for _, clusterKey := range clusterKeys {
		if err := forge_cluster.CheckClusterType(ctx, ws, clusterKey); err != nil {
			return errors.Wrapf(err, "verify Cluster %q for Forge Worker %q", clusterKey, workerObjectKey)
		}
	}
	return nil
}

// verifyForgeWorkerPeer proves the Worker's keypairs include devicePeerID, the
// peer the plugin signs for. A Worker linked to other peers cannot take work
// from its Cluster on this Device.
func verifyForgeWorkerPeer(ctx context.Context, ws world.WorldState, workerObjectKey, devicePeerID string) error {
	keypairs, _, err := forge_worker.CollectWorkerKeypairs(ctx, ws, workerObjectKey)
	if err != nil {
		return errors.Wrapf(err, "collect keypairs for Forge Worker %q", workerObjectKey)
	}

	// Match one keypair against the Device's peer, keeping the others to report.
	var workerPeerIDs []string
	for _, keypair := range keypairs {
		peerID, err := keypair.ParsePeerID()
		if err != nil {
			continue
		}
		if peerID.String() == devicePeerID {
			return nil
		}
		workerPeerIDs = append(workerPeerIDs, peerID.String())
	}
	if len(workerPeerIDs) == 0 {
		return errors.Errorf("Forge Worker %q has no usable keypair", workerObjectKey)
	}
	return errors.Errorf("Forge Worker %q peers %q do not include Device peer %q", workerObjectKey, workerPeerIDs, devicePeerID)
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = forgeWorker{}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeCapability = forgeWorker{}
