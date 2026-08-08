//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore_controller "github.com/s4wave/spacewave/core/cdn/bstore/controller"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/dex"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const (
	releaseCDNFallbackSpaceID      = "01launcherfallback0000000000"
	releaseCDNFallbackBucketID     = "spacewave-release"
	releaseCDNFallbackStoreID      = "spacewave-release-cdn"
	releaseCDNFallbackCacheStoreID = "dist/spacewave"
)

func TestSignedDistConfigAppliesReleaseCDNBlockFallbackWithWriteback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blockData := []byte("release world block from cdn pack")
	pack := buildReleaseCDNFallbackPack(t, "01launcherpack00000000000001", blockData)
	pointerBytes := encodeReleaseCDNFallbackPointer(t, pack)
	server := newReleaseCDNFallbackServer(t, pointerBytes, pack)
	hs := httptest.NewServer(http.HandlerFunc(server.handle))
	defer hs.Close()

	ref := &block.BlockRef{Hash: pack.blockHash}
	le := logrus.NewEntry(logrus.New())
	b, sr, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(cdn_bstore_controller.NewFactory(b))

	_, _, configSetRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer configSetRef.Release()

	cache := newReleaseCDNFallbackWritebackStore()
	cacheCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo("test/release-cdn-cache", cdn_bstore_controller.Version, "release cdn fallback cache"),
		block_store_controller.NewBlockStoreBuilder(block_store.NewStore(releaseCDNFallbackCacheStoreID, cache)),
		[]string{releaseCDNFallbackCacheStoreID},
		true,
		nil,
		false,
		false,
	)
	relCacheCtrl, err := b.AddController(ctx, cacheCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relCacheCtrl()

	launcherConfigSet, err := buildReleaseCDNFallbackConfigSet(1, hs.URL)
	if err != nil {
		t.Fatal(err.Error())
	}
	ctrl := &Controller{
		le:  le,
		bus: b,
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DistConfig: &spacewave_launcher.DistConfig{
					ProjectId:         "spacewave",
					Rev:               1,
					ChannelKey:        "stable",
					LauncherConfigSet: launcherConfigSet,
				},
			},
		),
	}
	applyCtx, applyCancel := context.WithCancel(ctx)
	defer applyCancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.applyDistConfigSet(applyCtx)
	}()
	defer func() {
		applyCancel()
		if err := <-errCh; err != context.Canceled {
			t.Fatalf("applyDistConfigSet() error = %v, want context.Canceled", err)
		}
	}()

	store, _, storeRef, err := block_store.ExLookupFirstBlockStore(ctx, b, releaseCDNFallbackStoreID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer storeRef.Release()
	if store.GetID() != releaseCDNFallbackStoreID {
		t.Fatalf("block store id = %q, want %q", store.GetID(), releaseCDNFallbackStoreID)
	}

	got := waitReleaseCDNFallbackLookup(t, ctx, b, ref)
	if !bytes.Equal(got, blockData) {
		t.Fatalf("lookup data = %q, want %q", got, blockData)
	}
	if err := cache.waitPut(ctx); err != nil {
		t.Fatal(err.Error())
	}
	cached, found, err := cache.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || !bytes.Equal(cached, blockData) {
		t.Fatalf("writeback cache found=%v data=%q", found, cached)
	}

	rangesAfterFirst := server.ranges
	got = waitReleaseCDNFallbackLookup(t, ctx, b, ref)
	if !bytes.Equal(got, blockData) {
		t.Fatalf("second lookup data = %q, want %q", got, blockData)
	}
	if server.ranges != rangesAfterFirst {
		t.Fatalf("second lookup fetched remote ranges: before=%d after=%d", rangesAfterFirst, server.ranges)
	}
}

func buildReleaseCDNFallbackConfigSet(
	rev uint64,
	cdnBaseURL string,
) (map[string]*configset_proto.ControllerConfig, error) {
	conf := &cdn_bstore_controller.Config{
		BlockStoreId:      releaseCDNFallbackStoreID,
		SpaceId:           releaseCDNFallbackSpaceID,
		CdnBaseUrl:        cdnBaseURL,
		CacheBlockStoreId: releaseCDNFallbackCacheStoreID,
		BucketIds:         []string{releaseCDNFallbackBucketID},
	}
	entry, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(rev, conf), false)
	if err != nil {
		return nil, err
	}
	return map[string]*configset_proto.ControllerConfig{
		"release-world-cdn-store": entry,
	}, nil
}

