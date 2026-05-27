package cdn_bstore_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	block_store "github.com/s4wave/spacewave/db/block/store"
	core_test "github.com/s4wave/spacewave/db/core/test"
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
