package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// heldDurableAdmission holds a successful durability receipt until release.
type heldDurableAdmission struct {
	world.Admission
	// waiting closes after the wrapped WaitDurable succeeds.
	waiting chan struct{}
	// release lets WaitDurable return to its committer.
	release <-chan struct{}
}

// WaitDurable holds the committer after durability without delaying head watches.
func (a *heldDurableAdmission) WaitDurable(ctx context.Context) (uint64, error) {
	// Wait for real durability before holding the receipt.
	seqno, err := a.Admission.WaitDurable(ctx)
	if err != nil {
		return seqno, err
	}
	close(a.waiting)

	// Leave the head watch free to report while the committer waits.
	select {
	case <-a.release:
		return seqno, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// _ asserts that the wrapper forwards the admission contract.
var _ world.Admission = (*heldDurableAdmission)(nil)
