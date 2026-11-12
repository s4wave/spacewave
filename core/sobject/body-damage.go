package sobject

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// StoreRefusal is the store refusal the latest attempts of one write met in
// a row: the store refused a block whose child no store holds
// (block.ErrMissingChild). A write refused twice in a row is damaged: replay
// cannot heal it, so its writer stops trying until the next mount.
type StoreRefusal struct {
	// err is the latest refusal. It names the write, the block and its
	// child by their block references.
	err error
	// repeats counts the attempts in a row it refused.
	repeats int
}

// storeRefusalRepeats is how many attempts in a row a write is refused before
// it is damaged. The first refusal retries once: a page replay computes again
// may heal it on the next attempt.
const storeRefusalRepeats = 2

// NoteStoreRefusal returns the refusal state of a write after an attempt that
// ended with err: a missing-child refusal counts one more, success clears it,
// and any other error leaves it.
func NoteStoreRefusal(r *StoreRefusal, err error) *StoreRefusal {
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, block.ErrMissingChild):
		return r
	case r == nil:
		return &StoreRefusal{err: err, repeats: 1}
	default:
		return &StoreRefusal{err: err, repeats: r.repeats + 1}
	}
}

// Damage returns the refusal once it repeated, or nil.
func (r *StoreRefusal) Damage() error {
	if r == nil || r.repeats < storeRefusalRepeats {
		return nil
	}
	return r.err
}

// DamagedBody is a Body whose own writes, such as a replay cursor save, may
// be refused beyond what replay heals.
type DamagedBody interface {
	Body
	// Damaged returns the store refusal of the body's own writes replay
	// could not heal, or nil.
	Damaged() error
}

// damage returns the store refusal that damaged the body, or nil: a step's,
// a checkpoint store's or the body's own write's that repeated. The caller
// holds mtx.
func (d *BodyDriver) damage() error {
	for _, r := range []*StoreRefusal{d.stepRefusal, d.storeRefusal} {
		if err := r.Damage(); err != nil {
			return err
		}
	}
	if b, ok := d.body.(DamagedBody); ok {
		return b.Damaged()
	}
	return nil
}
