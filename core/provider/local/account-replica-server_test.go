package provider_local

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestAccountReplicaStartupAfterTransportStop returns the retained bus's shutdown error.
func TestAccountReplicaStartupAfterTransportStop(t *testing.T) {
	// Run a real session transport until its child bus is ready.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Capture the bus a P2P generation retains at readiness.
	le := logrus.New().WithField("test", t.Name())
	sessionTransport, err := transport.NewSessionTransport(le, tb.Bus, key, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for transport readiness while retaining its execution result.
	transportCtx, stopTransport := context.WithCancel(ctx)
	t.Cleanup(stopTransport)
	done := make(chan error, 1)
	go func() { done <- sessionTransport.Execute(transportCtx) }()
	if err := sessionTransport.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	state := &p2pSyncState{ctx: transportCtx, sessionTransport: sessionTransport, childBus: sessionTransport.GetChildBus()}

	// Complete transport shutdown before the account service tries to attach.
	stopTransport()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if sessionTransport.GetChildBus() != nil {
		t.Fatal("transport retained its child bus after shutdown")
	}
	account := &ProviderAccount{le: le}
	if err := account.startAccountReplicaSync(state); err == nil {
		t.Fatal("account replica service attached after transport shutdown")
	}
}
