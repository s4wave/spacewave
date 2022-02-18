package forge_world

import (
	forge_cluster "github.com/aperturerobotics/forge/cluster"
	forge_execution_tx "github.com/aperturerobotics/forge/execution/tx"
	forge_pass_tx "github.com/aperturerobotics/forge/pass/tx"
	forge_worker "github.com/aperturerobotics/forge/worker"
	"github.com/aperturerobotics/hydra/world"
)

// LookupWorldOp looks up the operation with the type id.
var LookupWorldOp world.LookupOp = world.NewLookupOpFromSlice([]world.LookupOp{
	forge_execution_tx.LookupWorldOp,
	forge_pass_tx.LookupWorldOp,
	forge_worker.LookupWorkerOp,
	forge_cluster.LookupClusterOp,
})

// _ is a type assertion
var _ world.LookupOp = LookupWorldOp
