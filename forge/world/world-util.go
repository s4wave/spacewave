package forge_world

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	identity_world "github.com/s4wave/spacewave/identity/world"
)

const keypairObjectGraphPathLimit uint32 = 1_000_000

// The world is used for managing objects, i.e.:
// Cluster, Job, Target, Task, Pass, Execution
var ForgeObjectTypeIDs = []string{
	forge_cluster.ClusterTypeID,
	forge_job.JobTypeID,
	forge_task.TaskTypeID,
	forge_pass.PassTypeID,
	forge_execution.ExecutionTypeID,
	forge_worker.WorkerTypeID,
}

// ListKeypairObjects lists all Forge objects linked to by the Keypair.
// It returns the object keys in graph traversal order.
func ListKeypairObjects(ctx context.Context, w world.WorldState, keypairKeys ...string) ([]string, error) {
	objKeys, err := world.CollectGraphPathStepWithKeys(
		ctx,
		w,
		keypairKeys,
		world.GraphPathDirectionIn,
		identity_world.PredObjectToKeypair.String(),
		keypairObjectGraphPathLimit,
	)
	if err != nil {
		return nil, err
	}

	metadata, err := world_types.GetObjectMetadataBatch(ctx, w, objKeys)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, md := range metadata {
		if slices.Contains(ForgeObjectTypeIDs, md.TypeID) {
			result = append(result, md.ObjectKey)
		}
	}
	return result, nil
}
