package provider_local_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
)

// TestStartP2PSyncConfiguresOneForwardHop proves local-provider startup gives
// each shared-object DEX controller the immediate-relay forwarding budget.
func TestStartP2PSyncConfiguresOneForwardHop(t *testing.T) {
	// Bound the test and release its timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	// Start a local account session and its session transport.
	_, _, account, session, release := setupProviderAndSession(ctx, t)
	t.Cleanup(release)
	if err := account.CreateSessionTransport(ctx, session.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(account.StopSessionTransport)

	// Observe the exact DEX configuration submitted by provider startup.
	configs := make(chan *dex_solicit.Config, 1)
	removeHandler, err := account.GetSessionTransport().GetChildBus().AddHandler(directive.NewFuncHandler(
		func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
			// Capture the DEX controller config submitted on the session child bus.
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			config, ok := load.GetLoadControllerConfig().(*dex_solicit.Config)
			if !ok {
				return nil, nil
			}
			select {
			case configs <- config.CloneVT():
			default:
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeHandler)

	// Start P2P sync and require its first DEX controller to allow one relay.
	if err := account.StartP2PSync(ctx, account.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(account.StopP2PSync)
	select {
	case config := <-configs:
		if config.GetMaxForwardHops() != 1 {
			t.Fatalf("DEX max forward hops = %d, want 1", config.GetMaxForwardHops())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for DEX controller configuration")
	}
}
