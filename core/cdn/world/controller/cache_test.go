package cdn_world_controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_rpc "github.com/s4wave/spacewave/db/block/store/rpc"
	block_store_rpc_server "github.com/s4wave/spacewave/db/block/store/rpc/server"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// notifyingBlockStore signals putCh after each successful write so a test can
// wait for the CDN writeback to land in the cache.
type notifyingBlockStore struct {
	block.StoreOps
	putCh chan struct{}
}

// PutBlock writes the block and signals one put.
func (s *notifyingBlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	s.signalPut()
	return ref, existed, nil
}

// PutBlockBatch writes entries and signals one put.
func (s *notifyingBlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := s.StoreOps.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	s.signalPut()
	return nil
}

// signalPut records a put without blocking when one is already pending.
func (s *notifyingBlockStore) signalPut() {
	select {
	case s.putCh <- struct{}{}:
	default:
	}
}

// waitPut waits for a put or for ctx to end.
func (s *notifyingBlockStore) waitPut(ctx context.Context) error {
	select {
	case <-s.putCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// packTestBlock packs data as the only block of a packfile with id packID. It
// returns the block hash, the packfile bytes and the packfile entry.
func packTestBlock(t *testing.T, packID string, data []byte) (*hash.Hash, []byte, *packfile.PackfileEntry) {
	// Hash the block.
	blockHash, err := hash.Sum(hash.HashType_HashType_SHA256, data)
	if err != nil {
		t.Fatal(err)
	}

	// Write a packfile holding only that block.
	var packData bytes.Buffer
	packResult, err := writer.PackBlocks(&packData, func() (*hash.Hash, *block.StoredBlock, error) {
		if packData.Len() != 0 {
			return nil, nil, nil
		}
		return blockHash, &block.StoredBlock{Data: data, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Describe the packfile for the root pointer.
	entry := &packfile.PackfileEntry{
		Id:          packID,
		BloomFilter: packResult.BloomFilter,
		BlockCount:  1,
		SizeBytes:   uint64(packData.Len()),
	}
	return blockHash, packData.Bytes(), entry
}

// encodeRootPointer encodes ptr as the packed message the CDN serves.
func encodeRootPointer(t *testing.T, ptr *cdn.CdnRootPointer) []byte {
	data, err := ptr.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return []byte(packedmsg.EncodePackedMessage(data))
}

// testHeadCheckpoint returns a genesis checkpoint of spaceID whose plain World
// has an empty head ref.
func testHeadCheckpoint(t *testing.T, spaceID string) *sobject.SOCheckpoint {
	// Sign a checkpoint whose World has an empty head.
	t.Helper()
	state, err := (&sobject_world_engine.InnerState{HeadRef: &bucket.ObjectRef{}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	inner, err := (&sobject.SOCheckpointInner{
		SharedObjectId: spaceID,
		ConfigHash:     make([]byte, sha256.Size),
		ReplayVersion:  sobject.SOReplayVersion,
		StateData:      state,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return &sobject.SOCheckpoint{Inner: inner}
}

// writePackRange answers an HTTP Range request for "bytes=off-end" or
// "bytes=off-" with the matching slice of pack.
func writePackRange(w http.ResponseWriter, rangeHeader string, pack []byte) {
	// Parse the requested span and clamp it to the pack.
	parts := strings.SplitN(strings.TrimPrefix(rangeHeader, "bytes="), "-", 2)
	off, _ := strconv.Atoi(parts[0])
	end := len(pack) - 1
	if len(parts) == 2 && parts[1] != "" {
		end, _ = strconv.Atoi(parts[1])
	}
	end = min(end, len(pack)-1)

	// Write the partial content.
	w.Header().Set("Content-Range", "bytes "+strconv.Itoa(off)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(len(pack)))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(pack[off : end+1])
}

// addTestCacheStore serves ops as the block store id on b until the test ends.
func addTestCacheStore(ctx context.Context, t *testing.T, b bus.Bus, id string, ops block.StoreOps) {
	// Serve ops under id from a block store controller.
	t.Helper()
	ctrl := block_store_controller.NewController(
		logrus.NewEntry(logrus.New()),
		controller.NewInfo("test/cache", controller.MustParseVersion("0.0.1"), "test cache"),
		func(context.Context, func()) (block_store.Store, func(), error) {
			return block_store.NewStore(id, ops), nil, nil
		},
		[]string{id},
		true,
		nil,
		false,
		false,
	)
	release, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

// TestCdnOutageKeepsCachedBlocksReadable proves that a controller started
// while the CDN is unreachable reports its World engine unavailable and still
// serves blocks cached under the last fetched root pointer.
func TestCdnOutageKeepsCachedBlocksReadable(t *testing.T) {
	// Name the fixture and bound the test.
	const (
		spaceID = "01kpftest0000000000000003"
		cacheID = "dist"
		packID  = "01kcdnpack0000000000000008"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish one packed block under the Space root pointer.
	data := []byte("cached before a CDN outage")
	blockHash, packData, packEntry := packTestBlock(t, packID, data)
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId: spaceID,
		Packs:   []*packfile.PackfileEntry{packEntry},
	})

	// Serve the CDN until it goes down.
	var down atomic.Bool
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case down.Load():
			http.Error(w, "CDN is unavailable", http.StatusServiceUnavailable)
		case r.URL.Path == "/"+spaceID+"/root.packedmsg":
			_, _ = w.Write(pointer)
		case strings.HasSuffix(r.URL.Path, "/"+packID+".kvf"):
			if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
				writePackRange(w, rangeHeader, packData)
				return
			}
			_, _ = w.Write(packData)
		default:
			http.NotFound(w, r)
		}
	}))

	// Start a bus with an in-memory volume to back the cache.
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), testbed.WithVolumeConfig(
		&volume_kvtxinmem.Config{
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias:           []string{cacheID},
				DisableLookupBlockStore: true,
			},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	b := tb.Bus
	cacheStoreOps := &notifyingBlockStore{
		StoreOps: tb.Volume,
		putCh:    make(chan struct{}, 1),
	}
	addTestCacheStore(ctx, t, b, cacheID, cacheStoreOps)

	// Read the block once while the CDN is up, caching it and its pointer.
	conf := NewConfig("release-world", spaceID, cdnURL)
	conf.CacheBlockStoreId = cacheID
	onlineStore, releaseOnline, err := NewController(logrus.NewEntry(logrus.New()), b, conf).newBlockStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: blockHash}
	if got, found, err := onlineStore.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("online read found=%v err=%v data=%q", found, err, got)
	}
	if err := cacheStoreOps.waitPut(ctx); err != nil {
		t.Fatalf("wait for CDN writeback: %v", err)
	}
	releaseOnline()

	// Mount the Space in a new controller while the CDN is down.
	down.Store(true)
	release, err := b.AddController(ctx, NewController(logrus.NewEntry(logrus.New()), b, conf), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The engine lookup goes idle without a value.
	errUnavailable := errors.New("world engine unavailable")
	_, _, engineRef, err := bus.ExecWaitValue[world.LookupWorldEngineValue](
		ctx, b, world.NewLookupWorldEngine(conf.GetEngineId()),
		func(isIdle bool, _ []error) (bool, error) {
			if isIdle {
				return false, errUnavailable
			}
			return true, nil
		},
		nil,
		nil,
	)
	if engineRef != nil {
		engineRef.Release()
	}
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("engine lookup returned %v, want idle without an engine", err)
	}

	// The cached block still reads through the mounted store.
	store, _, storeRef, err := block_store.ExLookupFirstBlockStore(ctx, b, spaceID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer storeRef.Release()
	got, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("cached read during outage found=%v err=%v data=%q", found, err, got)
	}
}

// TestConfiguredCacheWritebackSurvivesCdnRestart proves a block read from the
// CDN is written back to the configured cache, and a restarted controller
// serves it from that cache without another Range request.
func TestConfiguredCacheWritebackSurvivesCdnRestart(t *testing.T) {
	// Name the fixture and bound the test.
	const (
		spaceID = "01kpftest0000000000000002"
		cacheID = "dist"
		packID  = "01kcdnpack0000000000000007"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish one packed block under the Space root pointer.
	data := []byte("world controller durable cache")
	blockHash, packData, packEntry := packTestBlock(t, packID, data)
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId: spaceID,
		Packs:   []*packfile.PackfileEntry{packEntry},
	})

	// Serve the CDN, counting Range requests and failing them once blocked.
	var reqMu sync.Mutex
	var rangeRequests int
	var packBlocked bool
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+spaceID+"/root.packedmsg":
			_, _ = w.Write(pointer)
		case strings.HasPrefix(r.URL.Path, "/"+spaceID+"/packs/") &&
			strings.HasSuffix(r.URL.Path, "/"+packID+".kvf"):
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				_, _ = w.Write(packData)
				return
			}

			reqMu.Lock()
			rangeRequests++
			blocked := packBlocked
			reqMu.Unlock()
			if blocked {
				http.Error(w, "pack source blocked", http.StatusServiceUnavailable)
				return
			}
			writePackRange(w, rangeHeader, packData)
		default:
			http.NotFound(w, r)
		}
	}))

	// Start a bus with an in-memory volume to back the cache.
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), testbed.WithVolumeConfig(
		&volume_kvtxinmem.Config{
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias:           []string{cacheID},
				DisableLookupBlockStore: true,
			},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	b := tb.Bus

	// Expose the volume as the cache block store and observe its writes.
	cacheStoreOps := &notifyingBlockStore{
		StoreOps: tb.Volume,
		putCh:    make(chan struct{}, 1),
	}
	addTestCacheStore(ctx, t, b, cacheID, cacheStoreOps)

	// Configure the controller to write CDN reads back to the cache.
	conf := NewConfig("release-world", spaceID, cdnURL)
	conf.CacheBlockStoreId = cacheID
	conf.WritebackWindowBytes = 1 << 20

	// Read the block through the first controller's CDN store.
	firstController := NewController(logrus.NewEntry(logrus.New()), b, conf)
	firstStore, releaseFirst, err := firstController.newBlockStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: blockHash}
	got, found, err := firstStore.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("first CDN read found=%v err=%v data=%q", found, err, got)
	}

	// Wait for the writeback and check the cache holds the block.
	if err := cacheStoreOps.waitPut(ctx); err != nil {
		t.Fatalf("wait for CDN writeback: %v", err)
	}
	cacheStore, _, cacheRef, err := block_store.ExLookupFirstBlockStore(ctx, b, cacheID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cacheRef.Release()
	cached, cachedFound, err := cacheStore.GetBlock(ctx, ref)
	if err != nil || !cachedFound || !bytes.Equal(cached, data) {
		t.Fatalf("cached block found=%v err=%v data=%q", cachedFound, err, cached)
	}

	// Block the pack source and stop the first controller's store.
	reqMu.Lock()
	firstRanges := rangeRequests
	packBlocked = true
	reqMu.Unlock()
	if firstRanges == 0 {
		t.Fatal("first CDN read did not use an HTTP Range request")
	}
	releaseFirst()

	// Read the block again through a restarted controller.
	secondController := NewController(logrus.NewEntry(logrus.New()), b, conf)
	secondStore, releaseSecond, err := secondController.newBlockStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if _, err := secondStore.Refresh(ctx); err != nil {
		t.Fatalf("restart root refresh failed while pack source was blocked: %v", err)
	}
	cached, cachedFound, err = secondStore.GetBlock(ctx, ref)
	if err != nil || !cachedFound || !bytes.Equal(cached, data) {
		t.Fatalf("restart cache read found=%v err=%v data=%q", cachedFound, err, cached)
	}

	// Check the restarted read came from the cache.
	reqMu.Lock()
	restartRanges := rangeRequests
	reqMu.Unlock()
	if restartRanges != firstRanges {
		t.Fatalf("restart cache path attempted CDN Range requests: before=%d after=%d", firstRanges, restartRanges)
	}
}

