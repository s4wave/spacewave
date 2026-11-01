package sobject_world_engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// admissionPreparationKey scopes preparation to the engine that produced it.
type admissionPreparationKey struct{ store string }

// admissionRetry retains only preparation needs after a discarded candidate.
type admissionRetry struct {
	// engine bounds the wait and validates the original publication history.
	engine *soEngine
	// epoch is the history before the original body ran.
	epoch uint64
	// origin is the original predecessor, retained across body retries.
	origin *block.PublicationReceipt
	// bytes is the last save's required block capacity, or zero before sizing.
	bytes int64
	// state asks preparation to wait for provider state intent first.
	state bool
	// settled asks a declined publication's retry to use durable provider state.
	settled bool
}

// Error describes a retry state rather than an admitted write failure.
func (r *admissionRetry) Error() string {
	return "World admission needs preparation outside caller writers"
}

// check refuses retired or reinstalled history, including an ancestor failure
// whose successor receipt has not resolved yet.
func (r *admissionRetry) check() error {
	if r.engine.lifetime.Err() != nil {
		return world_block.ErrEngineClosed
	}
	if r.origin != nil && r.origin.Err() != nil {
		return block.NewPublicationDependencyError(r.origin.Err())
	}
	if r.engine.publicationEpoch.Load() != r.epoch {
		return block.NewPublicationDependencyError(errors.New("world reinstalled during publication preparation"))
	}
	return nil
}

// Prepare takes the candidate lease before capacity, so another candidate cannot
// own that lease while waiting for the capacity this preparation would hold.
// State intent is released before reserving bytes and before any retry body.
func (r *admissionRetry) Prepare(ctx context.Context) (world.AdmissionPreparation, error) {
	// Bind waits to both caller and engine, rejecting failed history first.
	ctx, cancel := r.engine.publicationContext(ctx)
	defer cancel()
	if err := r.check(); err != nil {
		return nil, err
	}

	// Take the sole candidate lease before any capacity claim.
	store := r.engine.so.GetBlockStore().(block.PublicationReserver)
	lease, err := store.AcquirePublicationPreparation(ctx)
	if err != nil {
		return nil, err
	}
	p := &admissionPreparation{retry: r, lease: lease}
	owned := false
	defer func() {
		if !owned {
			p.Release()
		}
	}()

	// Let a direct writer finish without retaining its intent over a callback.
	if r.state {
		if reserver := stateWriteReserver(r.engine.so); reserver != nil {
			claim, err := reserver.ReserveStateWrite(ctx, r.settled)
			if err != nil {
				return nil, err
			}
			if claim != nil {
				claim.Release()
			}
		}
	}

	// A failed state drain must not wait for unrelated remaining capacity.
	if err := r.check(); err != nil {
		return nil, err
	}

	// Reserve the previous estimate and recheck history before a new body runs.
	p.capacity, err = store.ReservePublication(ctx, r.bytes)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.check(); err != nil {
		return nil, err
	}
	owned = true
	return p, nil
}

// admissionPreparation owns one candidate lease and an unused capacity claim.
type admissionPreparation struct {
	// retry describes the history and settled-state requirement of this attempt.
	retry *admissionRetry
	// lease bounds candidates before either an engine or caller writer.
	lease func()
	// capacity is consumed only by this attempt's SubmitAtomic call.
	capacity block.PublicationReservation
	// used prevents a context from opening two candidates with the same lease.
	used atomic.Bool
	// released refuses contexts retained after their owner released preparation.
	released atomic.Bool
	// once returns the owned resources once, including repeated Discard calls.
	once sync.Once
}

// WithContext binds an opaque claim without exposing it to other publications.
func (p *admissionPreparation) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, admissionPreparationKey{p.retry.engine.so.GetBlockStore().GetID()}, p)
}

// Release returns capacity and the candidate lease after the attempt ends.
func (p *admissionPreparation) Release() {
	p.once.Do(func() {
		p.released.Store(true)
		if p.capacity != nil {
			p.capacity.Release()
		}
		p.lease()
	})
}

// take transfers this preparation to one NewTransaction call.
func (p *admissionPreparation) take(engine *soEngine) error {
	if p.retry.engine != engine {
		return world_block.ErrEngineClosed
	}
	if err := p.retry.check(); err != nil {
		return err
	}
	if p.released.Load() || !p.used.CompareAndSwap(false, true) {
		return errors.New("admission preparation is no longer available")
	}
	return p.retry.check()
}

// admissionRetry returns an unadmitted wait without retaining candidate blocks.
func (t *soEngineWriteTx) admissionRetry(bytes int64, state, settled bool) error {
	origin := t.baseReceipt
	if t.admissionPreparation != nil {
		origin = t.admissionPreparation.retry.origin
		settled = settled || t.admissionPreparation.retry.settled
	}
	return &admissionRetry{engine: t.eng, epoch: t.publicationEpoch, origin: origin, bytes: bytes, state: state || settled, settled: settled}
}

// checkPublicationBase rejects a failed fork and, for a discarded-body retry,
// the original history. An ordinary fork may replay across a later failure.
func (t *soEngineWriteTx) checkPublicationBase() error {
	if t.baseReceipt != nil && t.baseReceipt.Err() != nil {
		return block.NewPublicationDependencyError(t.baseReceipt.Err())
	}
	if t.admissionPreparation != nil {
		return t.admissionPreparation.retry.check()
	}
	if t.eng.lifetime.Err() != nil {
		return world_block.ErrEngineClosed
	}
	return nil
}

// _ checks the public preparation contracts.
var (
	_ world.AdmissionRetry       = (*admissionRetry)(nil)
	_ world.AdmissionPreparation = (*admissionPreparation)(nil)
)
