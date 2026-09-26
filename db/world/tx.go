package world

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
)

// Tx implements the world state transaction interfaces.
//
// Concurrent calls to WorldState functions should be supported.
type Tx interface {
	// WorldState contains the world read/write ops.
	WorldState
	// Tx contains the transaction Commit/Discard ops.
	tx.Tx
}

// orderedCommitKey marks a context whose write transactions may commit ordered.
type orderedCommitKey struct{}

// WithOrderedCommit marks write transactions committed with ctx as allowed to
// commit ordered: Commit returns once the change is applied and safe against a
// process crash, and the change becomes durable at the engine's next Sync or
// its storage's own durability point. Engines without ordered commits ignore
// the mark and commit in full.
func WithOrderedCommit(ctx context.Context) context.Context {
	return context.WithValue(ctx, orderedCommitKey{}, true)
}

// OrderedCommit reports whether ctx carries WithOrderedCommit.
func OrderedCommit(ctx context.Context) bool {
	ordered, _ := ctx.Value(orderedCommitKey{}).(bool)
	return ordered
}
