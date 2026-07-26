package provider_spacewave

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

type sessionLookupTestLower struct {
	data      []byte
	blocks    map[string][]byte
	getCalls  atomic.Int32
	putCalls  atomic.Int32
	writable  bool
	beforeGet func()
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
	if s.beforeGet != nil {
		s.beforeGet()
	}
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

func newProductionSessionStore(
	account *ProviderAccount,
	id string,
	direct, cache, cloud *sessionLookupTestLower,
) (*sessionBlockStore, error) {
	cacheOwner := &sourceTrackingStore{
		StoreOps:   cache,
		account:    account,
		bstoreID:   id,
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}
	directOwner := &sourceTrackingStore{
		StoreOps:       direct,
		account:        account,
		bstoreID:       id,
		source:         SyncTelemetryBlockSourceDirect,
		demandStarted:  func() { account.directDemandStarted(id) },
		demandFinished: func() { account.directDemandFinished(id) },
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
	store, err := accountStore.newSessionBlockStore(func() block.StoreOps {
		return directOwner
	})
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

	direct1 := &sessionLookupTestLower{
		blocks: map[string][]byte{refDirect.MarshalString(): []byte("session-one")},
	}
	direct2 := &sessionLookupTestLower{
		blocks: map[string][]byte{refDirect.MarshalString(): []byte("session-two")},
	}

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
	store1, err := newProductionSessionStore(account, "session-one", direct1, cache1, cloud1)
	if err != nil {
		t.Fatal(err)
	}
	store2, err := newProductionSessionStore(account, "session-two", direct2, cache2, cloud2)
	if err != nil {
		t.Fatal(err)
	}

	data, found, err := store1.GetBlock(ctx, refCache)
	if err != nil || !found || string(data) != "cached-one" {
		t.Fatalf("local cache read = %q/%v/%v", data, found, err)
	}
	if direct1.getCalls.Load() != 0 || cloud1.getCalls.Load() != 0 {
		t.Fatalf("local cache hit touched DEX/Cloud: %d/%d", direct1.getCalls.Load(), cloud1.getCalls.Load())
	}

	data, found, err = store1.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-one" {
		t.Fatalf("session one direct read = %q/%v/%v", data, found, err)
	}
	if direct1.getCalls.Load() != 1 || cloud1.getCalls.Load() != 0 || cache1.putCalls.Load() != 1 {
		t.Fatalf("direct hit calls = dex %d/cloud %d/cache puts %d, want 1/0/1", direct1.getCalls.Load(), cloud1.getCalls.Load(), cache1.putCalls.Load())
	}
	data, found, err = store1.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-one" {
		t.Fatalf("session one cached direct read = %q/%v/%v", data, found, err)
	}
	if direct1.getCalls.Load() != 1 || cloud1.getCalls.Load() != 0 {
		t.Fatalf("cached direct read touched DEX/Cloud: %d/%d", direct1.getCalls.Load(), cloud1.getCalls.Load())
	}

	data, found, err = store1.GetBlock(ctx, refFallback)
	if err != nil || !found || string(data) != "cloud-one" {
		t.Fatalf("Cloud fallback read = %q/%v/%v", data, found, err)
	}
	if direct1.getCalls.Load() != 2 || cloud1.getCalls.Load() != 1 {
		t.Fatalf("fallback calls = dex %d/cloud %d, want 2/1", direct1.getCalls.Load(), cloud1.getCalls.Load())
	}

	data, found, err = store2.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-two" {
		t.Fatalf("session two direct read = %q/%v/%v", data, found, err)
	}
	data, found, err = store2.GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "session-two" {
		t.Fatalf("session two cached direct read = %q/%v/%v", data, found, err)
	}
	if direct2.getCalls.Load() != 1 || cloud2.getCalls.Load() != 0 {
		t.Fatalf("session two cache routing = dex %d/cloud %d, want 1/0", direct2.getCalls.Load(), cloud2.getCalls.Load())
	}

	one := telemetryStore(t, account, "session-one")
	if one.DirectHitCount != 1 || one.CloudHitCount != 1 || one.CacheHitCount != 2 {
		t.Fatalf("session one telemetry = %+v, want direct=1 cloud=1 cache=2", one)
	}

	two := telemetryStore(t, account, "session-two")
	if two.DirectHitCount != 1 || two.CloudHitCount != 0 || two.CacheHitCount != 1 {
		t.Fatalf("session two telemetry = %+v, want direct=1 cloud=0 cache=1", two)
	}
}

func TestSessionDirectLookupTracksActiveDemand(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	account := &ProviderAccount{}
	owner := &account.transportComposition
	owner.init(account)
	state := newTransportCompositionSession()
	state.snapshot = TransportCompositionSnapshot{
		DirectP2PEnabled: true,
		P2PState:         TransportCompositionP2PStateIdle,
		ActivePeerCount:  1,
	}
	owner.mtx.Lock()
	owner.sessions["session"] = state
	owner.mtx.Unlock()

	data := []byte("active direct")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	direct := &sessionLookupTestLower{
		blocks: map[string][]byte{ref.MarshalString(): data},
		beforeGet: func() {
			close(started)
			<-release
		},
	}
	cache := &sessionLookupTestLower{writable: true}
	cloud := &sessionLookupTestLower{}
	store, err := newProductionSessionStore(account, "session", direct, cache, cloud)
	if err != nil {
		t.Fatal(err)
	}

	resultCh := make(chan struct {
		data  []byte
		found bool
		err   error
	}, 1)
	go func() {
		got, found, getErr := store.GetBlock(ctx, ref)
		resultCh <- struct {
			data  []byte
			found bool
			err   error
		}{data: got, found: found, err: getErr}
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("direct lookup did not start")
	}
	snapshot, _ := account.GetTransportCompositionSnapshotWithWait("session")
	if snapshot.P2PState != TransportCompositionP2PStateActive {
		t.Fatalf("demand state while reading = %v, want active", snapshot.P2PState)
	}
	close(release)

	select {
	case result := <-resultCh:
		if result.err != nil || !result.found || string(result.data) != string(data) {
			t.Fatalf("direct lookup = %q/%v/%v", result.data, result.found, result.err)
		}
	case <-ctx.Done():
		t.Fatal("direct lookup did not finish")
	}
	snapshot, _ = account.GetTransportCompositionSnapshotWithWait("session")
	if snapshot.P2PState != TransportCompositionP2PStateIdle {
		t.Fatalf("demand state after reading = %v, want idle", snapshot.P2PState)
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
	direct := &sessionLookupTestLower{
		blocks: map[string][]byte{refDirect.MarshalString(): []byte("enabled-direct")},
	}
	account := &ProviderAccount{
		accountID: "account",
		le:        logrus.NewEntry(logrus.New()),
		p2pSyncs:  make(map[string]*p2pSyncState),
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
	facade, err := account.newSessionSharedObject("A", ref, base)
	if err != nil {
		t.Fatalf("mount while direct disabled: %v", err)
	}

	account.p2pSyncMtx.Lock()
	account.p2pSyncs["A"] = &p2pSyncState{
		stores: map[string]block.StoreOps{
			BlockStoreBucketID("account", "store"): direct,
		},
	}
	account.p2pSyncMtx.Unlock()

	data, found, err := facade.GetBlockStore().GetBlock(ctx, refDirect)
	if err != nil || !found || string(data) != "enabled-direct" {
		t.Fatalf("enabled direct read on stable facade = %q/%v/%v", data, found, err)
	}
	if direct.getCalls.Load() != 1 {
		t.Fatalf("enabled direct requests = %d, want 1", direct.getCalls.Load())
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
