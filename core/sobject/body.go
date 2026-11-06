package sobject

import (
	"context"
)

// Body is the state a SharedObject agrees on, such as a Space's World. The
// SharedObject never reads the state. A BodyDriver replays the agreed order of
// operations into the body, resets it to each checkpoint the SharedObject
// adopts, and asks it for the state a new checkpoint carries. The driver calls
// one method at a time; only the store Root returns may run beside them.
//
// An index counts the operations above the checkpoint the body last
// installed: index 0 is the first operation above it.
type Body interface {
	// Root returns the encoded state after the first n operations above the
	// installed checkpoint, and store, which holds that state so a checkpoint
	// may carry it: once store returns nil, the body keeps the state readable
	// after a restart until an Install moves past it. Root returns quickly and
	// keeps the state readable in memory until store returns, so the driver
	// may run store beside its later calls, off its apply loop. The caller
	// calls store once.
	// Returns nil data while the body has not applied n operations.
	Root(ctx context.Context, n int) (data []byte, store func(context.Context) error, err error)
	// Apply applies op as the operation at index above the installed
	// checkpoint, after the operations at the lower indexes.
	// index is at most the number of operations applied so far. A lower index
	// means the order changed there: the body continues from its state after
	// the operations before index.
	// Returns the reason the body refused op, or empty when it applied.
	// An error leaves op unapplied, and the driver applies it again later, so
	// an error is never a refusal: a canceled read or a missing block returns
	// an error.
	Apply(ctx context.Context, index int, op *OrderedOperation) (string, error)
	// Install resets the body to the state checkpoint carries in its
	// StateData.
	// covered is how many of the operations the body applied above its
	// previous checkpoint the new one covers, or -1 when they are not the
	// first operations the body applied.
	// Returns whether the body kept the operations it applied after the
	// covered ones. It keeps them only when covered is not -1 and its state
	// after the covered operations equals the checkpoint's; otherwise the
	// driver applies every operation above checkpoint again.
	Install(ctx context.Context, checkpoint *SOCheckpointInner, covered int) (bool, error)
}

// OrderedOperation is one operation of the agreed order, as a Body applies it.
type OrderedOperation struct {
	// Hash identifies the operation.
	Hash []byte
	// Inner is the verified operation, with its data still encoded.
	Inner *SOOperationInner
	// Author is the author's participant entry in the config the operation
	// was written under, or nil when the operation applies nothing.
	Author *SOParticipantConfig
	// Data is the operation's decoded data, or nil when it applies nothing: an
	// acknowledgment, or an operation every member refuses.
	Data []byte
	// Refused is the reason every member refuses the operation before its body
	// sees it, such as an author the config removed or an equivocation, or
	// empty. The body applies nothing for it and returns it as the outcome.
	Refused string
}
