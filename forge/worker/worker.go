package forge_worker

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/identity"
)

// NewWorkerBlock constructs a new Worker block.
func NewWorkerBlock() block.Block {
	return &Worker{}
}

// UnmarshalWorker unmarshals a worker block from the cursor.
func UnmarshalWorker(ctx context.Context, bcs *block.Cursor) (*Worker, error) {
	return block.UnmarshalBlock[Worker](ctx, bcs)
}

// Validate checks the Worker object cursorily.
func (e *Worker) Validate() error {
	if err := identity.ValidateEntityID(e.GetName()); err != nil {
		return err
	}
	return nil
}

// _ is a type assertion
var _ block.Block = (*Worker)(nil)
