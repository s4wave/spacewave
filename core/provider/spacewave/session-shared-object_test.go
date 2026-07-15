package provider_spacewave

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	bus_controller "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/dex"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

type sessionLookupTestController struct {
	values   map[string][]byte
	requests atomic.Int32
}

func (c *sessionLookupTestController) GetControllerInfo() *bus_controller.Info {
	return bus_controller.NewInfo(
		"test/session-lookup",
		bus_controller.MustParseVersion("0.0.1"),
		"session lookup test controller",
	)
}

func (c *sessionLookupTestController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *sessionLookupTestController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(dex.LookupBlockFromNetwork)
	if !ok {
		return nil, nil
	}
	return directive.R(directive.NewAccessResolver(func(
		_ context.Context,
		released func(),
	) (dex.LookupBlockFromNetworkValue, func(), error) {
		c.requests.Add(1)
		data := c.values[dir.LookupBlockFromNetworkRef().MarshalString()]
		return dex.NewLookupBlockFromNetworkValue(data, nil), func() {
			if released != nil {
				released()
			}
		}, nil
	}), nil)
}

func (c *sessionLookupTestController) Close() error { return nil }

var _ bus_controller.Controller = ((*sessionLookupTestController)(nil))

type sessionLookupTestLower struct {
	data     []byte
	blocks   map[string][]byte
	getCalls atomic.Int32
	putCalls atomic.Int32
	writable bool
}

func (s *sessionLookupTestLower) GetHashType() hash.HashType {
	return hash.HashType_HashType_SHA256
}
func (s *sessionLookupTestLower) GetSupportedFeatures() block.StoreFeature { return 0 }
func (s *sessionLookupTestLower) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}
func (s *sessionLookupTestLower) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if !s.writable {
		return nil, false, block_store.ErrReadOnly
	}
	ref := opts.GetForceBlockRef()
	if ref == nil {
		return nil, false, block_store.ErrReadOnly
	}
	if s.blocks == nil {
		s.blocks = make(map[string][]byte)
	}
	s.blocks[ref.MarshalString()] = append([]byte(nil), data...)
	s.putCalls.Add(1)
	return ref, true, nil
}
func (s *sessionLookupTestLower) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return block_store.ErrReadOnly
}
func (s *sessionLookupTestLower) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}
func (s *sessionLookupTestLower) Sync(context.Context) (bool, error) { return true, nil }
func (s *sessionLookupTestLower) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.getCalls.Add(1)
	var data []byte
	if s.blocks != nil {
		data = s.blocks[ref.MarshalString()]
	} else {
		data = s.data
	}
	if len(data) == 0 {
		return nil, false, nil
	}
	return data, true, nil
}
func (s *sessionLookupTestLower) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}
func (s *sessionLookupTestLower) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
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
func (s *sessionLookupTestLower) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

var _ block.StoreOps = ((*sessionLookupTestLower)(nil))

func newSessionLookupTestBus(
	ctx context.Context,
	values map[string][]byte,
) (bus.Bus, *sessionLookupTestController, func(), error) {
	dc := directive_controller.NewController(ctx, logrus.NewEntry(logrus.New()))
	b := bus_inmem.NewBus(dc)
	ctrl := &sessionLookupTestController{values: values}
	release, err := b.AddController(ctx, ctrl, nil)
	return b, ctrl, release, err
}

func newProductionSessionStore(
	ctx context.Context,
	account *ProviderAccount,
	id string,
	childBus bus.Bus,
	cache, cloud *sessionLookupTestLower,
) (*sessionBlockStore, error) {
	cacheOwner := &sourceTrackingStore{
		StoreOps:   cache,
		account:    account,
		bstoreID:   id,
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}
	cloudOwner := &sourceTrackingStore{
		StoreOps: cloud,
		account:  account,
		bstoreID: id,
		source:   SyncTelemetryBlockSourceCloud,
	}
	accountStore := &BlockStore{
		store:      block_store.NewStore(id, cache),
		cacheStore: cacheOwner,
		cloudStore: cloudOwner,
	}
	direct := &sessionDirectLookupStore{
		bus:      childBus,
		bucketID: id,
		hashType: cache.GetHashType(),
		account:  account,
		bstoreID: id,
	}
	store, err := accountStore.newSessionBlockStore(ctx, logrus.NewEntry(logrus.New()), direct)
	if err != nil {
		return nil, err
	}
	return store.(*sessionBlockStore), nil
}

