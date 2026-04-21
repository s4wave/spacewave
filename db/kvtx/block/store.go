package kvtx_block

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// CommitFn is a function to call with the updated root before confirming it.
// Should be used to write the updated state back to storage.
// Note: store rmtx is locked while cb is called, do not block or call engine funcs!
// If an error is returned the change will be rolled back.
// Do not change the nref during this call.
type CommitFn func(nref *bucket.ObjectRef) error

// Store is a block graph backed kvtx store.
type Store struct {
	// le is the logger
	le *logrus.Entry
	// wmtx ensures only one write transaction is active at a time
	wmtx *semaphore.Weighted
	// rmtx locks the read-only world instance field & root field & read/writeTx
	rmtx sync.RWMutex
	// baseRoot is the base root cursor to use.
	// the root cursor is derived with FollowRef from this cursor.
	baseRoot *bucket_lookup.Cursor
	// root is the root cursor in use
	root *bucket_lookup.Cursor
	// readTx is the current read-only world instance
	readTx kvtx.BlockTx
	// writeTx is the current write tx
	// canceled if the state changes mid-write
	// note: may not be set to nil when canceled
	writeTx *storeTx
	// commitFn is a function to be called just before a commit is confirmed.
	// can be nil
	commitFn CommitFn
}

// NewStore constructs a new store with a root cursor.
func NewStore(
	ctx context.Context,
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	commitFn CommitFn,
) (*Store, error) {
	st := &Store{
		le:       le,
		baseRoot: root,
		root:     root.Clone(),
		commitFn: commitFn,

		wmtx: semaphore.NewWeighted(1),
	}
	if err := st.updateReadWriteTxns(ctx); err != nil {
		return nil, err
	}
	return st, nil
}

// GetRootRef gets the current root cursor reference.
func (s *Store) GetRootRef() *bucket.ObjectRef {
	s.rmtx.RLock()
	ref := s.root.GetRef().Clone()
	s.rmtx.RUnlock()
	return ref
}

// SetRootRef updates the root cursor to point to a new reference.
// Re-creates the internal read transaction with the updated state.
// Cancels any ongoing write tx (to be re-created against new state).
// Can return an error to indicate validation failure.
func (s *Store) SetRootRef(ctx context.Context, ref *bucket.ObjectRef) error {
	s.rmtx.Lock()
	defer s.rmtx.Unlock()

	return s.setRootRefLocked(ctx, ref)
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return s.NewKvtxBlockTransaction(ctx, write)
}

// NewKvtxBlockTransaction returns a new kvtx block transaction.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (s *Store) NewKvtxBlockTransaction(ctx context.Context, write bool) (kvtx.BlockTx, error) {
	// writeTx is nil if it's a read-only tx
	if !write {
		s.rmtx.Lock()
		defer s.rmtx.Unlock()
		return s.newStoreTx(nil, nil), nil
	}

	// Released in Discard or Commit
	if err := s.wmtx.Acquire(ctx, 1); err != nil {
		return nil, err
	}

	s.rmtx.Lock()
	defer s.rmtx.Unlock()

	writeTx, writeBtx, err := s.buildBlockTx(ctx, true)
	if err != nil {
		s.wmtx.Release(1)
		return nil, err
	}

	storeTx := s.newStoreTx(writeTx, writeBtx)
	s.writeTx = storeTx
	return storeTx, nil
}

// setRootRefLocked updates the root reference while rmtx is locked.
func (s *Store) setRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) error {
	// if no changes, ignore the call
	if s.root.GetRef().EqualsRef(ref) {
		return nil
	}

	// validate the new root
	if err := ref.Validate(); err != nil {
		return err
	}

	// apply committed changes or rollback
	oldRoot := s.root
	nextRoot, err := s.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return err
	}
	s.root = nextRoot
	err = s.updateReadWriteTxns(ctx)
	if err == nil {
		oldRoot.Release()
	} else {
		s.root = oldRoot
		nextRoot.Release()
	}
	return err
}

// buildBlockTx builds a new kvtx block transaction.
// expects caller to hold rmtx
func (s *Store) buildBlockTx(ctx context.Context, write bool) (kvtx.BlockTx, *block.Transaction, error) {
	btx, bcs := s.root.BuildTransaction(nil)
	if !write {
		btx = nil
	}
	mtx, err := BuildKvTransaction(ctx, bcs, write)
	if err != nil {
		return nil, nil, err
	}
	return mtx, btx, nil
}

// updateReadWriteTxns updates the readTx and cancels writeTx if the state changed
// expects caller to hold rmtx lock
// the state has been affected only if nil is returned
func (s *Store) updateReadWriteTxns(ctx context.Context) error {
	// If no changes have occurred...
	if s.readTx != nil &&
		s.readTx.GetCursor().GetRef().EqualsRef(s.root.GetRef().GetRootRef()) {
		return nil
	}

	readTx, _, err := s.buildBlockTx(ctx, false)
	if err != nil {
		return err
	}
	// cancel the old write tx if active
	if s.writeTx != nil {
		s.writeTx.Discard()
		s.writeTx = nil // field is checked during Commit() as well
	}
	// swap in the new read tx
	if s.readTx != nil {
		s.readTx.Discard()
	}
	s.readTx = readTx
	return nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Store)(nil))
