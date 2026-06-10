package block_transform_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
)

func TestTransformerDecodedBlockCacheTransformKeyStable(t *testing.T) {
	first := newTestTransformer(t, &transform_chksum.Config{}, &transform_s2.Config{Better: true})
	second := newTestTransformer(t, &transform_chksum.Config{}, &transform_s2.Config{Better: true})

	var keyer block.DecodedBlockCacheTransformer = first
	key := keyer.DecodedBlockCacheTransformKey()
	if key == "" || key == block.DecodedBlockCacheNoTransformKey {
		t.Fatalf("transform key = %q, want production transform key", key)
	}
	if second.DecodedBlockCacheTransformKey() != key {
		t.Fatalf("stable transform key mismatch: %q != %q", second.DecodedBlockCacheTransformKey(), key)
	}
}

func TestGzipEncodeDecode(t *testing.T) {
	xfrm := newTestTransformer(t, &transform_gzip.Config{})
	input := []byte("gzip block transform round trip")

	encoded, err := xfrm.EncodeBlock(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := xfrm.DecodeBlock(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, input) {
		t.Fatalf("decoded gzip block = %q, want %q", decoded, input)
	}
}

func TestTransformerDecodedBlockCacheTransformKeySeparatesConfigs(t *testing.T) {
	base := newTestTransformer(t, &transform_s2.Config{})
	better := newTestTransformer(t, &transform_s2.Config{Better: true})
	ordered := newTestTransformer(t, &transform_chksum.Config{}, &transform_s2.Config{})

	if base.DecodedBlockCacheTransformKey() == better.DecodedBlockCacheTransformKey() {
		t.Fatalf("different s2 configs collided on key %q", base.DecodedBlockCacheTransformKey())
	}
	if base.DecodedBlockCacheTransformKey() == ordered.DecodedBlockCacheTransformKey() {
		t.Fatalf("different ordered transform configs collided on key %q", base.DecodedBlockCacheTransformKey())
	}
}

func TestTransformerDecodedBlockCacheTransformKeyNoTransformAndStepOnlyBypass(t *testing.T) {
	emptyConf := newTestTransformer(t)
	if got := emptyConf.DecodedBlockCacheTransformKey(); got != block.DecodedBlockCacheNoTransformKey {
		t.Fatalf("empty config key = %q, want %q", got, block.DecodedBlockCacheNoTransformKey)
	}

	emptySteps := block_transform.NewTransformerWithSteps(nil)
	if got := emptySteps.DecodedBlockCacheTransformKey(); got != block.DecodedBlockCacheNoTransformKey {
		t.Fatalf("empty step transformer key = %q, want %q", got, block.DecodedBlockCacheNoTransformKey)
	}

	stepOnly := block_transform.NewTransformerWithSteps([]block_transform.Step{passthroughStep{}})
	if got := stepOnly.DecodedBlockCacheTransformKey(); got != "" {
		t.Fatalf("manual step-only transformer key = %q, want empty bypass key", got)
	}
}

func TestCursorFetchReturnsDecodedProductionTransformedBytes(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	xfrm := newTestTransformer(t, &transform_s2.Config{})

	tx, writeCursor := block.NewTransaction(store, xfrm, nil, nil)
	writeCursor.SetBlock(&block_mock.Example{Msg: "fetch-transform"}, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	_, cursor := block.NewTransaction(store, newTestTransformer(t, &transform_s2.Config{}), ref, nil)
	data, found, err := cursor.Fetch(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected transformed block data to be found")
	}
	if err := ref.VerifyData(data, false); err == nil {
		t.Fatal("Fetch returned encoded bytes that still verify against the transformed block ref")
	}
	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(data); err != nil {
		t.Fatal(err.Error())
	}
	if got := example.GetMsg(); got != "fetch-transform" {
		t.Fatalf("fetched decoded message = %q, want fetch-transform", got)
	}
}

func TestDecodedBlockCacheReusesProductionTransformedReads(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	writeXfrm := newTestTransformer(t, &transform_s2.Config{})

	tx, writeCursor := block.NewTransaction(store, writeXfrm, nil, nil)
	writeCursor.SetBlock(&block_mock.Example{Msg: "production-transform"}, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	firstCtx = block.WithDecodedBlockCache(firstCtx, decodedBlocks)
	_, firstCursor := block.NewTransaction(store, newTestTransformer(t, &transform_s2.Config{}), ref, nil)
	first, err := firstCursor.Unmarshal(firstCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	first.(*block_mock.Example).Msg = "mutated"
	decodedBlocks.Wait()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	secondCtx = block.WithDecodedBlockCache(secondCtx, decodedBlocks)
	_, secondCursor := block.NewTransaction(store, newTestTransformer(t, &transform_s2.Config{}), ref, nil)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "production-transform" {
		t.Fatalf("cached transformed clone msg = %q, want production-transform", got)
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockUnmarshalCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 ||
		firstSnapshot.DecodedBlockStoreAttemptCount != 1 ||
		firstSnapshot.DecodedBlockStoreAcceptedCount != 1 ||
		firstSnapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected first transformed cache counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 0 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 0 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 1 ||
		secondSnapshot.DecodedBlockCloneCount != 1 ||
		secondSnapshot.DecodedBlockUncacheableCount != 0 {
		t.Fatalf("unexpected second transformed cache counters: %+v", secondSnapshot)
	}
}

func TestDecodedBlockCacheRejectsTransformedRefVerificationMismatch(t *testing.T) {
	ctx := context.Background()
	baseStore := block_mock.NewMockStore(0)
	writeXfrm := newTestTransformer(t, &transform_s2.Config{})

	tx, writeCursor := block.NewTransaction(baseStore, writeXfrm, nil, nil)
	writeCursor.SetBlock(&block_mock.Example{Msg: "transformed-original"}, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	poison, err := (&block_mock.Example{Msg: "transformed-poison"}).MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	poison, err = writeXfrm.EncodeBlock(poison)
	if err != nil {
		t.Fatal(err.Error())
	}
	store := &transformCorruptingStore{StoreOps: baseStore, data: poison}
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	opCtx, counter := block.WithReadCounter(ctx)
	opCtx = block.WithDecodedBlockCache(opCtx, decodedBlocks)
	for range 2 {
		_, cursor := block.NewTransaction(store, newTestTransformer(t, &transform_s2.Config{}), ref, nil)
		blk, err := cursor.Unmarshal(opCtx, block_mock.NewExampleBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got := blk.(*block_mock.Example).GetMsg(); got != "transformed-poison" {
			t.Fatalf("decoded message = %q, want transformed-poison", got)
		}
		decodedBlocks.Wait()
	}

	snapshot := counter.Snapshot()
	if snapshot.BlockReadCount != 2 ||
		snapshot.DecodedBlockUnmarshalCount != 2 ||
		snapshot.DecodedBlockCacheAttemptCount != 2 ||
		snapshot.DecodedBlockCacheMissCount != 2 ||
		snapshot.DecodedBlockCacheHitCount != 0 ||
		snapshot.DecodedBlockUncacheableCount != 2 {
		t.Fatalf("transformed verification mismatch should bypass shared cache: %+v", snapshot)
	}
}

func TestDecodedBlockCacheSeparatesProductionTransformConfigs(t *testing.T) {
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	writeXfrm := newTestTransformer(t, &transform_s2.Config{})

	tx, writeCursor := block.NewTransaction(store, writeXfrm, nil, nil)
	writeCursor.SetBlock(&block_mock.Example{Msg: "transform-config-boundary"}, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	firstCtx, firstCounter := block.WithReadCounter(ctx)
	firstCtx = block.WithDecodedBlockCache(firstCtx, decodedBlocks)
	_, firstCursor := block.NewTransaction(store, newTestTransformer(t, &transform_s2.Config{}), ref, nil)
	if _, err := firstCursor.Unmarshal(firstCtx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	secondCtx, secondCounter := block.WithReadCounter(ctx)
	secondCtx = block.WithDecodedBlockCache(secondCtx, decodedBlocks)
	_, secondCursor := block.NewTransaction(store, newTestTransformer(t, &transform_s2.Config{Better: true}), ref, nil)
	second, err := secondCursor.Unmarshal(secondCtx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := second.(*block_mock.Example).GetMsg(); got != "transform-config-boundary" {
		t.Fatalf("decoded second config message = %q, want transform-config-boundary", got)
	}

	firstSnapshot := firstCounter.Snapshot()
	if firstSnapshot.BlockReadCount != 1 ||
		firstSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		firstSnapshot.DecodedBlockCacheMissCount != 1 {
		t.Fatalf("unexpected first transform config counters: %+v", firstSnapshot)
	}
	secondSnapshot := secondCounter.Snapshot()
	if secondSnapshot.BlockReadCount != 1 ||
		secondSnapshot.DecodedBlockUnmarshalCount != 1 ||
		secondSnapshot.DecodedBlockCacheAttemptCount != 1 ||
		secondSnapshot.DecodedBlockCacheMissCount != 1 ||
		secondSnapshot.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("different transform config should not hit shared cache: %+v", secondSnapshot)
	}
}

func newTestTransformer(t *testing.T, steps ...config.Config) *block_transform.Transformer {
	t.Helper()
	conf, err := block_transform.NewConfig(steps)
	if err != nil {
		t.Fatal(err.Error())
	}
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_chksum.NewStepFactory())
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	sfs.AddStepFactory(transform_s2.NewStepFactory())
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, sfs, conf)
	if err != nil {
		t.Fatal(err.Error())
	}
	return xfrm
}

type passthroughStep struct{}

func (passthroughStep) EncodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

func (passthroughStep) DecodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

type transformCorruptingStore struct {
	block.StoreOps
	data []byte
}

func (s *transformCorruptingStore) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return bytes.Clone(s.data), true, nil
}
