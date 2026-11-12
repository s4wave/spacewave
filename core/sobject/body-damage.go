package sobject

import (
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// StoreRefusal is the store refusal the latest attempts of one write met in
// a row: the store refused a block whose child no store holds
// (block.MissingChildError). A write refused twice in a row is damaged:
// replay cannot heal it, so its writer tries again only once per
// storeDamageRetry, and the first attempt that stores clears it.
type StoreRefusal struct {
	// refused is the latest refusal.
	refused *block.MissingChildError
	// since is when the first refusal in the row happened.
	since time.Time
	// at is when the latest refusal happened.
	at time.Time
	// repeats counts the attempts in a row it refused.
	repeats int
}

// storeRefusalRepeats is how many attempts in a row a write is refused before
// it is damaged. The first refusal retries once: a page replay computes again
// may heal it on the next attempt.
const storeRefusalRepeats = 2

// storeDamageRetry is how long a damaged write waits after its latest refusal
// before it tries again, so a cause healed outside replay, such as a block
// restored from a backup, clears the damage without a remount, while a cause
// that stays costs one refused attempt a minute.
const storeDamageRetry = time.Minute

// NoteStoreRefusal returns the refusal state of a write after an attempt that
// ended with err at now: a missing-child refusal counts one more, success
// clears it, and any other error leaves it.
func NoteStoreRefusal(r *StoreRefusal, err error, now time.Time) *StoreRefusal {
	// A write that landed clears the refusal.
	if err == nil {
		return nil
	}

	// Count only a missing-child refusal.
	var refused *block.MissingChildError
	if !errors.As(err, &refused) {
		return r
	}
	if r == nil {
		return &StoreRefusal{refused: refused, since: now, at: now, repeats: 1}
	}
	return &StoreRefusal{refused: refused, since: r.since, at: now, repeats: r.repeats + 1}
}

// Due reports whether the write may be tried at now: it is not damaged, or
// storeDamageRetry passed since its latest refusal.
func (r *StoreRefusal) Due(now time.Time) bool {
	return r.Damage() == nil || !now.Before(r.at.Add(storeDamageRetry))
}

// Damage returns the refusal once it repeated, or nil.
func (r *StoreRefusal) Damage() *SOStoreDamage {
	if r == nil || r.repeats < storeRefusalRepeats {
		return nil
	}
	return &SOStoreDamage{
		Operation: r.refused.Op,
		Block:     r.refused.Block,
		Child:     r.refused.Child,
		Since:     timestamppb.New(r.since),
		Refusals:  uint32(r.repeats), //nolint:gosec // a count of attempts.
	}
}

// DamagedBody is a Body whose own writes, such as a replay cursor save, may
// be refused beyond what replay heals.
type DamagedBody interface {
	Body
	// Damaged returns the store refusal of the body's own writes replay
	// could not heal, or nil.
	Damaged() *SOStoreDamage
}

// damage returns the store refusal that damaged the body, or nil: a step's,
// a checkpoint store's or the body's own write's that repeated. The caller
// holds mtx.
func (d *BodyDriver) damage() *SOStoreDamage {
	for _, r := range []*StoreRefusal{d.stepRefusal, d.storeRefusal} {
		if damage := r.Damage(); damage != nil {
			return damage
		}
	}
	if b, ok := d.body.(DamagedBody); ok {
		return b.Damaged()
	}
	return nil
}
