package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// sessionSharedObject is a shallow per-Session facade over one account-owned
// SharedObject. Reads use the Session child bus; state and identity remain on
// the account-owned object.
type sessionSharedObject struct {
	*SharedObject
	account      *ProviderAccount
	sessionID    string
	sessionStore bstore.BlockStore
}

func (s *sessionSharedObject) GetBus() bus.Bus {
	return s.account.getSessionBusForSession(s.sessionID)
}

func (s *sessionSharedObject) GetBlockStore() bstore.BlockStore { return s.sessionStore }

// sessionSharedObjectMountController owns MountSharedObject only on one
// Session child bus. The transport bridge excludes this exact directive, so a
// child request cannot also resolve the account's generic parent value.
type sessionSharedObjectMountController struct {
	account   *ProviderAccount
	sessionID string
}

func (c *sessionSharedObjectMountController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"spacewave/session-shared-object-mount/"+c.sessionID,
		controller.MustParseVersion("0.0.1"),
		"session-scoped shared object mount facade",
	)
}

func (c *sessionSharedObjectMountController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *sessionSharedObjectMountController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(sobject.MountSharedObject)
	if !ok {
		return nil, nil
	}
	ref := dir.MountSharedObjectRef()
	if ref == nil || ref.GetProviderResourceRef() == nil {
		return nil, nil
	}
	providerRef := ref.GetProviderResourceRef()
	if providerRef.GetProviderAccountId() != c.account.accountID ||
		providerRef.GetProviderId() != c.account.GetProviderID() {
		return nil, nil
	}
	return directive.R(directive.NewAccessResolver(func(
		ctx context.Context,
		released func(),
	) (sobject.MountSharedObjectValue, func(), error) {
		baseValue, baseRelease, err := c.account.MountSharedObject(ctx, ref, released)
		if err != nil {
			return nil, nil, err
		}
		facade, err := c.account.newSessionSharedObject(ctx, c.sessionID, ref, baseValue)
		if err != nil {
			baseRelease()
			return nil, nil, err
		}
		return facade, baseRelease, nil
	}), nil)
}

func (c *sessionSharedObjectMountController) Close() error { return nil }

var _ controller.Controller = ((*sessionSharedObjectMountController)(nil))

func (a *ProviderAccount) newSessionSharedObject(
	ctx context.Context,
	sessionID string,
	ref *sobject.SharedObjectRef,
	baseValue sobject.MountSharedObjectValue,
) (*sessionSharedObject, error) {
	base, ok := baseValue.(*SharedObject)
	if !ok || base == nil {
		return nil, errors.Errorf("unexpected account shared object type %T", baseValue)
	}
	direct := &sessionDirectLookupStore{
		busForSession:  func() bus.Bus { return a.getSessionChildBusForSession(sessionID) },
		bucketID:       BlockStoreBucketID(a.accountID, ref.GetBlockStoreId()),
		hashType:       base.blkStore.GetHashType(),
		account:        a,
		bstoreID:       ref.GetBlockStoreId(),
		demandStarted:  func() { a.directDemandStarted(sessionID) },
		demandFinished: func() { a.directDemandFinished(sessionID) },
	}
	facadeStore, err := base.newSessionBlockStore(ctx, a.le, direct)
	if err != nil {
		return nil, err
	}
	return &sessionSharedObject{
		SharedObject: base,
		account:      a,
		sessionID:    sessionID,
		sessionStore: facadeStore,
	}, nil
}

func (s *SharedObject) newSessionBlockStore(
	ctx context.Context,
	le *logrus.Entry,
	direct *sessionDirectLookupStore,
) (bstore.BlockStore, error) {
	baseStore, ok := s.blkStore.(*BlockStore)
	if !ok || baseStore == nil {
		return nil, errors.Errorf("unexpected shared object block store type %T", s.blkStore)
	}
	return baseStore.newSessionBlockStore(ctx, le, direct)
}

func (b *BlockStore) newSessionBlockStore(
	ctx context.Context,
	le *logrus.Entry,
	direct *sessionDirectLookupStore,
) (bstore.BlockStore, error) {
	if b == nil || b.cacheStore == nil || b.cloudStore == nil {
		return nil, errors.New("account block store read owners are unavailable")
	}
	readStore := &sessionReadStore{
		cache:  b.cacheStore,
		direct: direct,
		cloud:  b.cloudStore,
		le:     le,
	}
	return &sessionBlockStore{
		BlockStore: b,
		readStore:  block_store.NewStore(b.GetID(), readStore),
	}, nil
}

// sessionBlockStore keeps writes and durability on the account-owned store
// while routing reads through the Session cache, child DEX, and Cloud owners.
type sessionBlockStore struct {
	*BlockStore
	readStore block_store.Store
}

func (s *sessionBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	ops, release, err := s.readStore.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &sessionBlockStore{
		BlockStore: s.BlockStore,
		readStore:  block_store.NewStore(s.GetID(), ops),
	}, release, nil
}

func (s *sessionBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.readStore.GetBlock(ctx, ref)
}

