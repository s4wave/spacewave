package world

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/refcount"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// WorldAccess is the borrowed access value passed to WorldStorage callbacks.
// Its cursor and transaction pair are valid only for the callback duration.
type WorldAccess struct {
	cursor  *bucket_lookup.Cursor
	storage *cursorWorldStorage
	btx     *block.Transaction
	bcs     *block.Cursor
}

// Cursor returns the borrowed lookup cursor.
func (a *WorldAccess) Cursor() *bucket_lookup.Cursor {
	if a == nil {
		return nil
	}
	return a.cursor
}

// BuildTransaction builds the owner-selected block transaction and cursor.
func (a *WorldAccess) BuildTransaction(putOpts *block.PutOpts) (*block.Transaction, *block.Cursor) {
	raw := a.Cursor()
	if raw == nil {
		return nil, nil
	}
	if a.btx == nil {
		a.btx, a.bcs = raw.BuildTransactionAtRefWithStore(
			putOpts,
			raw.GetRef().GetRootRef(),
			a.storage.storeFor(raw),
		)
	}
	return a.btx, a.bcs
}

// BuildTransactionAtRef builds an owner-selected block transaction at ref.
func (a *WorldAccess) BuildTransactionAtRef(putOpts *block.PutOpts, ref *block.BlockRef) (*block.Transaction, *block.Cursor) {
	raw := a.Cursor()
	if raw == nil {
		return nil, nil
	}
	rootRef := raw.GetRef().GetRootRef()
	if (ref.GetEmpty() && rootRef.GetEmpty()) || (ref != nil && ref.EqualsRef(rootRef)) {
		return a.BuildTransaction(putOpts)
	}
	return raw.BuildTransactionAtRefWithStore(putOpts, ref, a.storage.storeFor(raw))
}

type lookupAuthority struct {
	refs *refcount.RefCount[*bucket_lookup.Cursor]
}

func newLookupAuthority(raw *bucket_lookup.Cursor) (*lookupAuthority, refcount.RefLike, error) {
	return newLookupAuthorityWithRelease(raw, raw.Release)
}

func newLookupAuthorityWithRelease(
	raw *bucket_lookup.Cursor,
	release func(),
) (*lookupAuthority, refcount.RefLike, error) {
	refs := refcount.NewRefCount(
		context.Background(),
		false,
		nil,
		nil,
		func(context.Context, func()) (*bucket_lookup.Cursor, func(), error) {
			return raw, release, nil
		},
	)
	_, lease, err := refs.Wait(context.Background())
	if err != nil {
		return nil, nil, err
	}
	return &lookupAuthority{refs: refs}, lease, nil
}

func (a *lookupAuthority) retain(ctx context.Context) (refcount.RefLike, error) {
	if a == nil || a.refs == nil {
		return nil, ErrWorldStorageUnavailable
	}
	_, lease, err := a.refs.Wait(ctx)
	if err != nil {
		return nil, err
	}
	return lease, nil
}

// OwnedLookupCursor owns a lookup cursor beyond a borrowed access callback.
type OwnedLookupCursor struct {
	raw   *bucket_lookup.Cursor
	auth  *lookupAuthority
	lease refcount.RefLike
	once  sync.Once
}

func newOwnedLookupCursor(
	raw *bucket_lookup.Cursor,
	auth *lookupAuthority,
	lease refcount.RefLike,
) *OwnedLookupCursor {
	return &OwnedLookupCursor{raw: raw, auth: auth, lease: lease}
}

// Clone returns an independently owned cursor at the same reference.
func (c *OwnedLookupCursor) Clone() (*OwnedLookupCursor, error) {
	if c == nil || c.raw == nil || c.auth == nil {
		return nil, ErrWorldStorageUnavailable
	}
	lease, err := c.auth.retain(context.Background())
	if err != nil {
		return nil, err
	}
	return newOwnedLookupCursor(c.raw.Clone(), c.auth, lease), nil
}

// Cursor returns the borrowed raw cursor.
func (c *OwnedLookupCursor) Cursor() *bucket_lookup.Cursor {
	if c == nil {
		return nil
	}
	return c.raw
}

// FollowRef follows ref and returns an independently owned cursor.
func (c *OwnedLookupCursor) FollowRef(ctx context.Context, ref *bucket.ObjectRef) (*OwnedLookupCursor, error) {
	if c == nil || c.raw == nil || c.auth == nil {
		return nil, ErrWorldStorageUnavailable
	}
	parentLease, err := c.auth.retain(ctx)
	if err != nil {
		return nil, err
	}
	followed, err := c.raw.FollowRef(ctx, ref)
	if err != nil {
		parentLease.Release()
		return nil, err
	}
	auth, lease, err := newLookupAuthorityWithRelease(followed, func() {
		followed.Release()
		parentLease.Release()
	})
	if err != nil {
		followed.Release()
		parentLease.Release()
		return nil, err
	}
	return newOwnedLookupCursor(followed.Clone(), auth, lease), nil
}

// Release releases the owned cursor. It is idempotent.
func (c *OwnedLookupCursor) Release() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		if c.raw != nil {
			c.raw.Release()
		}
		if c.lease != nil {
			c.lease.Release()
		}
		c.raw = nil
		c.auth = nil
		c.lease = nil
	})
}

