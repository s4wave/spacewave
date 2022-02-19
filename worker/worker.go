package forge_worker

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/identity"
	"github.com/cayleygraph/quad"
	"github.com/golang/protobuf/proto"
)

const (
	// WorkerTypeID is the type identifier for a Worker.
	WorkerTypeID = "forge/worker"

	// PredWorkerToKeypair is the predicate linking Worker to a Keypair.
	PredWorkerToKeypair = quad.IRI("forge/worker-keypair")
)

// NewWorkerBlock constructs a new Worker block.
func NewWorkerBlock() block.Block {
	return &Worker{}
}

// NewWorkerToKeypairQuad creates a quad linking a Worker to a Execution.
func NewWorkerToKeypairQuad(workerObjKey, keypairObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		workerObjKey,
		PredWorkerToKeypair.String(),
		keypairObjKey,
		"",
	)
}

// LookupWorkerOp performs the lookup operation for the Worker op types.
func LookupWorkerOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case WorkerCreateOpId:
		return &WorkerCreateOp{}, nil
	}
	return nil, nil
}

// UnmarshalWorker unmarshals a worker block from the cursor.
func UnmarshalWorker(bcs *block.Cursor) (*Worker, error) {
	vi, err := bcs.Unmarshal(NewWorkerBlock)
	if err != nil {
		return nil, err
	}
	if vi == nil {
		return nil, nil
	}
	b, ok := vi.(*Worker)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return b, nil
}

// Validate performs cursory checks of the Worker object.
func (e *Worker) Validate() error {
	if err := identity.ValidateEntityID(e.GetName()); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Worker) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Worker) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// _ is a type assertion
var _ block.Block = ((*Worker)(nil))