func (s *sessionBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.readStore.GetBlockExists(ctx, ref)
}

func (s *sessionBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return s.readStore.GetBlockExistsBatch(ctx, refs)
}

func (s *sessionBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return s.readStore.StatBlock(ctx, ref)
}

// sessionReadStore is the read-only Session pipeline. A direct hit is written
// to the non-dirty account cache before returning to make the next read local.
type sessionReadStore struct {
	cache  block.StoreOps
	direct block.StoreOps
	cloud  block.StoreOps
	le     *logrus.Entry
}

func (s *sessionReadStore) GetHashType() hash.HashType {
	if hashType := s.cache.GetHashType(); hashType != 0 {
		return hashType
	}
	if hashType := s.direct.GetHashType(); hashType != 0 {
		return hashType
	}
	return s.cloud.GetHashType()
}

func (s *sessionReadStore) GetSupportedFeatures() block.StoreFeature {
	return s.cache.GetSupportedFeatures() & s.cloud.GetSupportedFeatures()
}

func (s *sessionReadStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	cache, releaseCache, err := s.cache.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	direct, releaseDirect, err := s.direct.BeginReadOperation(ctx)
	if err != nil {
		releaseCache()
		return nil, nil, err
	}
	cloud, releaseCloud, err := s.cloud.BeginReadOperation(ctx)
	if err != nil {
		releaseDirect()
		releaseCache()
		return nil, nil, err
	}
	return &sessionReadStore{
			cache:  cache,
			direct: direct,
			cloud:  cloud,
			le:     s.le,
		}, func() {
			releaseCloud()
			releaseDirect()
			releaseCache()
		}, nil
}

func (s *sessionReadStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

func (s *sessionReadStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return block_store.ErrReadOnly
}

func (s *sessionReadStore) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}

func (s *sessionReadStore) Sync(context.Context) (bool, error) { return true, nil }

func (s *sessionReadStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.cache.GetBlock(ctx, ref)
	if err != nil || found {
		return data, found, err
	}
	data, found, err = s.direct.GetBlock(ctx, ref)
	if err != nil || !found {
		if err != nil {
			return nil, false, err
		}
		return s.cloud.GetBlock(ctx, ref)
	}
	s.le.Debug("writing direct block through to session cache")
	if _, _, cacheErr := s.cache.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref.Clone()}); cacheErr != nil {
		return nil, false, cacheErr
	}
	return data, true, nil
}

func (s *sessionReadStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

func (s *sessionReadStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

func (s *sessionReadStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

var _ block.StoreOps = ((*sessionReadStore)(nil))
var _ block_store.Store = ((*sessionBlockStore)(nil))

// sessionDirectLookupStore resolves only the direct DEX path. The account
// BlockStore remains the overlay's lower Cloud/local fallback, so one miss
// cannot re-enter the direct path or fan out to another Session.
type sessionDirectLookupStore struct {
	bus            bus.Bus
	busForSession  func() bus.Bus
	bucketID       string
	hashType       hash.HashType
	account        *ProviderAccount
	bstoreID       string
	demandStarted  func()
	demandFinished func()
}

func (s *sessionDirectLookupStore) GetHashType() hash.HashType               { return s.hashType }
func (s *sessionDirectLookupStore) GetSupportedFeatures() block.StoreFeature { return 0 }
func (s *sessionDirectLookupStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}
func (s *sessionDirectLookupStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}
func (s *sessionDirectLookupStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return block_store.ErrReadOnly
}
func (s *sessionDirectLookupStore) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}
func (s *sessionDirectLookupStore) Sync(context.Context) (bool, error) { return true, nil }

func (s *sessionDirectLookupStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	childBus := s.bus
	if s.busForSession != nil {
		childBus = s.busForSession()
	}
	if childBus == nil {
		return nil, false, nil
	}
	if s.demandStarted != nil {
		s.demandStarted()
		defer s.demandFinished()
	}
	val, _, valRef, err := bus.ExecWaitValue[dex.LookupBlockFromNetworkValue](ctx, childBus, dex.NewLookupBlockFromNetwork(s.bucketID, ref), bus.ReturnWhenIdle(), nil,
		func(value dex.LookupBlockFromNetworkValue) (bool, error) {
			if value.GetError() != nil && value.GetError() != block.ErrNotFound {
				return true, value.GetError()
			}
			return true, nil
		})
	if valRef != nil {
		valRef.Release()
	}
	if err != nil || val == nil {
		return nil, false, err
	}
	if val.GetError() != nil {
		if val.GetError() == block.ErrNotFound {
			return nil, false, nil
		}
		return nil, false, val.GetError()
	}
	data := val.GetData()
	found := len(data) != 0
	if found && s.account != nil {
		s.account.recordSyncTelemetryBlockSource(s.bstoreID, SyncTelemetryBlockSourceDirect)
	}
	return data, found, nil
}

func (s *sessionDirectLookupStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

func (s *sessionDirectLookupStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

func (s *sessionDirectLookupStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

var _ block.StoreOps = ((*sessionDirectLookupStore)(nil))
