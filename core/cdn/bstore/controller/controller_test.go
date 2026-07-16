package cdn_bstore_controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	core_test "github.com/s4wave/spacewave/db/core/test"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestConfigValidation(t *testing.T) {
	valid := NewConfig("release-cdn", "01release", "https://cdn.example.invalid")
	valid.PointerTtlDur = "5s"
	valid.RangeCacheMaxBytes = 1024
	valid.WritebackWindowBytes = 2048
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	ttl, err := valid.ParsePointerTTLDur()
	if err != nil {
		t.Fatal(err.Error())
	}
	if ttl.String() != "5s" {
		t.Fatalf("pointer TTL = %s", ttl)
	}
}

func TestControllerResolvesBlockStore(t *testing.T) {
	ctx := context.Background()
	b, sr, err := core_test.NewTestingBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(NewFactory(b))

	conf := NewConfig("release-cdn", "01release", "https://cdn.example.invalid")
	_, _, ctrlRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(conf), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()

	store, _, storeRef, err := block_store.ExLookupFirstBlockStore(ctx, b, "release-cdn", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer storeRef.Release()
	if store.GetID() != "release-cdn" {
		t.Fatalf("store id = %q", store.GetID())
	}
}

func TestBlockStoreBuilderReleaseClosesCdnStore(t *testing.T) {
	ctx := context.Background()
	conf := NewConfig("release-cdn", "01release", "https://cdn.example.invalid")
	builder := NewBlockStoreBuilder(logrus.NewEntry(logrus.New()), nil, conf)

	store, release, err := builder(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	handle, ok := store.(*blockStoreHandle)
	if !ok {
		t.Fatalf("store type = %T, want *blockStoreHandle", store)
	}
	if handle.cdnStore.GetDecodedBlockCache() == nil {
		t.Fatal("expected CDN store decoded cache before release")
	}

	release()
	if handle.cdnStore.GetDecodedBlockCache() != nil {
		t.Fatal("release did not close CDN store decoded cache")
	}
}

type notifyingStore struct {
	block.StoreOps
	putCh chan struct{}
}

func (s *notifyingStore) PutBlock(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
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

func (s *notifyingStore) waitPut(ctx context.Context) error {
	select {
	case <-s.putCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestBlockStoreBuilderDurableIndexCacheSurvivesRestart(t *testing.T) {
	const (
		spaceID = "01kpftest0000000000000003"
		cacheID = "dist"
		packID  = "01kcdnpack0000000000000008"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := []byte("bstore controller durable cache")
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
				http.Error(w, "pack source blocked", http.StatusInternalServerError)
				return
			}
			parts := strings.SplitN(strings.TrimPrefix(rangeHeader, "bytes="), "-", 2)
			off, parseErr := strconv.Atoi(parts[0])
			if parseErr != nil || off < 0 || off >= packData.Len() {
				http.Error(w, "invalid pack range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			end := packData.Len() - 1
			if len(parts) == 2 && parts[1] != "" {
				end, parseErr = strconv.Atoi(parts[1])
				if parseErr != nil {
					http.Error(w, "invalid pack range", http.StatusRequestedRangeNotSatisfiable)
					return
				}
				if end >= packData.Len() {
					end = packData.Len() - 1
				}
			}
			if end < off {
				http.Error(w, "invalid pack range", http.StatusRequestedRangeNotSatisfiable)
				return
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

	cacheStoreOps := &notifyingStore{
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
	cacheControllerRelease, err := tb.Bus.AddController(ctx, cacheCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cacheControllerRelease()

	conf := NewConfig("release-cdn", spaceID, httpServer.URL)
	conf.CacheBlockStoreId = cacheID
	conf.WritebackWindowBytes = 1 << 20
	firstStore, releaseFirst, err := NewBlockStoreBuilder(
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		conf,
	)(ctx, nil)
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

	objHandle, _, objRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		tb.Bus,
		false,
		cdn_bstore.PackIndexObjectStoreID(spaceID),
		cacheID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	indexData, indexFound, err := manifest.NewIndexCache(objHandle.GetObjectStore()).Get(ctx, packID)
	objRef.Release()
	if err != nil {
		t.Fatal(err)
	}
	if !indexFound || len(indexData) == 0 {
		t.Fatalf("durable pack index found=%v bytes=%d", indexFound, len(indexData))
	}

	reqMu.Lock()
	firstRanges := rangeRequests
	packBlocked = true
	reqMu.Unlock()
	if firstRanges == 0 {
		t.Fatal("first CDN read did not use an HTTP Range request")
	}
	releaseFirst()

	secondStore, releaseSecond, err := NewBlockStoreBuilder(
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		conf,
	)(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	got, found, err = secondStore.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("restart cache read found=%v err=%v data=%q", found, err, got)
	}
	reqMu.Lock()
	restartRanges := rangeRequests
	reqMu.Unlock()
	if restartRanges != firstRanges {
		t.Fatalf("restart cache path attempted CDN Range requests: before=%d after=%d", firstRanges, restartRanges)
	}
}
