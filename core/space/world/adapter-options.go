package space_world

import (
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/block"
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
	// FenceTimeout bounds the wait of a storage fence for every writer
	// device's answer. Zero is defaultFenceTimeout.
	FenceTimeout time.Duration
	// Verbose logs every operation's outcome.
	Verbose bool
	// ReaderPinDelay sets how long a replaced head a reader still holds
	// waits before the engine pins it, or is nil for the engine's default.
	ReaderPinDelay *time.Duration

	// transient replays without holding roots or saving the cursor, for a
	// one-off replay beside the Space's own adapter.
	transient bool
	// pagesStore is the store the kept pages land in past the bound, such as
	// a transient replay's stage, or nil for the SharedObject's block store.
	pagesStore block.StoreOps
}