// TestReleaseWorldExternalBucketBuildAPIUsesCdnStoreMapping proves a bucket
// API build resolves only through the CDN store mapping, and makes no CDN
// request for a mapping to another store.
func TestReleaseWorldExternalBucketBuildAPIUsesCdnStoreMapping(t *testing.T) {
	// Name the fixture and bound the test.
	const (
		bucketID = "spacewave-release"
		spaceID  = "01releaseworld00000000000001"
		storeID  = "spacewave-release-cdn"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Serve an empty root pointer and count its requests.
	pointer := encodeRootPointer(t, &cdn.CdnRootPointer{SpaceId: spaceID})
	var rootRequests atomic.Int32
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+spaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		rootRequests.Add(1)
		_, _ = w.Write(pointer)
	}))

	// Open the CDN block store.
	cdnStore, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: cdnURL,
		SpaceID:    spaceID,
		HttpClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cdnStore.Close()
	cdnBlockStore := block_store.NewStore(storeID, cdnStore)

	// Map the bucket to the CDN store on a fresh bus.
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	bucketConf, err := bucket.NewConfig(bucketID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	bucketCtrl := block_store_bucket.NewController(
		storeID,
		bucketConf,
		func(context.Context, func()) (block_store.Store, func(), error) {
			return cdnBlockStore, func() {}, nil
		},
	)
	releaseBucketCtrl, err := b.AddController(ctx, bucketCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBucketCtrl()

	// A build through another store waits and touches no CDN.
	wrongCtx, wrongCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer wrongCancel()
	_, _, wrongRef, wrongErr := bucket.ExBuildBucketAPI(wrongCtx, b, false, bucketID, "entrypoint", nil)
	if wrongRef != nil {
		wrongRef.Release()
	}

	// The wait times out without a CDN request.
	if wrongErr == nil || wrongCtx.Err() != context.DeadlineExceeded {
		t.Fatalf("entrypoint bucket mapping error = %v context = %v, want timed-out wait", wrongErr, wrongCtx.Err())
	}
	if got := rootRequests.Load(); got != 0 {
		t.Fatalf("entrypoint mapping made %d CDN requests before resolving a bucket", got)
	}

	// Build the bucket API through the CDN store.
	handle, _, handleRef, err := bucket.ExBuildBucketAPI(ctx, b, false, bucketID, storeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handleRef.Release()

	// A read misses the empty root after one root request.
	rootHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("release root"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := handle.GetBucket().GetBlock(ctx, &block.BlockRef{Hash: rootHash})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("empty CDN root unexpectedly contained the requested block")
	}
	if got := rootRequests.Load(); got != 1 {
		t.Fatalf("CDN store mapping made %d CDN root requests, want one", got)
	}
}

// retryingWorldController reports the first Execute result on firstErr and
// runs Execute once more when it failed.
type retryingWorldController struct {
	*Controller
	firstErr chan error
}

// Execute runs the controller, retrying once after a failure.
func (c *retryingWorldController) Execute(ctx context.Context) error {
	err := c.Controller.Execute(ctx)
	c.firstErr <- err
	if err == nil {
		return nil
	}
	return c.Controller.Execute(ctx)
}

// observedContext closes done the first time a caller waits on Done.
type observedContext struct {
	context.Context
	done chan struct{}
	once sync.Once
}

// Done reports the wait and returns the wrapped context's channel.
func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.done) })
	return c.Context.Done()
}

