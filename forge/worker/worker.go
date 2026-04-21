package forge_worker

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
)

const (
	// WorkerTypeID is the type identifier for a Worker.
	WorkerTypeID = "forge/worker"
)

// NewWorkerBlock constructs a new Worker block.
func NewWorkerBlock() block.Block {
	return &Worker{}
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
func UnmarshalWorker(ctx context.Context, bcs *block.Cursor) (*Worker, error) {
	return block.UnmarshalBlock[*Worker](ctx, bcs, NewWorkerBlock)
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
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Worker) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*Worker)(nil))
