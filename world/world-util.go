package forge_world

import (
	"context"

	"github.com/aperturerobotics/cayley"
	forge_cluster "github.com/aperturerobotics/forge/cluster"
	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_job "github.com/aperturerobotics/forge/job"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_task "github.com/aperturerobotics/forge/task"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	identity_world "github.com/aperturerobotics/identity/world"
)

// The world is used for managing objects, i.e.:
// Cluster, Job, Target, Task, Pass, Execution
var ForgeObjectTypeIDs = []string{
	forge_cluster.ClusterTypeID,
	forge_job.JobTypeID,
	forge_task.TaskTypeID,
	forge_pass.PassTypeID,
	forge_execution.ExecutionTypeID,
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
