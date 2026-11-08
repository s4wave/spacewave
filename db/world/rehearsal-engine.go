package world

import (
	"context"
	"errors"
)

// ErrRehearsalCommit is returned by Commit on a rehearsal transaction.
var ErrRehearsalCommit = errors.New("a rehearsal transaction cannot commit")

// RehearsalEngine is a World engine that applies operations to a write fork
// of its newest World without taking the writer slot.
type RehearsalEngine interface {
	// NewRehearsalTransaction opens a write fork of the newest World that
	// never commits. Always call Discard when done.
	NewRehearsalTransaction(ctx context.Context) (Tx, error)
}
