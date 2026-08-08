package cdn_world_controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

type notifyingBlockStore struct {
	block.StoreOps
	putCh chan struct{}
}

func (s *notifyingBlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	select {
	case s.putCh <- struct{}{}:
	default:
	}
	return ref, existed, nil
}

func (s *notifyingBlockStore) waitPut(ctx context.Context) error {
	select {
	case <-s.putCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestConfiguredCacheWritebackSurvivesCdnRestart(t *testing.T) {
	const (
		spaceID = "01kpftest0000000000000002"
		cacheID = "dist"
		packID  = "01kcdnpack0000000000000007"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := []byte("world controller durable cache")
	blockHash, err := hash.Sum(hash.HashType_HashType_SHA256, data)
	if err != nil {
		t.Fatal(err)
	}
	var packData bytes.Buffer
	packResult, err := writer.PackBlocks(&packData, func() (*hash.Hash, []byte, error) {
		if packData.Len() != 0 {
			return nil, nil, nil
		}
		return blockHash, data, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pointer, err := (&cdn.CdnRootPointer{
		SpaceId: spaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          packID,
			BloomFilter: packResult.BloomFilter,
			BlockCount:  1,
			SizeBytes:   uint64(packData.Len()),
		}},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	pointer = []byte(packedmsg.EncodePackedMessage(pointer))

	var reqMu sync.Mutex
	var rangeRequests int
	var packBlocked bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+spaceID+"/root.packedmsg":
			_, _ = w.Write(pointer)
		case strings.HasPrefix(r.URL.Path, "/"+spaceID+"/packs/") &&
			strings.HasSuffix(r.URL.Path, "/"+packID+".kvf"):
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				_, _ = w.Write(packData.Bytes())
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
			const prefix = "bytes="
			parts := strings.SplitN(strings.TrimPrefix(rangeHeader, prefix), "-", 2)
			off, _ := strconv.Atoi(parts[0])
			end := len(packData.Bytes()) - 1
			if parts[1] != "" {
				end, _ = strconv.Atoi(parts[1])
			}
			if end >= len(packData.Bytes()) {
				end = len(packData.Bytes()) - 1
			}
			w.Header().Set("Content-Range", "bytes "+strconv.Itoa(off)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(packData.Len()))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(packData.Bytes()[off : end+1])
		default:
			http.NotFound(w, r)
		}
	}))
	defer httpServer.Close()

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
	cacheCtrl := block_store_controller.NewController(
		logrus.NewEntry(logrus.New()),
		controller.NewInfo("test/cache", controller.MustParseVersion("0.0.1"), "test cache"),
		func(context.Context, func()) (block_store.Store, func(), error) {
			return block_store.NewStore(cacheID, cacheStoreOps), nil, nil
		},
		[]string{cacheID},
		true,
		nil,
		false,
		false,
	)
	cacheControllerRelease, err := b.AddController(ctx, cacheCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cacheControllerRelease()

	conf := NewConfig("release-world", spaceID, httpServer.URL)
	conf.CacheBlockStoreId = cacheID
	conf.WritebackWindowBytes = 1 << 20
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

	if err := cacheStoreOps.waitPut(ctx); err != nil {
		t.Fatalf("wait for CDN writeback: %v", err)
	}
	cacheStore, _, cacheRef, err := block_store.ExLookupFirstBlockStore(ctx, b, cacheID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cacheRef.Release()
	cached, cachedFound, cacheErr := cacheStore.GetBlock(ctx, ref)
	if cacheErr != nil {
		t.Fatal(cacheErr)
	}
	if !cachedFound || !bytes.Equal(cached, data) {
		t.Fatalf("cached block found=%v data=%q", cachedFound, cached)
	}
	reqMu.Lock()
	firstRanges := rangeRequests
	packBlocked = true
	reqMu.Unlock()
	if firstRanges == 0 {
		t.Fatal("first CDN read did not use an HTTP Range request")
	}
	releaseFirst()

	secondController := NewController(logrus.NewEntry(logrus.New()), b, conf)
	secondStore, releaseSecond, err := secondController.newBlockStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if _, err := secondStore.Refresh(ctx); err != nil {
		t.Fatalf("restart root refresh failed while pack source was blocked: %v", err)
	}
	cached, cachedFound, cacheErr = secondStore.GetBlock(ctx, ref)
	if cacheErr != nil {
		t.Fatal(cacheErr)
	}
	if !cachedFound || !bytes.Equal(cached, data) {
		t.Fatalf("restart cache read found=%v data=%q", cachedFound, cached)
	}
	reqMu.Lock()
	restartRanges := rangeRequests
	reqMu.Unlock()
	if restartRanges != firstRanges {
		t.Fatalf("restart cache path attempted CDN Range requests: before=%d after=%d", firstRanges, restartRanges)
	}
}

func TestReleaseWorldExternalBucketBuildAPIUsesCdnStoreMapping(t *testing.T) {
	const (
		bucketID = "spacewave-release"
		spaceID  = "01releaseworld00000000000001"
		storeID  = "spacewave-release-cdn"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pointer, err := (&cdn.CdnRootPointer{SpaceId: spaceID}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	pointer = []byte(packedmsg.EncodePackedMessage(pointer))
	var rootRequests atomic.Int32
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+spaceID+"/root.packedmsg" {
			http.NotFound(w, r)
			return
		}
		rootRequests.Add(1)
		_, _ = w.Write(pointer)
	}))
	defer hs.Close()

	cdnStore, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: hs.URL,
		SpaceID:    spaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cdnStore.Close()
	cdnBlockStore := block_store.NewStore(storeID, cdnStore)

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

	wrongCtx, wrongCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, _, wrongRef, wrongErr := bucket.ExBuildBucketAPI(
		wrongCtx,
		b,
		false,
		bucketID,
		"entrypoint",
		nil,
	)
	if wrongRef != nil {
		wrongRef.Release()
	}
	if wrongErr == nil || wrongCtx.Err() != context.DeadlineExceeded {
		t.Fatalf("entrypoint bucket mapping error = %v context = %v, want timed-out wait", wrongErr, wrongCtx.Err())
	}
	wrongCancel()
	if got := rootRequests.Load(); got != 0 {
		t.Fatalf("entrypoint mapping made %d CDN requests before resolving a bucket", got)
	}

	handle, _, handleRef, err := bucket.ExBuildBucketAPI(ctx, b, false, bucketID, storeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handleRef.Release()
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
