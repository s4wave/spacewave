package writequeue

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// Transaction pairs a World attempt with optional auxiliary state. Discard
// releases the attempt's writer and preparation. Durable remains valid after
// Discard and marks auxiliary reads ready only after the receipt succeeds.
type Transaction interface {
	world.Tx
	// BodyContext binds this attempt's auxiliary writes and preparation to a body.
	BodyContext(context.Context) context.Context
	// UsePreparation transfers ownership of the next admission's prepared resources.
	UsePreparation(world.AdmissionPreparation)
	// Admit publishes the complete attempt without waiting for its durable receipt.
	Admit(context.Context) (world.Admission, error)
	// Durable marks auxiliary state visible through the durable revision.
	Durable(uint64)
}

// basicTx adapts the engine's own admission when no auxiliary store participates.
type basicTx struct {
	world.Tx
	// preparation owns resources obtained before the writer lock.
	preparation world.AdmissionPreparation
}

// wrapBasic uses the World engine's admission and no auxiliary state.
func wrapBasic(_ context.Context, tx world.Tx) Transaction { return &basicTx{Tx: tx} }

// BodyContext binds preparation to the body of this attempt.
func (t *basicTx) BodyContext(ctx context.Context) context.Context {
	if t.preparation != nil {
		return t.preparation.WithContext(ctx)
	}
	return ctx
}

// UsePreparation transfers the prepared claim to this attempt.
func (t *basicTx) UsePreparation(p world.AdmissionPreparation) { t.preparation = p }

// Admit delegates publication to the engine with its prepared claim.
func (t *basicTx) Admit(ctx context.Context) (world.Admission, error) {
	return world.CommitAdmit(t.BodyContext(ctx), t.Tx)
}

// Durable has no auxiliary state to publish.
func (t *basicTx) Durable(uint64) {}

// Discard releases the writer before the prepared claim can be reused.
func (t *basicTx) Discard() {
	t.Tx.Discard()
	if t.preparation != nil {
		t.preparation.Release()
	}
}
