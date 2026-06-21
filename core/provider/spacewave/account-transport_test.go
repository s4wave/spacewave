package provider_spacewave

import (
	"context"
	"strings"
	"testing"
)

func TestCreateSessionTransportPublishesReadyAndStops(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	priv, _ := generateTestKeypair(t)
	ctx := t.Context()

	if err := acc.CreateSessionTransport(ctx, priv, ""); err != nil {
		t.Fatalf("CreateSessionTransport: %v", err)
	}
	if st := acc.GetSessionTransport(); st == nil {
		t.Fatal("expected ready session transport to be published")
	}
	running, _ := acc.GetTransportSnapshotWithWait()
	if !running {
		t.Fatal("expected transport snapshot to report running")
	}

	acc.StopSessionTransport()
	if st := acc.GetSessionTransport(); st != nil {
		t.Fatal("expected stopped session transport to be cleared")
	}
	running, _ = acc.GetTransportSnapshotWithWait()
	if running {
		t.Fatal("expected stopped transport snapshot to report not running")
	}
}

func TestCreateSessionTransportReplacesExisting(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	ctx := t.Context()

	firstPriv, _ := generateTestKeypair(t)
	if err := acc.CreateSessionTransport(ctx, firstPriv, ""); err != nil {
		t.Fatalf("first CreateSessionTransport: %v", err)
	}
	first := acc.GetSessionTransport()
	if first == nil {
		t.Fatal("expected first session transport")
	}

	secondPriv, _ := generateTestKeypair(t)
	if err := acc.CreateSessionTransport(ctx, secondPriv, ""); err != nil {
		t.Fatalf("second CreateSessionTransport: %v", err)
	}
	second := acc.GetSessionTransport()
	if second == nil {
		t.Fatal("expected second session transport")
	}
	if second == first {
		t.Fatal("expected replacement to publish a new session transport")
	}
	if first.GetChildBus() != nil {
		t.Fatal("expected replaced session transport child bus to be cleared")
	}
}

func TestSessionTransportStateExitBeforeReadyFailsStartup(t *testing.T) {
	sts := &sessionTransportState{}
	sts.setExited(nil)

	err := sts.WaitStarted(context.Background())
	if err == nil {
		t.Fatal("expected startup failure")
	}
	if !strings.Contains(err.Error(), "session transport exited before ready") {
		t.Fatalf("startup error = %v, want exited-before-ready failure", err)
	}
}
