package bucket_lookup

import (
	"bytes"
	"context"
	"testing"
	"time"

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
	"github.com/sirupsen/logrus"
)

func TestTransformConfEnvelopeRoundTrip(t *testing.T) {
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

	decoded, err := UnmarshalTransformConf(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(conf) {
		t.Fatalf("decoded config mismatch: got %v, want %v", decoded, conf)
	}
}

func TestTransformConfLegacyCRC32RoundTrip(t *testing.T) {
	conf := testTransformConf(t)
	payload, err := conf.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := transform_chksum.EncodeCRC32(payload)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := UnmarshalTransformConf(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.EqualVT(conf) {
		t.Fatalf("decoded legacy config mismatch: got %v, want %v", decoded, conf)
	}

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

	followed, err := cursor.FollowRef(ctx, &bucket.ObjectRef{BucketId: "authoring-world"})
	if err != nil {
		t.Fatal(err)
	}
	defer followed.Release()
	if got := followed.GetOpArgs().GetBucketId(); got != "public-cdn" {
		t.Fatalf("followed bucket = %q, want public-cdn", got)
	}

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
	const (
		sourceBucketID   = "source-world"
		externalBucketID = "spacewave-release"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sourceOps := block_mock.NewMockStore(0)
	externalOps := block_mock.NewMockStore(0)
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
	handlerRelease, err := b.AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
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
	defer handlerRelease()

	transformConf := testTransformConf(t)
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{},
		transform_all.BuildFactorySet(),
		transformConf,
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceData := []byte("compressed source world root")
	encodedSource, err := xfrm.EncodeBlock(sourceData)
	if err != nil {
		t.Fatal(err)
	}
	sourceRef, _, err := sourceOps.PutBlock(ctx, encodedSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	externalData := []byte("untransformed Release World root")
	externalRef, _, err := externalOps.PutBlock(ctx, externalData, nil)
	if err != nil {
		t.Fatal(err)
	}

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
	got, found, err := external.GetBlock(ctx, externalRef)
	if err != nil || !found || !bytes.Equal(got, externalData) {
		t.Fatalf("untransformed external root read found=%v err=%v data=%q", found, err, got)
	}
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

func (l *staticBucketLookup) LookupBlock(
	ctx context.Context,
	ref *block.BlockRef,
	_ ...LookupBlockOption,
) ([]byte, bool, error) {
	return l.store.GetBlock(ctx, ref)
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
