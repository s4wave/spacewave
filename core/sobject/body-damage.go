package sobject

import (
	"context"
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
	// retry is how long the damaged write waits after the latest refusal.
	retry time.Duration
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
// clears it, and any other error leaves it. A damaged write waits
// storeDamageRetry after its latest refusal.
func NoteStoreRefusal(r *StoreRefusal, err error, now time.Time) *StoreRefusal {
	return noteStoreRefusal(r, err, now, storeDamageRetry)
}

// noteStoreRefusal is NoteStoreRefusal with the wait of a damaged write.
func noteStoreRefusal(r *StoreRefusal, err error, now time.Time, retry time.Duration) *StoreRefusal {
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
		return &StoreRefusal{refused: refused, since: now, at: now, repeats: 1, retry: retry}
	}
	return &StoreRefusal{refused: refused, since: r.since, at: now, repeats: r.repeats + 1, retry: retry}
}

// RetryAt returns when the damaged write may be tried again, or the zero time
// while it is not damaged.
func (r *StoreRefusal) RetryAt() time.Time {
	if r.Damage() == nil {
		return time.Time{}
	}
	return r.at.Add(r.retry)
}

// Due reports whether the write may be tried at now: it is not damaged, or
// its retry wait passed since its latest refusal.
func (r *StoreRefusal) Due(now time.Time) bool {
	at := r.RetryAt()
	return at.IsZero() || !now.Before(at)
}

// earliest returns the earliest of the non-zero times, or the zero time.
func earliest(times ...time.Time) time.Time {
	var out time.Time
	for _, t := range times {
		if !t.IsZero() && (out.IsZero() || t.Before(out)) {
			out = t
		}
	}
	return out
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
	// RetryAt returns when Retry next has a damaged write due, or the zero
	// time while none waits on it. The driver wakes then to call Retry. A
	// damaged write the body retries on its own adds nothing.
	RetryAt() time.Time
	// Retry writes again each of the body's own writes Damaged reports,
	// once its refusal is due, whatever the body applied since, so a cause
	// healed outside replay clears the damage on a quiet Space. The driver
	// calls it on its goroutine, after the body applied the order.
	Retry(ctx context.Context) error
	// WatchDamage sets wake, which the body calls when what Damaged or
	// RetryAt returns changes outside a call from the driver, such as when a
	// write it runs in the background lands, so the health shows it at once.
	// The driver calls it once, before Execute.
	WatchDamage(wake func())
}

// retryState returns when the earliest damaged write still waiting out its
// retry comes due, or the zero time, and whether a refused write, the steps',
// the checkpoint stores' or the body's own, is due at now. A refused store
// that has not repeated is due at once. The caller holds mtx.
func (d *BodyDriver) retryState(now time.Time) (time.Time, bool) {
	// Collect the retry time of each damaged write.
	at := []time.Time{d.stepRefusal.RetryAt(), d.storeRefusal.RetryAt()}
	if b, ok := d.body.(DamagedBody); ok {
		at = append(at, b.RetryAt())
	}

	// Sort them into due and still waiting.
	var waiting time.Time
	due := d.storeRefusal != nil && d.storeRefusal.Damage() == nil
	for _, t := range at {
		switch {
		case t.IsZero():
		case !now.Before(t):
			due = true
		default:
			waiting = earliest(waiting, t)
		}
	}
	return waiting, due
}

// backoff is the wait of a write after errors that are not store refusals:
// it doubles from bodyRetryMin to bodyRetryMax with each error in a row, and
// resets once the write lands. The zero value waits for nothing.
type backoff struct {
	// delay is the latest wait, or zero before the first error.
	delay time.Duration
	// at is when the latest wait ends.
	at time.Time
}

// fail notes an error at now and returns the wait before the next attempt.
func (b *backoff) fail(now time.Time) time.Duration {
	b.delay = min(max(2*b.delay, bodyRetryMin), bodyRetryMax)
	b.at = now.Add(b.delay)
	return b.delay
}

// reset clears the wait once the write landed.
func (b *backoff) reset() {
	*b = backoff{}
}

// due reports whether the wait ended at now.
func (b *backoff) due(now time.Time) bool {
	return !now.Before(b.at)
}

// retry reports whether an error waits for an attempt that is due at now.
func (b *backoff) retry(now time.Time) bool {
	return b.delay != 0 && b.due(now)
}

// noteRefusal returns r after an attempt that ended with err, waiting
// damageRetry once damaged. The caller holds mtx.
func (d *BodyDriver) noteRefusal(r *StoreRefusal, err error) *StoreRefusal {
	return noteStoreRefusal(r, err, d.now(), d.damageRetry)
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