type cursorWorldStorage struct {
	buildCursorFn BuildCursorFn
	base          *OwnedLookupCursor
	store         block.StoreOps
	bucketID      string
	mtx           sync.Mutex
	retired       atomic.Bool
	retireOnce    sync.Once
}

func (s *cursorWorldStorage) storeFor(cursor *bucket_lookup.Cursor) block.StoreOps {
	if cursor != nil && cursor.GetOpArgs().GetBucketId() == s.bucketID && s.store != nil {
		return s.store
	}
	if cursor == nil {
		return s.store
	}
	return cursor.GetBucket()
}

func (s *cursorWorldStorage) buildOwned(ctx context.Context, ref *bucket.ObjectRef) (*OwnedLookupCursor, error) {
	if s == nil {
		return nil, ErrWorldStorageUnavailable
	}
	s.mtx.Lock()
	if s.retired.Load() {
		s.mtx.Unlock()
		return nil, ErrWorldStorageUnavailable
	}
	if s.base != nil {
		owned, err := s.base.Clone()
		s.mtx.Unlock()
		if err != nil {
			return nil, err
		}
		if ref != nil && !owned.Cursor().GetRef().EqualsRef(ref) {
			followed, err := owned.FollowRef(ctx, ref)
			owned.Release()
			if err != nil {
				return nil, err
			}
			owned = followed
		}
		return owned, nil
	}
	buildCursorFn := s.buildCursorFn
	s.mtx.Unlock()
	if buildCursorFn == nil {
		return nil, ErrWorldStorageUnavailable
	}
	raw, err := buildCursorFn(ctx)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, ErrWorldStorageUnavailable
	}
	s.mtx.Lock()
	if s.retired.Load() {
		s.mtx.Unlock()
		raw.Release()
		return nil, ErrWorldStorageUnavailable
	}
	auth, lease, err := newLookupAuthority(raw)
	s.mtx.Unlock()
	if err != nil {
		raw.Release()
		return nil, err
	}
	owned := newOwnedLookupCursor(raw.Clone(), auth, lease)
	if ref != nil && !owned.Cursor().GetRef().EqualsRef(ref) {
		followed, err := owned.FollowRef(ctx, ref)
		owned.Release()
		if err != nil {
			return nil, err
		}
		owned = followed
	}
	return owned, nil
}

func (s *cursorWorldStorage) buildRaw(ctx context.Context) (*bucket_lookup.Cursor, error) {
	if s == nil {
		return nil, ErrWorldStorageUnavailable
	}
	s.mtx.Lock()
	if s.retired.Load() {
		s.mtx.Unlock()
		return nil, ErrWorldStorageUnavailable
	}
	if s.base != nil {
		raw := s.base.Cursor().Clone()
		raw.SetRootRef(nil)
		s.mtx.Unlock()
		return raw, nil
	}
	buildCursorFn := s.buildCursorFn
	s.mtx.Unlock()
	if buildCursorFn == nil {
		return nil, ErrWorldStorageUnavailable
	}
	raw, err := buildCursorFn(ctx)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, ErrWorldStorageUnavailable
	}
	s.mtx.Lock()
	retired := s.retired.Load()
	s.mtx.Unlock()
	if retired {
		raw.Release()
		return nil, ErrWorldStorageUnavailable
	}
	raw.SetRootRef(nil)
	return raw, nil
}

func (s *cursorWorldStorage) access(ctx context.Context, ref *bucket.ObjectRef, cb func(*WorldAccess) error) (err error) {
	owned, err := s.buildOwned(ctx, ref)
	if err != nil {
		return err
	}
	defer func() {
		owned.Release()
		if recovered := recover(); recovered != nil {
			panic(recovered)
		}
	}()
	return cb(&WorldAccess{
		cursor:  owned.Cursor(),
		storage: s,
	})
}

func (s *cursorWorldStorage) retire() {
	if s == nil {
		return
	}
	s.retireOnce.Do(func() {
		s.mtx.Lock()
		s.retired.Store(true)
		if s.base != nil {
			s.base.Release()
			s.base = nil
		}
		s.mtx.Unlock()
	})
}

// NewWorldStorageFromOwnedLookupCursor transfers owned to a transaction-scoped storage owner.
func NewWorldStorageFromOwnedLookupCursor(owned *OwnedLookupCursor, store block.StoreOps) WorldStorage {
	if owned == nil {
		return nil
	}
	return &cursorWorldStorage{
		base:     owned,
		store:    store,
		bucketID: owned.Cursor().GetOpArgs().GetBucketId(),
	}
}

// NewWorldStorageFromCursorWithStore transfers cursor to a storage owner.
func NewWorldStorageFromCursorWithStore(cursor *bucket_lookup.Cursor, store block.StoreOps) WorldStorage {
	if cursor == nil {
		return nil
	}
	auth, lease, err := newLookupAuthority(cursor)
	if err != nil {
		cursor.Release()
		return nil
	}
	owned := newOwnedLookupCursor(cursor.Clone(), auth, lease)
	return &cursorWorldStorage{
		base:     owned,
		store:    store,
		bucketID: cursor.GetOpArgs().GetBucketId(),
	}
}

// RetireWorldStorage retires a storage owner and releases its owner lease.
func RetireWorldStorage(storage WorldStorage) {
	if owner, ok := storage.(*cursorWorldStorage); ok {
		owner.retire()
	}
}
