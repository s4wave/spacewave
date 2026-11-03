package feed

import (
	"context"

	"github.com/s4wave/spacewave/db/world/viewstream"
)

// Options binds feeds to their application's World and authorization boundary.
// Callbacks must outlive the hub. Context returns a detached context carrying
// only the requesting stream's scheduling priority. Public removes actor
// authority; Admit checks the requesting subscriber before it joins.
type Options[S any] struct {
	// Context returns a detached context carrying scheduling priority.
	Context func(context.Context) context.Context
	// Public removes actor authority for a public source.
	Public func(S) S
	// Admit checks a subscriber before joining a public source.
	Admit func(context.Context, S) error
	// Watch runs the source and responds to immediate rerun requests.
	Watch func(context.Context, S, *viewstream.Pokes, func(context.Context) error) error
	// Revision returns the source World revision before each read.
	Revision func(context.Context, S) (uint64, error)
	// Own returns the actor's write watch, or nil for an anonymous source.
	Own func(S) func(uint64) (bool, <-chan struct{})
}
