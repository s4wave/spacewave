package provider_spacewave

import (
	"context"
	"errors"
	"os"

	"github.com/s4wave/spacewave/core/sobject"
)

// CloudWriteBarrierEnv turns off the cloud host's write barrier when set to
// "off" at startup.
//
// The barrier keeps one mount's writes of the verified state cache from
// landing after the next mount loaded it. Its invariants:
//
//   - Every write of the verified state cache by a cloudSOHost runs under its
//     acceptMu, taken through lockAccept. The cache writer's record write is a
//     plain set, so this lock is the only thing ordering a host's writes.
//   - A closed host writes nothing: lockAccept refuses it after close, so no
//     persist or publication registration starts.
//   - The tracker run calls close before it returns, and close waits for the
//     acceptMu holder in progress. When the run returns, each of the host's
//     writes has committed or will never start. The keyed routine starts the
//     next run only after this one has returned, so the next mount's load
//     reads every write the old mount made.
//   - close runs before the run detaches the host from the sync controller, so
//     a closed host never registers its publication again.
//   - A failed persist in a local write may have committed. The write then
//     returns sobject.ErrStateWriteUnknown and ends the host, so the next mount
//     settles it from what the cache holds.
//
// With the barrier off, a host keeps writing after its run ended and returns
// a persist failure as a plain error, as before the barrier.
const CloudWriteBarrierEnv = "SPACEWAVE_CLOUD_WRITE_BARRIER"

// cloudWriteBarrierOff selects CloudWriteBarrierEnv's old behavior. It is read
// once, here, at startup.
var cloudWriteBarrierOff = os.Getenv(CloudWriteBarrierEnv) == "off"

// errHostClosed is returned for a write to a cloud host whose tracker run
// ended. The write stored nothing.
var errHostClosed = errors.New("cloud shared object host closed")

// lockAccept takes acceptMu for a write of the host's accepted state. It
// returns errHostClosed, holding nothing, once the host is closed.
func (h *cloudSOHost) lockAccept(ctx context.Context) (func(), error) {
	// Serialize with the host's other acceptance and persistence.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return nil, err
	}

	// Refuse a write once the host's run has ended.
	if h.closed {
		release()
		return nil, errHostClosed
	}
	return release, nil
}

// close waits for the write that holds acceptMu, then refuses every later
// write. The tracker run calls it before it returns.
func (h *cloudSOHost) close() {
	// Keep the old behavior when the barrier is off.
	if cloudWriteBarrierOff {
		return
	}

	// Mark the host closed once its current writer finishes.
	release, _ := h.acceptMu.Lock(context.Background())
	h.closed = true
	release()
}

// persistUnknown reports a failed persist of a local write. A commit error
// can follow a stored write, so the write's outcome is unknown: the host ends,
// and the next mount settles the write from the cache. With the barrier off
// it returns err.
func (h *cloudSOHost) persistUnknown(err error) error {
	// Keep the old behavior when the barrier is off.
	if cloudWriteBarrierOff {
		return err
	}

	// End the host so the next mount reads what the persist left.
	if h.ctxCancel != nil {
		h.ctxCancel()
	}
	return errors.Join(sobject.ErrStateWriteUnknown, err)
}
