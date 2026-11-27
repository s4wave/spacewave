//go:build !js && !wasip1

package world_recovery

import (
	"context"

	"github.com/s4wave/spacewave/db/blocktype"
)

// Options supplies native body constructors and bounded fault/step hooks.
type Options struct {
	// Lookup resolves a type established by each historical World's own graph.
	Lookup func(context.Context, string) (blocktype.BlockType, error)
	// InterpretationID binds Lookup's constructor rules into resumable run identity.
	InterpretationID string
	// Witness validates native history and returns accepted/before/cover/refused World roots.
	Witness func(context.Context, *WitnessInput, func([]byte) ([]byte, error)) (*WitnessEvidence, error)
	// BeforeStep injects a fault before a durable scratch step.
	// A nil BeforeStep runs normally.
	BeforeStep func(Phase) error
	// Committed observes each durable batch commit, for interruption run-throughs.
	Committed func(*Result) error
	// BeforeRead injects an input read failure without adding source capabilities.
	BeforeRead func() error
	// BeforeDecode injects a transient decoder failure at a durable boundary.
	BeforeDecode func() error
	// BeforeCommit injects interruption after a step's rows and result are prepared, before the step joins its uncommitted batch.
	BeforeCommit func(*Result) error
}
