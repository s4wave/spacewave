package forge_world

import (
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
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
