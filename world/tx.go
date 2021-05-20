package world

import "github.com/aperturerobotics/hydra/tx"

// Tx implements the world state transaction interfaces.
//
// Concurrent calls to WorldState functions should be supported.
type Tx interface {
	// WorldState contains the world read/write ops.
	WorldState
	// Tx contains the transaction Confirm/Discard ops.
	tx.Tx
}
