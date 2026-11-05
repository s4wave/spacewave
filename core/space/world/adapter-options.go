package space_world

import (
	"github.com/aperturerobotics/controllerbus/bus"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// defaultMaxPendingBytes bounds the World pages an Adapter keeps in memory
// when its options set no bound.
const defaultMaxPendingBytes = 64 << 20

// AdapterOptions configures an Adapter.
type AdapterOptions struct {
	// Bus resolves World operations and plugin handlers, or is nil.
	Bus bus.Bus
	// StepFactorySet builds the World block transformers.
	StepFactorySet *block_transform.StepFactorySet
	// EngineID identifies the Space World on the bus.
	EngineID string
	// OpTypes resolves the World operations replay applies before the bus.
	OpTypes OpTypes
	// DisableLookup resolves operations with OpTypes alone.
	DisableLookup bool
	// InitWorldOp overrides the operation an owner queues to initialize an
	// empty World.
	InitWorldOp *InitWorldOp
	// MaxPendingBytes bounds the World pages kept in memory before the
	// adapter stores the head World under the replay cursor root. Zero is
	// defaultMaxPendingBytes.
	MaxPendingBytes int
	// Verbose logs every operation's outcome.
	Verbose bool
}
