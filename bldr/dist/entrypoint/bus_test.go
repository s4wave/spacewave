package dist_entrypoint

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cdn_bstore_controller "github.com/s4wave/spacewave/core/cdn/bstore/controller"
	"github.com/sirupsen/logrus"
)

func TestIsWebDistPlatform(t *testing.T) {
	for _, tt := range []struct {
		platformID string
		want       bool
	}{
		{platformID: "js", want: true},
		{platformID: "web/js/wasm", want: true},
		{platformID: "desktop/js/wasm", want: true},
		{platformID: "desktop/darwin/arm64", want: false},
		{platformID: "linux/amd64", want: false},
	} {
		if got := isWebDistPlatform(tt.platformID); got != tt.want {
			t.Fatalf("isWebDistPlatform(%q) = %v, want %v", tt.platformID, got, tt.want)
		}
	}
}

func TestNewCoreBusResolvesReleaseWorldCdnBlockStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b, _, err := NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}

	_, _, ref, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(cdn_bstore_controller.NewConfig(
			"spacewave-release-cdn",
			"01launcherreleaseworld000000",
			"https://cdn.example.invalid",
		)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ref.Release()
}
