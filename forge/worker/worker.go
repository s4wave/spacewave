// Package forge_worker defines the Worker, the named identity under which a
// Device runs Forge Executions, and the Placement that binds work to it.
//
// A Worker is a World object with a DNS-label name. The keypairs linked to it
// are the peers that act as the Worker, and a Cluster links to it to schedule
// work on it. A Placement freezes one Worker object key and one Device peer
// linked to that Worker for a Job, Pass or Execution, and ValidateLinked checks
// that the peer still belongs to the Worker. The controller subpackage runs on
// the Worker's peer and starts controllers for the objects assigned to it.
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
