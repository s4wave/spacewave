package flowgraph_run

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// stepPlacement returns the Forge placement of a Step's next activation. A
// Step with a Device placement edge runs on that Device's Forge Worker. Any
// other Step runs where the run's Job is placed, which may be anywhere. When
// the Device cannot take work, the returned reason pauses the run at the Step,
// so a resume retries once the Device is ready.
func (r *runner) stepPlacement(ctx context.Context, nodeID string) (*forge_worker.Placement, string, error) {
	// Follow the Step's Device edge. A Step without one inherits the Job's
	// placement.
	edge := r.run.GetPlacements()[nodeID].First()
	if edge.GetKind() != s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE {
		job, err := forge_job.LookupJobBody(ctx, r.ws, r.run.GetJobKey())
		if err != nil {
			return nil, "", err
		}
		return job.GetPlacement(), "", nil
	}

	// Resolve the Device to its Worker and peer at this revision.
	deviceKey := edge.GetObjectKey()
	dev, err := world.LookupObjectBody[s4wave_device.Device](ctx, r.ws, deviceKey)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, "step device " + deviceKey + " does not exist", nil
	}
	if err != nil {
		return nil, "", err
	}
	placement, err := forge_runtime.DeviceWorkerPlacement(ctx, r.ws, dev)
	if err != nil {
		return nil, "step device " + deviceKey + ": " + err.Error(), nil
	}
	return placement, "", nil
}
