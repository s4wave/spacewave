//go:build space_adapter_decl

package space_world

import (
	"github.com/s4wave/spacewave/db/world"
)

// AdapterOptions configures an Adapter.
type AdapterOptions struct {
	// EngineID identifies the Space World on the bus.
	EngineID string
	// LookupOp resolves the World operations replay applies.
	LookupOp world.LookupOp
	// MaxPendingBytes bounds the World pages kept in memory before the
	// adapter names a World under the replay cursor root.
	MaxPendingBytes int
	// Verbose logs every operation's outcome.
	Verbose bool
}