func waitReleaseCDNFallbackLookup(
	t *testing.T,
	ctx context.Context,
	b bus.Bus,
	ref *block.BlockRef,
) []byte {
	t.Helper()
	val, _, valRef, err := bus.ExecWaitValue[dex.LookupBlockFromNetworkValue](
		ctx,
		b,
		dex.NewLookupBlockFromNetwork(releaseCDNFallbackBucketID, ref),
		nil,
		nil,
		func(val dex.LookupBlockFromNetworkValue) (bool, error) {
			return val.GetError() == nil && len(val.GetData()) != 0, val.GetError()
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer valRef.Release()
	return val.GetData()
}

type releaseCDNFallbackPack struct {
	id        string
	data      []byte
	bloom     []byte
	blockHash *hash.Hash
}

func buildReleaseCDNFallbackPack(t *testing.T, id string, data []byte) *releaseCDNFallbackPack {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
	if err != nil {
		t.Fatal(err.Error())
	}
	var buf bytes.Buffer
	wrote := false
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if wrote {
			return nil, nil, nil
		}
		wrote = true
		return h, data, nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return &releaseCDNFallbackPack{
		id:        id,
		data:      buf.Bytes(),
		bloom:     result.BloomFilter,
		blockHash: h,
	}
}

func encodeReleaseCDNFallbackPointer(t *testing.T, pack *releaseCDNFallbackPack) []byte {
	t.Helper()
	raw, err := (&cdn.CdnRootPointer{
		SpaceId: releaseCDNFallbackSpaceID,
		Packs: []*packfile.PackfileEntry{{
			Id:          pack.id,
			BloomFilter: pack.bloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(pack.data)),
		}},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	return []byte(packedmsg.EncodePackedMessage(raw))
}

type releaseCDNFallbackServer struct {
	t       *testing.T
	pointer []byte
	pack    *releaseCDNFallbackPack
	ranges  int
}

func newReleaseCDNFallbackServer(
	t *testing.T,
	pointer []byte,
	pack *releaseCDNFallbackPack,
) *releaseCDNFallbackServer {
	t.Helper()
	return &releaseCDNFallbackServer{t: t, pointer: pointer, pack: pack}
}

func (s *releaseCDNFallbackServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/"+releaseCDNFallbackSpaceID+"/root.packedmsg" {
		_, _ = w.Write(s.pointer)
		return
	}
	packPrefix := "/" + releaseCDNFallbackSpaceID + "/packs/"
	if !strings.HasPrefix(r.URL.Path, packPrefix) {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, packPrefix), "/")
	if len(parts) != 2 || strings.TrimSuffix(parts[1], ".kvf") != s.pack.id {
		http.NotFound(w, r)
		return
	}
	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		_, _ = w.Write(s.pack.data)
		return
	}
	s.ranges++
	off, end, err := parseReleaseCDNFallbackRange(rangeHeader, int64(len(s.pack.data)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.Itoa(len(s.pack.data)))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(s.pack.data[off : end+1])
}

func parseReleaseCDNFallbackRange(header string, size int64) (int64, int64, error) {
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return 0, 0, errors.New("unsupported range syntax")
	}
	parts := strings.SplitN(strings.TrimPrefix(header, prefix), "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("malformed range spec")
	}
	off, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, errors.Wrap(err, "parse range start")
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, errors.Wrap(err, "parse range end")
		}
	}
	if end >= size {
		end = size - 1
	}
	if off < 0 || off > end {
		return 0, 0, errors.New("range out of bounds")
	}
	return off, end, nil
}

type releaseCDNFallbackWritebackStore struct {
	block.StoreOps
	putCh chan struct{}
}

func newReleaseCDNFallbackWritebackStore() *releaseCDNFallbackWritebackStore {
	return &releaseCDNFallbackWritebackStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			hash.HashType_HashType_SHA256,
			false,
		),
		putCh: make(chan struct{}, 1),
	}
}

func (s *releaseCDNFallbackWritebackStore) PutBlock(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	s.notifyPut(err)
	return ref, existed, err
}

func (s *releaseCDNFallbackWritebackStore) PutBlockBatch(
	ctx context.Context,
	entries []*block.PutBatchEntry,
) error {
	err := s.StoreOps.PutBlockBatch(ctx, entries)
	s.notifyPut(err)
	return err
}

func (s *releaseCDNFallbackWritebackStore) notifyPut(err error) {
	if err != nil {
		return
	}
	select {
	case s.putCh <- struct{}{}:
	default:
	}
}

func (s *releaseCDNFallbackWritebackStore) waitPut(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.putCh:
		return nil
	}
}

// _ is a type assertion
var _ block.StoreOps = (*releaseCDNFallbackWritebackStore)(nil)
