//go:build space_adapter_decl

package space_world

import (
	"context"
	"errors"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// errAdapterUnbuilt marks an Adapter method declared for review.
var errAdapterUnbuilt = errors.New("space world adapter: declared, not built")

// Adapter serves a Space whose body is a World: it is the sobject.Body the
// SharedObject replays its order into, the world.Authority of the one World
// engine that serves the Space, and the holder of the named roots replay
// needs on the SharedObject's block store.
//
// A local write is a turn of the engine. Its candidate is queued as an
// operation and decided once the driver applies it: Accepted when the
// operation applied at its position, Superseded when replay refused it as a
// conflict with an operation the writer did not build on, and Rejected for
// any other refusal. A decided candidate is durable at once, because the
// SharedObject's state holds its operation durably and replay rebuilds the
// head from it. The next writer's turn begins on the admitted candidate
// before it is decided, so writes pipeline.
//
// The adapter keeps the pages of every World it computes in a PendingStore
// and stores a World only when a named root moves onto it: the checkpoint,
// the replay base and the replay cursor. Pages written and replaced between
// those Worlds never reach the Volume.
//
// Invariants:
//   - The adopted checkpoint's World is held, by replay-base or checkpoint,
//     at every point, so a restart always finds the World replay starts from.
//   - The replay cursor record names a World only after replay-cursor holds
//     it.
//   - The writer head is the installed head or a candidate admitted after it,
//     and a candidate's base is the writer head when it was admitted.
//   - The installed head never loses an operation an earlier installed head
//     applied, until replay places that operation again or refuses it.
type Adapter struct {
	// le is the logger.
	le *logrus.Entry
	// so is the Space's SharedObject.
	so sobject.SharedObject
	// opts are the adapter's options.
	opts AdapterOptions
	// pages keeps the World pages in front of so's block store. The serving
	// engine and the replay engine read and write through it.
	pages *block.PendingStore
	// engine serves the Space World over this Authority.
	engine *world_block.Engine
	// replay holds the positions above the installed checkpoint and the
	// replay engine that computes them. The driver goroutine owns it.
	replay *spaceReplay
	// queue queues admitted candidates as operations, in admission order.
	queue *opQueue
	// turnMtx is held from Begin to the turn's Release, so one local writer
	// proposes at a time.
	turnMtx csync.Mutex

	// bcast guards the fields below and wakes head watchers.
	bcast broadcast.Broadcast
	// head is the installed head: the World after the last position the
	// driver applied. Guarded by bcast.
	head world.Head
	// writer is the head the next turn builds on: the last admitted candidate,
	// or head when none waits. Guarded by bcast.
	writer world.Head
	// pending are the admitted candidates not yet decided, in admission order.
	// Guarded by bcast.
	pending []*spaceAdmission
}

// NewAdapter mounts the Space World of so: it opens the PendingStore over
// so's block store, reads the replay roots and the saved cursor, and builds
// the engine at the head they reach. Execute runs the queue and attaches the
// adapter to so as its body.
func NewAdapter(ctx context.Context, le *logrus.Entry, so sobject.SharedObject, opts AdapterOptions) (*Adapter, error) {
	return nil, errAdapterUnbuilt
}

// GetEngine returns the World engine serving the Space.
func (a *Adapter) GetEngine() *world_block.Engine {
	return a.engine
}

// Execute runs the operation queue and attaches the adapter to the
// SharedObject as its body until ctx ends.
func (a *Adapter) Execute(ctx context.Context) error {
	return errAdapterUnbuilt
}

// Head returns the installed head.
func (a *Adapter) Head(ctx context.Context) (world.Head, error) {
	return world.Head{}, errAdapterUnbuilt
}

// WatchHead calls fn with the installed head, then with each later installed
// head, until release is called. Every head is durable.
func (a *Adapter) WatchHead(ctx context.Context, fn func(world.Head)) (func(), error) {
	return nil, errAdapterUnbuilt
}

// Begin waits for the local writer turn. The turn's Base is the writer head,
// which is not durable while a candidate admitted before it waits.
func (a *Adapter) Begin(ctx context.Context) (world.Turn, error) {
	return nil, errAdapterUnbuilt
}

// Root names the World after the first n positions under the checkpoint
// root, storing its kept pages with the move, and returns its encoded
// InnerState.
func (a *Adapter) Root(ctx context.Context, n int) ([]byte, error) {
	return nil, errAdapterUnbuilt
}

// Apply computes the World after op at index. The World before index and
// op's local ID matching an admitted candidate built on that World adopts the
// candidate's root without running op again; otherwise the replay engine runs
// op. A position past the installed head installs the head after it and
// decides the candidates it places.
func (a *Adapter) Apply(ctx context.Context, index int, op *sobject.OrderedOperation) (string, error) {
	return "", errAdapterUnbuilt
}

// Install moves the replay base to checkpoint's World and releases the
// checkpoint root, keeping the positions after the covered ones when the
// World after them is the checkpoint's.
func (a *Adapter) Install(ctx context.Context, checkpoint *sobject.SOCheckpointInner, covered int) (bool, error) {
	return false, errAdapterUnbuilt
}

// _ is a type assertion
var (
	_ world.Authority = (*Adapter)(nil)
	_ sobject.Body    = (*Adapter)(nil)
)
