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

// GetBus returns the Session child bus that serves this facade's reads.
func (s *sessionSharedObject) GetBus() bus.Bus {
	return s.account.getSessionBusForSession(s.sessionID)
}

// GetBlockStore returns the Session read view over the account block store.
func (s *sessionSharedObject) GetBlockStore() bstore.BlockStore { return s.sessionStore }

// sessionSharedObjectMountController owns MountSharedObject only on one
// Session child bus. The transport bridge excludes this exact directive, so a
// child request cannot also resolve the account's generic parent value.
type sessionSharedObjectMountController struct {
	account   *ProviderAccount
	sessionID string
}

// GetControllerInfo returns the controller info scoped to the Session.
func (c *sessionSharedObjectMountController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"spacewave/session-shared-object-mount/"+c.sessionID,
		controller.MustParseVersion("0.0.1"),
		"session-scoped shared object mount facade",
	)
}

// Execute holds the controller until ctx ends; it only resolves directives.
func (c *sessionSharedObjectMountController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// HandleDirective resolves MountSharedObject for this account's objects with
// the Session facade.
func (c *sessionSharedObjectMountController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	// Accept only MountSharedObject for this provider account.
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
		// Mount the account object and wrap it in the Session facade.
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

// Close releases nothing: the controller holds no resources.
func (c *sessionSharedObjectMountController) Close() error { return nil }

func (a *ProviderAccount) newSessionSharedObject(
	sessionID string,
	ref *sobject.SharedObjectRef,
	baseValue sobject.MountSharedObjectValue,
) (*sessionSharedObject, error) {
	// Require the account-owned object, then compose its Session read view.
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
	remote := block_store.NewStoreReadThrough(func() block.StoreOps {
		dex := direct()
		if dex == nil {
			return nil
		}
		return &sessionDirectStore{StoreOps: dex, cloud: b.cloudStore}
	}, func() block.StoreOps {
		return b.cloudStore
	}, nil)
	readStore := block_store.NewStoreReadThrough(func() block.StoreOps {
		return b.cacheStore
	}, func() block.StoreOps {
		return remote
	}, block_store.CacheFill)
	return &sessionBlockStore{
		BlockStore: b,
		readStore:  block_store.NewStore(b.GetID(), readStore),
	}, nil
}

// sessionDirectStore is a Session's DEX read view that asks Cloud for a block
// no peer could be asked about.
//
// Invariant: a block read never waits for a peer to connect. DEX reads run
// under block.WithoutPeerWait, so with no peer session the solicit store
// reports block.ErrUnavailable at once, or once a peer whose session just
// ended is restored or its replace window ends; a read a departed peer could
// not serve fails the same way. Either way the block is then read from Cloud.
// A Cloud miss returns the DEX error rather than a miss, because no peer
// answered for the block. With a peer session, peers are asked first and a
// miss they confirm falls through to Cloud as before.
type sessionDirectStore struct {
	block.StoreOps
	cloud block.StoreOps
}

// BeginReadOperation scopes both the DEX view and its Cloud fallback.
func (s *sessionDirectStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Scope the DEX view, then its Cloud fallback.
	dex, releaseDex, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	cloud, releaseCloud, err := s.cloud.BeginReadOperation(ctx)
	if err != nil {
		releaseDex()
		return nil, nil, err
	}
	return &sessionDirectStore{StoreOps: dex, cloud: cloud}, func() {
		releaseCloud()
		releaseDex()
	}, nil
}

// GetBlock reads the block as GetStoredBlock does.
func (s *sessionDirectStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock asks the peers without waiting for one, then Cloud when no
// peer could answer.
func (s *sessionDirectStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Ask the connected peers; with none, the read reports ErrUnavailable.
	dexCtx, skipped := block.WithoutPeerWait(ctx)
	stored, err := s.StoreOps.GetStoredBlock(dexCtx, ref)
	if !errors.Is(err, block.ErrUnavailable) || ctx.Err() != nil {
		return stored, err
	}

	// Serve the block from Cloud, or report it unavailable to the caller.
	stored, cloudErr := s.cloud.GetStoredBlock(ctx, ref)
	if cloudErr != nil || stored != nil {
		return stored, cloudErr
	}
	if wait := block.GetPeerWait(ctx); wait != nil && skipped.GetRef() != nil {
		wait.Skip(ref)
	}
	return nil, err
}

// sessionBlockStore keeps writes and durability on the account-owned store
// while routing reads through the Session cache, child DEX, and Cloud stores.
type sessionBlockStore struct {
	*BlockStore
	readStore block_store.Store
}

// BeginReadOperation scopes the Session read pipeline.
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

// GetBlock reads through the Session cache, DEX and Cloud.
func (s *sessionBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.readStore.GetBlock(ctx, ref)
}

// GetStoredBlock forwards to the inner store.
func (s *sessionBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return s.readStore.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks the Session read pipeline.
func (s *sessionBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.readStore.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks the Session read pipeline for each ref.
func (s *sessionBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return s.readStore.GetBlockExistsBatch(ctx, refs)
}

// StatBlock stats a block through the Session read pipeline.
func (s *sessionBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return s.readStore.StatBlock(ctx, ref)
}

// _ is a type assertion
var (
	_ controller.Controller = (*sessionSharedObjectMountController)(nil)
	_ block.StoreOps        = (*sessionDirectStore)(nil)
	_ block_store.Store     = (*sessionBlockStore)(nil)
)
