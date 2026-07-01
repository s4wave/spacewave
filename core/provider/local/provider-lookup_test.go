package provider_local

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bldr_core "github.com/s4wave/spacewave/bldr/core"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/sirupsen/logrus"
)

func TestLookupLocalProviderDoesNotWaitForPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	b, sr, err := bldr_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sr.AddFactory(NewFactory(b))

	_, provCtrlRef, err := b.AddDirective(resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: ProviderID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provCtrlRef.Release()

	prov, provRef, err := provider.ExLookupProvider(ctx, b, ProviderID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()
	if prov == nil {
		t.Fatal("local provider lookup returned nil")
	}
}
