package packmove

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// maxReadAttempts bounds how often readStable retries one read that a writer
// outran.
const maxReadAttempts = 8

// refresher is a store that follows a writer in another process when asked, as
// a read-only s4db handle does.
type refresher interface {
	// Refresh publishes the commits and checkpoint another process wrote.
	Refresh(ctx context.Context) error
}

// staleTx is a transaction that can tell the writer has moved past its
// snapshot, as a read-only s4db handle's does.
type staleTx interface {
	// Stale reports that the snapshot may have lost pages to the writer.
	Stale() bool
}

// readStable runs fn on a fresh snapshot of src and returns only when the
// writer did not pass the snapshot meanwhile.
//
// A handle open beside another process's writer holds no pin, so the writer may
// reuse the pages and values of an old snapshot and a read of it can fail a
// checksum, decode garbage or panic. readStable treats each of those as a torn
// read: it discards the snapshot, refreshes the store and runs fn again, up to
// maxReadAttempts times, and then returns the last failure. fn must keep its
// results in variables it resets on entry, since it can run more than once,
// and must not use the transaction after it returns. A store that cannot
// refresh or report staleness, such as an in-memory one, runs fn once.
func readStable(ctx context.Context, src kvtx.Store, fn func(tx kvtx.Tx) error) error {
	var last error
	for range maxReadAttempts {
		// Follow the writer before taking the snapshot.
		if r, ok := src.(refresher); ok {
			if err := r.Refresh(ctx); err != nil {
				last = errors.Wrap(err, "refresh")
				continue
			}
		}

		// Read the snapshot and check that the writer did not pass it.
		stale, err := readOnce(ctx, src, fn)
		if err == nil && !stale {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			last = err
		} else {
			last = errors.New("the writer passed the snapshot")
		}
	}
	return errors.Wrapf(last, "read did not settle in %d attempts", maxReadAttempts)
}

// readOnce runs fn on one snapshot, turning a panic into an error, and reports
// whether the snapshot is stale afterwards.
func readOnce(ctx context.Context, src kvtx.Store, fn func(tx kvtx.Tx) error) (stale bool, err error) {
	// Open the snapshot and turn a panic of a torn read into an error.
	tx, err := src.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("read failed: %v", r)
		}
	}()
	err = fn(tx)
	if st, ok := tx.(staleTx); ok {
		stale = st.Stale()
	}
	return stale, err
}
