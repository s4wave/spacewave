package bucket_lookup

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/sirupsen/logrus"
)

func TestTransformConfEnvelopeRoundTrip(t *testing.T) {
	// Encode a transform configuration in the versioned envelope.
	conf := testTransformConf(t)
	encoded, err := MarshalTransformConf(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < transformConfEnvelopeHeaderSize ||
		!bytes.Equal(encoded[:len(transformConfEnvelopeMagic)], []byte(transformConfEnvelopeMagic)) ||
		encoded[len(transformConfEnvelopeMagic)] != transformConfEnvelopeVersion {
		t.Fatalf("unexpected transform config envelope: %x", encoded)
	}

	// Verify decoding the envelope restores the transform configuration.
	decoded, err := UnmarshalTransformConf(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(conf) {
		t.Fatalf("decoded config mismatch: got %v, want %v", decoded, conf)
	}
}

func TestTransformConfLegacyCRC32RoundTrip(t *testing.T) {
	// Encode a transform configuration with its legacy CRC32 checksum.
	conf := testTransformConf(t)
	payload, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := transform_chksum.EncodeCRC32(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Verify decoding the legacy payload restores the transform configuration.
	decoded, err := UnmarshalTransformConf(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(conf) {
		t.Fatalf("decoded legacy config mismatch: got %v, want %v", decoded, conf)
	}

	// Verify a corrupt CRC32 transform payload is rejected.
	legacy[len(legacy)-1] ^= 0xff
	if _, err := UnmarshalTransformConf(legacy); err == nil {
		t.Fatal("corrupt legacy transform config accepted")
	}
}

func TestTransformConfEnvelopeRejectsUnknownVersion(t *testing.T) {
	encoded, err := MarshalTransformConf(testTransformConf(t))
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(transformConfEnvelopeMagic)]++
	if _, err := UnmarshalTransformConf(encoded); err == nil {
		t.Fatal("unknown transform config envelope version accepted")
	}
}

func TestCursorBucketIDOverridePinsImplicitReferences(t *testing.T) {
	// Pin the cursor's implicit bucket references to the public bucket.
	ctx := context.Background()
	cursor := NewCursor(
		ctx,
		nil,
		nil,
		nil,
		nil,
		nil,
		&bucket.ObjectRef{},
		&bucket.BucketOpArgs{BucketId: "public-cdn", VolumeId: "public-cdn"},
		&block_transform.Config{},
	)
	cursor.SetBucketIDOverride("public-cdn")

	// Verify an authoring bucket reference resolves through the pinned bucket.
	followed, err := cursor.FollowRef(ctx, &bucket.ObjectRef{BucketId: "authoring-world"})
	if err != nil {
		t.Fatal(err)
	}
	defer followed.Release()
	if got := followed.GetOpArgs().GetBucketId(); got != "public-cdn" {
		t.Fatalf("followed bucket = %q, want public-cdn", got)
	}

	// Verify nested authoring references retain the pinned bucket.
	nested, err := followed.FollowRef(ctx, &bucket.ObjectRef{BucketId: "another-authoring-world"})
	if err != nil {
		t.Fatal(err)
	}
	defer nested.Release()
	if got := nested.GetOpArgs().GetBucketId(); got != "public-cdn" {
		t.Fatalf("nested followed bucket = %q, want public-cdn", got)
	}
}

func TestCursorCrossBucketExternalRootClearsSourceTransform(t *testing.T) {
	// Prepare a bounded test context for two distinct bucket stores.
	const (
		sourceBucketID   = "source-world"
		externalBucketID = "spacewave-release"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Register source and external bucket lookups on the controller bus.
	b, handles, sourceOps, externalOps := newTwoBucketBus(t, ctx, sourceBucketID, externalBucketID)

	// Build the source bucket transformer.
	transformConf := testTransformConf(t)
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{},
		transform_all.BuildFactorySet(),
		transformConf,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Store the encoded source bucket root.
	sourceData := []byte("compressed source world root")
	encodedSource, err := xfrm.EncodeBlock(sourceData)
	if err != nil {
		t.Fatal(err)
	}
	sourceRef, _, err := sourceOps.PutBlock(ctx, encodedSource, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Store the raw external bucket root.
	externalData := []byte("untransformed Release World root")
	externalRef, _, err := externalOps.PutBlock(ctx, externalData, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the source cursor decodes its transformed root.
	cursor := NewCursor(
		ctx,
		b,
		logrus.NewEntry(logrus.New()),
		transform_all.BuildFactorySet(),
		NewBucketFromHandle(handles[sourceBucketID]),
		xfrm,
		&bucket.ObjectRef{
			BucketId:      sourceBucketID,
			RootRef:       sourceRef,
			TransformConf: transformConf,
		},
		&bucket.BucketOpArgs{BucketId: sourceBucketID},
		transformConf,
	)
	defer cursor.Release()
	if got, found, err := cursor.GetBlock(ctx, sourceRef); err != nil || !found || !bytes.Equal(got, sourceData) {
		t.Fatalf("compressed source root read found=%v err=%v data=%q", found, err, got)
	}

	// Follow the external root through a read-only bucket lookup.
	external, err := cursor.FollowRefWithOpArgsReadOnly(
		ctx,
		&bucket.ObjectRef{BucketId: externalBucketID, RootRef: externalRef},
		&bucket.BucketOpArgs{BucketId: externalBucketID},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer external.Release()
	if external.GetTransformConf() != nil && !external.GetTransformConf().GetEmpty() {
		t.Fatalf("external root retained source transform: %v", external.GetTransformConf())
	}

	// Verify the external root uses raw bytes without the source transform.
	got, found, err := external.GetBlock(ctx, externalRef)
	if err != nil || !found || !bytes.Equal(got, externalData) {
		t.Fatalf("untransformed external root read found=%v err=%v data=%q", found, err, got)
	}
}

func TestCursorStoreWrappersFollowIntoAnotherBucket(t *testing.T) {
	// Register two buckets and store a root in the second.
	const (
		sourceBucketID   = "source-world"
		externalBucketID = "external-world"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, handles, _, externalOps := newTwoBucketBus(t, ctx, sourceBucketID, externalBucketID)
	externalRef, _, err := externalOps.PutBlock(ctx, []byte("external root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Make the source cursor read-only through a wrapper that records the
	// stores it wraps.
	cursor := NewCursor(
		ctx,
		b,
		logrus.NewEntry(logrus.New()),
		transform_all.BuildFactorySet(),
		NewBucketFromHandle(handles[sourceBucketID]),
		nil,
		&bucket.ObjectRef{BucketId: sourceBucketID},
		&bucket.BucketOpArgs{BucketId: sourceBucketID},
		nil,
	)
	defer cursor.Release()
	var wrapped []block.StoreOps
	err = cursor.WrapTransactionStore(ctx, func(_ context.Context, store block.StoreOps) (block.StoreOps, error) {
		wrapped = append(wrapped, store)
		return block.NewReadOnlyStore(store), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Follow the root into the second bucket.
	external, err := cursor.FollowRefWithOpArgsReadOnly(
		ctx,
		&bucket.ObjectRef{BucketId: externalBucketID, RootRef: externalRef},
		&bucket.BucketOpArgs{BucketId: externalBucketID},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer external.Release()

	// The wrapper applies again to the second bucket's store, so the followed
	// cursor still rejects writes.
	if len(wrapped) != 2 || wrapped[1] != external.GetBucket() {
		t.Fatalf("wrapped stores %v, want the source and then the external bucket", wrapped)
	}
	if _, _, err := external.GetBlockStore().PutBlock(ctx, []byte("write"), nil); !errors.Is(err, tx.ErrNotWrite) {
		t.Fatalf("followed cursor write err = %v, want %v", err, tx.ErrNotWrite)
	}
}

// newTwoBucketBus returns a bus that resolves lookups of two buckets, each
// backed by its own mock store.
func newTwoBucketBus(
	t *testing.T,
	ctx context.Context,
	sourceBucketID, externalBucketID string,
) (bus.Bus, map[string]Handle, block.StoreOps, block.StoreOps) {
	// Start the bus and the stores behind each bucket.
	t.Helper()
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sourceOps := block_mock.NewMockStore(0)
	externalOps := block_mock.NewMockStore(0)

	// Construct distinct source and external bucket configurations.
	sourceConf, err := bucket.NewConfig(sourceBucketID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	externalConf, err := bucket.NewConfig(externalBucketID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	handles := map[string]Handle{
		sourceBucketID: &staticBucketLookupHandle{
			conf:   sourceConf,
			lookup: &staticBucketLookup{store: sourceOps, bucketID: sourceBucketID},
		},
		externalBucketID: &staticBucketLookupHandle{
			conf:   externalConf,
			lookup: &staticBucketLookup{store: externalOps, bucketID: externalBucketID},
		},
	}

	// Resolve bucket lookup directives through the fixture handles.
	handlerRelease, err := b.AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			// Resolve the requested bucket to its fixture handle.
			d, ok := di.GetDirective().(BuildBucketLookup)
			if !ok {
				return nil, nil
			}
			handle := handles[d.BuildBucketLookupBucketID()]
			if handle == nil {
				return nil, nil
			}
			return directive.R(
				directive.NewValueResolver([]BuildBucketLookupValue{handle}),
				nil,
			)
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(handlerRelease)
	return b, handles, sourceOps, externalOps
}

type staticBucketLookupHandle struct {
	conf   *bucket.Config
	lookup Lookup
}

func (h *staticBucketLookupHandle) GetDisposed() bool {
	return false
}

func (h *staticBucketLookupHandle) GetBucketConfig() *bucket.Config {
	return h.conf
}

func (h *staticBucketLookupHandle) GetLookup(context.Context) (Lookup, error) {
	return h.lookup, nil
}

type staticBucketLookup struct {
	store    block.StoreOps
	bucketID string
}

// BeginReadOperation retains the fixture lookup for a bounded read.
func (l *staticBucketLookup) BeginReadOperation(context.Context) (Lookup, func(), error) {
	return l, func() {}, nil
}

func (l *staticBucketLookup) LookupBlock(
	ctx context.Context,
	ref *block.BlockRef,
	_ ...LookupBlockOption,
) ([]byte, bool, error) {
	return l.store.GetBlock(ctx, ref)
}

// LookupStoredBlock reads the block without refs because this test lookup
// keeps no ref graph.
func (l *staticBucketLookup) LookupStoredBlock(ctx context.Context, ref *block.BlockRef, opts ...LookupBlockOption) (*block.StoredBlock, error) {
	data, found, err := l.LookupBlock(ctx, ref, opts...)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

func (l *staticBucketLookup) LookupBlockExistsBatch(
	ctx context.Context,
	refs []*block.BlockRef,
	_ ...LookupBlockOption,
) ([]bool, error) {
	return l.store.GetBlockExistsBatch(ctx, refs)
}

func (l *staticBucketLookup) PutBlock(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	ref, existed, err := l.store.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	return []*bucket.ObjectRef{{BucketId: l.bucketID, RootRef: ref}}, existed, nil
}

func testTransformConf(t *testing.T) *block_transform.Config {
	t.Helper()
	conf, err := block_transform.NewConfig([]config.Config{&transform_s2.Config{Better: true}})
	if err != nil {
		t.Fatal(err)
	}
	return conf
}