// TestReleaseWorldSharesTransportAndDurableCacheAcrossRpcBridge proves host
// and plugin reads share one CDN transport and one durable cache across the
// RPC bridge, and that the RPC authority follows each controller lifetime.
func TestReleaseWorldSharesTransportAndDurableCacheAcrossRpcBridge(t *testing.T) {
	// Name the fixture and bound the test.
	const (
		spaceID   = "01ksharedreleaseworld00000001"
		cacheID   = "dist"
		packID    = "01ksharedreleasepack00000001"
		serviceID = ReleaseBlockStoreID + "/block.rpc.BlockStore"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Pack one block and build an invalid and a valid root pointer for it.
	data := []byte("shared release world block")
	blockHash, packData, packEntry := packTestBlock(t, packID, data)
	invalidPointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId:    spaceID,
		Checkpoint: &sobject.SOCheckpoint{Inner: []byte("invalid checkpoint")},
		Packs:      []*packfile.PackfileEntry{packEntry},
	})
	validPointer := encodeRootPointer(t, &cdn.CdnRootPointer{
		SpaceId:    spaceID,
		Checkpoint: testHeadCheckpoint(t, spaceID),
		Packs:      []*packfile.PackfileEntry{packEntry},
	})

	// Serve the invalid root first and hold the first Range request until
	// released.
	firstRoot := make(chan struct{})
	releaseFirstRoot := make(chan struct{})
	rangeStarted := make(chan struct{})
	releaseRange := make(chan struct{})
	var rootRequests atomic.Int32
	var rangeRequests atomic.Int32
	cdnURL := serveTestCDN(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+spaceID+"/root.packedmsg" {
			if rootRequests.Add(1) == 1 {
				close(firstRoot)
				<-releaseFirstRoot
				_, _ = w.Write(invalidPointer)
				return
			}
			_, _ = w.Write(validPointer)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/"+packID+".kvf") {
			http.NotFound(w, r)
			return
		}
		if rangeRequests.Add(1) == 1 {
			close(rangeStarted)
			<-releaseRange
		}
		writePackRange(w, r.Header.Get("Range"), packData)
	}))

	// Start the host bus with a cache volume, and a separate plugin bus.
	le := logrus.NewEntry(logrus.New())
	host, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(
		&volume_kvtxinmem.Config{VolumeConfig: &volume_controller.Config{
			VolumeIdAlias:           []string{cacheID},
			DisableLookupBlockStore: true,
		}},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Release()
	pluginBus, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Expose the host volume as the cache block store and observe its writes.
	cacheStore := &notifyingBlockStore{StoreOps: host.Volume, putCh: make(chan struct{}, 1)}
	cacheCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo("test/shared-cache", controller.MustParseVersion("0.0.1"), "test shared cache"),
		func(context.Context, func()) (block_store.Store, func(), error) {
			return block_store.NewStore(cacheID, cacheStore), nil, nil
		},
		[]string{cacheID}, true, nil, false, false,
	)
	releaseCache, err := host.Bus.AddController(ctx, cacheCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCache()

	// Bridge plugin RPC lookups under the plugin-host/ prefix to the host bus.
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(host.Bus, "", false))))
	clientCtrl := bifrost_rpc.NewClientController(
		le,
		pluginBus,
		controller.NewInfo("test/plugin-rpc-client", controller.MustParseVersion("0.0.1"), "test plugin RPC client"),
		client,
		[]string{"plugin-host/"},
	)
	releaseClient, err := pluginBus.AddController(ctx, clientCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseClient()

	// A plugin store on the unprefixed service never resolves.
	wrongConf := block_store_rpc.NewConfig("wrong-prefix", serviceID, true, nil)
	wrongConf.LookupOnStart = true
	wrongCtrl := block_store_rpc.NewController(pluginBus, le, wrongConf)
	releaseWrong, err := pluginBus.AddController(ctx, wrongCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Its lookup times out.
	wrongCtx, wrongCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, _, wrongRef, wrongErr := block_store.ExLookupFirstBlockStore(wrongCtx, pluginBus, "wrong-prefix", false, nil)
	if wrongRef != nil {
		wrongRef.Release()
	}
	wrongCancel()
	releaseWrong()
	if wrongErr == nil || !errors.Is(wrongCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("unprefixed RPC service lookup error = %v context = %v, want deadline", wrongErr, wrongCtx.Err())
	}

	// Open the plugin store on the prefixed service.
	rpcConf := block_store_rpc.NewConfig(ReleaseBlockStoreID, "plugin-host/"+serviceID, true, []string{"spacewave-release"})
	rpcConf.LookupOnStart = true
	rpcCtrl := block_store_rpc.NewController(pluginBus, le, rpcConf)
	releaseRPC, err := pluginBus.AddController(ctx, rpcCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseRPC()

	// Look up the plugin store.
	pluginStore, _, pluginStoreRef, err := block_store.ExLookupFirstBlockStore(ctx, pluginBus, ReleaseBlockStoreID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pluginStoreRef.Release()

	// Serve the host Release block store as the RPC service.
	serverCtrl := block_store_rpc_server.NewController(host.Bus, block_store_rpc_server.NewConfig(
		ReleaseBlockStoreID, false, serviceID, "", hash.HashType_HashType_UNKNOWN,
	))
	releaseServer, err := host.Bus.AddController(ctx, serverCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseServer()

	// Watch the RPC service authorities the server adds and removes.
	serviceAdded := make(chan srpc.Invoker, 4)
	serviceRemoved := make(chan srpc.Invoker, 4)
	_, serviceRef, err := host.Bus.AddDirective(
		bifrost_rpc.NewLookupRpcService(serviceID, ""),
		directive.NewCallbackHandler(
			func(v directive.AttachedValue) { serviceAdded <- v.GetValue().(srpc.Invoker) },
			func(v directive.AttachedValue) { serviceRemoved <- v.GetValue().(srpc.Invoker) },
			nil,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer serviceRef.Release()

	// Configure the world controller with the durable cache.
	conf := NewConfig(releaseWorldEngineID, spaceID, cdnURL)
	conf.CacheBlockStoreId = cacheID
	conf.WritebackWindowBytes = 1 << 20
	worldCtrl := NewController(le, host.Bus, conf)

	// Run it until the first Execute fails.
	retryingCtrl := &retryingWorldController{Controller: worldCtrl, firstErr: make(chan error, 1)}
	releaseWorld, err := host.Bus.AddController(ctx, retryingCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-firstRoot
	firstService := <-serviceAdded
	close(releaseFirstRoot)
	if err := <-retryingCtrl.firstErr; err == nil {
		t.Fatal("invalid first root did not fail the first Execute attempt")
	}

	// The retry withdraws the first authority and adds a new one.
	if removed := <-serviceRemoved; removed != firstService {
		t.Fatal("RPC server withdrew a different first authority")
	}
	secondService := <-serviceAdded
	if secondService == firstService {
		t.Fatal("RPC server retained its first authority across Execute retry")
	}

	// Start a plugin read and hold it in the first Range request.
	hostStore, _, hostStoreRef, err := block_store.ExLookupFirstBlockStore(ctx, host.Bus, ReleaseBlockStoreID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := &block.BlockRef{Hash: blockHash}
	pluginCtx, cancelPlugin := context.WithCancel(ctx)
	pluginDone := make(chan error, 1)
	go func() {
		_, _, err := pluginStore.GetBlock(pluginCtx, ref)
		pluginDone <- err
	}()
	<-rangeStarted

	// Join a host read to it, then cancel the plugin read.
	hostCtx := &observedContext{Context: ctx, done: make(chan struct{})}
	hostDone := make(chan error, 1)
	var hostData []byte
	var hostFound bool
	go func() {
		var err error
		hostData, hostFound, err = hostStore.GetBlock(hostCtx, ref)
		hostDone <- err
	}()
	<-hostCtx.done
	cancelPlugin()
	if err := <-pluginDone; err == nil {
		t.Fatal("canceled plugin RPC read returned no error")
	}

	// The host read completes from the one shared Range request.
	close(releaseRange)
	if err := <-hostDone; err != nil {
		t.Fatal(err)
	}
	if !hostFound || !bytes.Equal(hostData, data) {
		t.Fatalf("host read found=%v data=%q, want %q", hostFound, hostData, data)
	}
	if got := rangeRequests.Load(); got != 1 {
		t.Fatalf("joined host and plugin reads made %d Range requests, want one", got)
	}

	// The block reaches the durable cache.
	if err := cacheStore.waitPut(ctx); err != nil {
		t.Fatalf("wait for durable writeback: %v", err)
	}
	cached, found, err := cacheStore.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(cached, data) {
		t.Fatalf("durable writeback found=%v err=%v data=%q", found, err, cached)
	}
	hostStoreRef.Release()

	// Replacing the controller withdraws its authority and adds a new one.
	releaseWorld()
	if removed := <-serviceRemoved; removed != secondService {
		t.Fatal("RPC server withdrew a different teardown authority")
	}
	worldCtrl = NewController(le, host.Bus, conf)
	releaseReplacement, err := host.Bus.AddController(ctx, worldCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReplacement()
	thirdService := <-serviceAdded
	if thirdService == secondService {
		t.Fatal("RPC server retained its closed authority after controller replacement")
	}

	// The retained plugin client reads through the replacement.
	got, found, err := pluginStore.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("retained plugin client after replacement found=%v err=%v data=%q", found, err, got)
	}
}
