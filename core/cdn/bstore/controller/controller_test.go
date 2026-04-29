package cdn_bstore_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/testbed"
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
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	conf := NewConfig("release-cdn", "01release", "https://cdn.example.invalid")
	_, _, ctrlRef, err := loader.WaitExecControllerRunning(ctx, tb.Bus, resolver.NewLoadControllerWithConfig(conf), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()

	store, _, storeRef, err := block_store.ExLookupFirstBlockStore(ctx, tb.Bus, "release-cdn", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer storeRef.Release()
	if store.GetID() != "release-cdn" {
		t.Fatalf("store id = %q", store.GetID())
	}
}
