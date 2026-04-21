package forge_world

import (
	"context"

	"github.com/aperturerobotics/cayley"
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
// returns: Cluster, Pass, Task, Execution
// returns list of object keys
func ListKeypairObjects(ctx context.Context, w world.WorldState, keypairKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		keypairKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			// In: traverse to all objects linking to the keypair.
			p = p.In(identity_world.PredObjectToKeypair)
			// Limit to types recognized as Forge types
			p = world_types.LimitNodesToTypes(p, ForgeObjectTypeIDs...)
			return p, nil
		},
	)
}
