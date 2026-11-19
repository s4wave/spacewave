package space_world

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// replayStage is the store a serving adapter's replay engine writes through.
// Reads serve the kept pages. Puts are kept as the PendingStore keeps them,
// and those past its bound go to the stage of the current save epoch, which
// runs from one cursor save's start to the next. A replayed transaction's
// early drains go there too: OpenStage returns the replay stage itself, so a
// drained page waits for the same save as the World that names it.
//
// The replay engine's caller holds its commits: no root the adapter owns
// reaches a replayed World until a cursor save holds it, and a re-put of a
// page the Volume still stores only restarts the page's grace. So:
//
// Invariant: every page the replay stores is held by its epoch's stage from
// the put until a cursor save that started after the put lands, whose root
// then holds the World the save names. A save that will end an epoch that
// stored a page moves the replay's floor to the World it names as it starts,
// so the Worlds before that one, which may name a page only the epoch held,
// are computed again from the base before replay runs an operation on one or
// a checkpoint takes one.
//
// Only the driver goroutine puts and rotates, and a replayed transaction
// runs within one operation, so the epoch does not change under it. A save's
// goroutine returns the epochs of a failed save through keep before it ends
// the save, so the driver's next rotate, which runs only once the save
// ended, takes them.
type replayStage struct {
	// PendingStore serves reads.
	*block.PendingStore
	// epoch takes the puts of the current save epoch.
	epoch *block.OverflowStage
	// unlanded are earlier epochs whose save failed.
	unlanded []*block.OverflowStage
}

// newReplayStage returns a replay stage over pages at its first epoch.
func newReplayStage(pages *block.PendingStore) *replayStage {
	return &replayStage{PendingStore: pages, epoch: pages.OpenOverflowStage()}
}

// PutBlock puts a block through the current epoch. A ref or hash type that
// does not validate fails as the content's; any other failure is this
// device's store's, as storeError says.
func (s *replayStage) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if err := opts.GetForceBlockRef().Validate(true); err != nil {
		return nil, false, err
	}
	if err := opts.SelectHashType(0).Validate(); err != nil {
		return nil, false, err
	}
	ref, existed, err := s.epoch.PutBlock(ctx, data, opts)
	return ref, existed, localStoreError(err)
}

// PutBlockBatch puts entries through the current epoch. A ref that does not
// validate fails as the content's; any other failure is this device's
// store's, as storeError says.
func (s *replayStage) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	for _, e := range entries {
		if err := e.Ref.Validate(false); err != nil {
			return nil, err
		}
	}
	existed, err := s.epoch.PutBlockBatch(ctx, entries)
	return existed, localStoreError(err)
}

// GetBlock reads a kept page or a stored block. A ref that does not validate
// fails as the content's; any other failure, as against a block that is not
// found, is this device's store's, as storeError says.
func (s *replayStage) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if err := ref.Validate(false); err != nil {
		return nil, false, err
	}
	data, found, err := s.PendingStore.GetBlock(ctx, ref)
	if err != nil {
		return nil, false, &storeError{err: err}
	}
	return data, found, nil
}

// storeError is a failure of the store a replay reads and writes, such as a
// full disk or a read error. It belongs to this device and not to the
// operation replay applies, so replay waits at the operation, a replay gap,
// instead of refusing it on this replica alone.
//
// A failure the content decides is not one: every replica meets it alike,
// so replay refuses the operation instead of waiting at it forever. The
// stage checks refs and hash types before the store sees them, and
// localStoreError passes on the two failures the store decides from the
// content, a tombstone a pending store refuses and bytes that do not hash to
// their ref. A stored block whose bytes do not hash to its ref is the store's
// corruption, a storeError.
type storeError struct {
	err error
}

// localStoreError wraps a non-nil put err as a storeError, unless the
// content put decides it.
func localStoreError(err error) error {
	if err == nil || errors.Is(err, block.ErrPendingTombstone) || errors.Is(err, block.ErrBlockRefMismatch) {
		return err
	}
	return &storeError{err: err}
}

// Error returns the store's failure.
func (e *storeError) Error() string {
	return e.err.Error()
}

// Unwrap returns the store's failure.
func (e *storeError) Unwrap() error {
	return e.err
}

// BeginReadOperation returns the store itself, so a scoped read still serves
// the kept pages.
func (s *replayStage) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// OpenStage returns the store itself with nothing to release: a write
// transaction's puts, its early drains among them, go to the current epoch,
// which holds them until the save of a World after them lands.
func (s *replayStage) OpenStage(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// ReleaseRoots releases nothing: the epoch is shared by every World of the
// replay, so a page one transaction left unreached may be one another World
// reaches.
func (s *replayStage) ReleaseRoots(context.Context, []*block.BlockRef) error {
	return nil
}

// rotate opens the next epoch, for the positions after a save that starts
// now, and returns the current epoch and the unlanded ones, which the save
// ends once it lands, and whether any of them stored a page. A nil stage
// returns none.
func (s *replayStage) rotate() ([]*block.OverflowStage, bool) {
	// A transient replay has no epochs.
	if s == nil {
		return nil, false
	}

	// Hand the save every earlier epoch, and put through a new one.
	epochs := append(s.unlanded, s.epoch)
	s.unlanded, s.epoch = nil, s.OpenOverflowStage()
	stored := false
	for _, e := range epochs {
		stored = stored || e.Stored()
	}
	return epochs, stored
}

// keep returns the epochs of a save that did not land, for the next save.
func (s *replayStage) keep(epochs []*block.OverflowStage) {
	if s != nil {
		s.unlanded = append(s.unlanded, epochs...)
	}
}

// end ends every epoch once the run ended and no save runs. A later put
// through the stage fails, which stops replay rather than storing pages
// nothing holds.
func (s *replayStage) end() {
	if s == nil {
		return
	}
	endEpochs(s.unlanded)
	s.epoch.Release()
	s.unlanded = nil
}

// endEpochs ends each epoch.
func endEpochs(epochs []*block.OverflowStage) {
	for _, e := range epochs {
		e.Release()
	}
}

// _ is a type assertion
var (
	_ block.StoreOps    = (*replayStage)(nil)
	_ block.LeasedRoots = (*replayStage)(nil)
)
