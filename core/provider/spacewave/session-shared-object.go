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
		facade, err := c.account.newSessionSharedObject(c.sessionID, ref, baseValue)
		if err != nil {
			baseRelease()
			return nil, nil, err
		}
		return facade, baseRelease, nil
	}), nil)
}

func (c *sessionSharedObjectMountController) Close() error { return nil }

var _ controller.Controller = (*sessionSharedObjectMountController)(nil)

func (a *ProviderAccount) newSessionSharedObject(
	sessionID string,
	ref *sobject.SharedObjectRef,
	baseValue sobject.MountSharedObjectValue,
) (*sessionSharedObject, error) {
	base, ok := baseValue.(*SharedObject)
	if !ok || base == nil {
		return nil, errors.Errorf("unexpected account shared object type %T", baseValue)
	}
	bucketID := BlockStoreBucketID(a.accountID, ref.GetBlockStoreId())
	direct := func() block.StoreOps {
		store := a.getSessionDEXStore(sessionID, bucketID)
		if store == nil {
			return nil
		}
		return &sourceTrackingStore{
			StoreOps:       store,
			account:        a,
			bstoreID:       ref.GetBlockStoreId(),
			source:         SyncTelemetryBlockSourceDirect,
			demandStarted:  func() { a.directDemandStarted(sessionID) },
			demandFinished: func() { a.directDemandFinished(sessionID) },
		}
	}
	facadeStore, err := base.newSessionBlockStore(direct)
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
	direct block_store.StoreSource,
) (bstore.BlockStore, error) {
	baseStore, ok := s.blkStore.(*BlockStore)
	if !ok || baseStore == nil {
		return nil, errors.Errorf("unexpected shared object block store type %T", s.blkStore)
	}
	return baseStore.newSessionBlockStore(direct)
}

func (b *BlockStore) newSessionBlockStore(
	direct block_store.StoreSource,
) (bstore.BlockStore, error) {
	if b == nil || b.cacheStore == nil || b.cloudStore == nil {
		return nil, errors.New("account block store read owners are unavailable")
	}
	remote := block_store.NewStoreReadThrough(direct, func() block.StoreOps {
		return b.cloudStore
	}, false)
	readStore := block_store.NewStoreReadThrough(func() block.StoreOps {
		return b.cacheStore
	}, func() block.StoreOps {
		return remote
	}, true)
	return &sessionBlockStore{
		BlockStore: b,
		readStore:  block_store.NewStore(b.GetID(), readStore),
	}, nil
}

// sessionBlockStore keeps writes and durability on the account-owned store
// while routing reads through the Session cache, child DEX, and Cloud stores.
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

var (
	_ block.StoreOps    = (*block_store.StoreReadThrough)(nil)
	_ block_store.Store = (*sessionBlockStore)(nil)
)

var _ block_store.Store = (*sessionBlockStore)(nil)