func telemetryStore(t *testing.T, account *ProviderAccount, id string) SyncTelemetryBlockStoreSnapshot {
	t.Helper()
	for _, store := range account.GetSyncTelemetrySnapshot().BlockStores {
		if store.BlockStoreID == id {
			return store
		}
	}
	t.Fatalf("missing telemetry store %q", id)
	return SyncTelemetryBlockStoreSnapshot{}
}

func TestSessionDirectLookupRoutesPerChildAndFallsBackOnce(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	refCache, err := block.BuildBlockRef([]byte("cache"), nil)
	if err != nil {
		t.Fatal(err)
	}
	refDirect, err := block.BuildBlockRef([]byte("direct"), nil)
	if err != nil {
		t.Fatal(err)
	}
	refFallback, err := block.BuildBlockRef([]byte("fallback"), nil)
	if err != nil {
		t.Fatal(err)
	}

	child1, ctrl1, release1, err := newSessionLookupTestBus(ctx, map[string][]byte{
		refDirect.MarshalString(): []byte("session-one"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release1()
	child2, ctrl2, release2, err := newSessionLookupTestBus(ctx, map[string][]byte{
		refDirect.MarshalString(): []byte("session-two"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release2()

	cache1 := &sessionLookupTestLower{
		blocks:   map[string][]byte{refCache.MarshalString(): []byte("cached-one")},
		writable: true,
	}
	cloud1 := &sessionLookupTestLower{
		blocks: map[string][]byte{refFallback.MarshalString(): []byte("cloud-one")},
	}
	cache2 := &sessionLookupTestLower{blocks: map[string][]byte{}, writable: true}
	cloud2 := &sessionLookupTestLower{blocks: map[string][]byte{}}
	account := &ProviderAccount{}
	store1, err := newProductionSessionStore(ctx, account, "session-one", child1, cache1, cloud1)
	if err != nil {
		t.Fatal(err)
	}
	store2, err := newProductionSessionStore(ctx, account, "session-two", child2, cache2, cloud2)
	if err != nil {
		t.Fatal(err)
	}

	data, found, err := store1.GetBlock(ctx, refCache)
	if err != nil || !found || string(data) != "cached-one" {
		t.Fatalf("local cache read = %q/%v/%v", data, found, err)
	}
	if ctrl1.requests.Load() != 0 || cloud1.getCalls.Load() != 0 {
		t.Fatalf("local cache hit touched DEX/Cloud: %d/%d", ctrl1.requests.Load(), cloud1.getCalls.Load())
	}

	data, found, err = store1.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-one" {
		t.Fatalf("session one direct read = %q/%v/%v", data, found, err)
	}
	if ctrl1.requests.Load() != 1 || cloud1.getCalls.Load() != 0 || cache1.putCalls.Load() != 1 {
		t.Fatalf("direct hit calls = dex %d/cloud %d/cache puts %d, want 1/0/1", ctrl1.requests.Load(), cloud1.getCalls.Load(), cache1.putCalls.Load())
	}
	data, found, err = store1.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-one" {
		t.Fatalf("session one cached direct read = %q/%v/%v", data, found, err)
	}
	if ctrl1.requests.Load() != 1 || cloud1.getCalls.Load() != 0 {
		t.Fatalf("cached direct read touched DEX/Cloud: %d/%d", ctrl1.requests.Load(), cloud1.getCalls.Load())
	}

	data, found, err = store1.GetBlock(ctx, refFallback)
	if err != nil || !found || string(data) != "cloud-one" {
		t.Fatalf("Cloud fallback read = %q/%v/%v", data, found, err)
	}
	if ctrl1.requests.Load() != 2 || cloud1.getCalls.Load() != 1 {
		t.Fatalf("fallback calls = dex %d/cloud %d, want 2/1", ctrl1.requests.Load(), cloud1.getCalls.Load())
	}

	data, found, err = store2.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-two" {
		t.Fatalf("session two direct read = %q/%v/%v", data, found, err)
	}
	data, found, err = store2.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-two" {
		t.Fatalf("session two cached direct read = %q/%v/%v", data, found, err)
	}
	if ctrl2.requests.Load() != 1 || cloud2.getCalls.Load() != 0 {
		t.Fatalf("session two cache routing = dex %d/cloud %d, want 1/0", ctrl2.requests.Load(), cloud2.getCalls.Load())
	}

	release1()
	data, found, err = store2.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-two" {
		t.Fatalf("session two survived session one teardown = %q/%v/%v", data, found, err)
	}
	if ctrl2.requests.Load() != 1 {
		t.Fatalf("session two request count after session one teardown = %d, want 1", ctrl2.requests.Load())
	}

	one := telemetryStore(t, account, "session-one")
	if one.DirectHitCount != 1 || one.CloudHitCount != 1 || one.CacheHitCount != 2 {
		t.Fatalf("session one telemetry = %+v, want direct=1 cloud=1 cache=2", one)
	}

	two := telemetryStore(t, account, "session-two")
	if two.DirectHitCount != 1 || two.CloudHitCount != 0 || two.CacheHitCount != 2 {
		t.Fatalf("session two telemetry = %+v, want direct=1 cloud=0 cache=2", two)
	}
}
func TestSessionFacadeMountDisabledThenEnablesDirectRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	refDirect, err := block.BuildBlockRef([]byte("disabled-direct"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "object",
			ProviderId:        "spacewave",
			ProviderAccountId: "account",
		},
		BlockStoreId: "store",
	}
	cache := &sessionLookupTestLower{blocks: map[string][]byte{}, writable: true}
	cloud := &sessionLookupTestLower{blocks: map[string][]byte{}}
	account := &ProviderAccount{
		accountID:         "account",
		le:                logrus.NewEntry(logrus.New()),
		sessionTransports: make(map[string]*sessionTransportState),
	}
	cacheOwner := &sourceTrackingStore{
		StoreOps:   cache,
		account:    account,
		bstoreID:   "store",
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}
	cloudOwner := &sourceTrackingStore{
		StoreOps: cloud,
		account:  account,
		bstoreID: "store",
		source:   SyncTelemetryBlockSourceCloud,
	}
	baseStore := &BlockStore{
		store:      block_store.NewStore("store", cache),
		cacheStore: cacheOwner,
		cloudStore: cloudOwner,
	}
	base := &SharedObject{blkStore: baseStore}
	facade, err := account.newSessionSharedObject(ctx, "A", ref, base)
	if err != nil {
		t.Fatalf("mount while direct disabled: %v", err)
	}

	parentBus, _, releaseParent, err := newSessionLookupTestBus(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseParent()
	priv, _, err := bifrost_crypto.GenerateKeyPair(bifrost_crypto.KeyType_Ed25519, 0)
	if err != nil {
		t.Fatal(err)
	}
	st, err := transport.NewSessionTransport(logrus.NewEntry(logrus.New()), parentBus, priv, "", "")
	if err != nil {
		t.Fatal(err)
	}
	transportCtx, cancelTransport := context.WithCancel(ctx)
	defer cancelTransport()
	go func() { _ = st.Execute(transportCtx) }()
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	ctrl := &sessionLookupTestController{values: map[string][]byte{
		refDirect.MarshalString(): []byte("enabled-direct"),
	}}
	releaseCtrl, err := st.GetChildBus().AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCtrl()
	account.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		account.sessionTransports["A"] = &sessionTransportState{
			sessionID: "A",
			transport: st,
		}
		broadcast()
	})

	data, found, err := facade.GetBlockStore().GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "enabled-direct" {
		t.Fatalf("enabled direct read on stable facade = %q/%v/%v", data, found, err)
	}
	if ctrl.requests.Load() != 1 {
		t.Fatalf("enabled direct requests = %d, want 1", ctrl.requests.Load())
	}
}

func TestSessionGetBusFallsBackWhenTransportMissing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dc := directive_controller.NewController(ctx, logrus.NewEntry(logrus.New()))
	parent := bus_inmem.NewBus(dc)
	account := &ProviderAccount{
		p:                 &Provider{b: parent},
		sessionTransports: make(map[string]*sessionTransportState),
	}
	sess := &Session{tkr: &sessionTracker{a: account, id: "A"}}
	if got := sess.GetBus(); got != parent {
		t.Fatalf("Session GetBus without transport = %v, want parent %v", got, parent)
	}
	facade := &sessionSharedObject{account: account, sessionID: "A"}
	if got := facade.GetBus(); got != parent {
		t.Fatalf("facade GetBus without transport = %v, want parent %v", got, parent)
	}
}

var _ sobject.SharedObject = ((*sessionSharedObject)(nil))
