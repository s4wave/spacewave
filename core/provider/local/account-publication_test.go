package provider_local

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/testbed"
)

func TestProviderAccountPublishesBeforeLinkedCloudDiscovery(t *testing.T) {
	ctx := t.Context()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: ProviderID,
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provCtrlRef.Release()

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, ProviderID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	localProv := prov.(*Provider)
	discoveryStarted := make(chan struct{})
	discoveryRelease := make(chan struct{})
	discoveryDone := make(chan struct{})
	var startedOnce sync.Once
	var doneOnce sync.Once
	localProv.linkedCloudAccountLoader = func(ctx context.Context, _ *ProviderAccount) (string, error) {
		startedOnce.Do(func() { close(discoveryStarted) })
		defer doneOnce.Do(func() { close(discoveryDone) })
		select {
		case <-discoveryRelease:
			return "cloud-account-123", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	type accessResult struct {
		acc *ProviderAccount
		rel func()
		err error
	}
	resultCh := make(chan accessResult, 1)
	go func() {
		accIface, accRel, err := localProv.AccessProviderAccount(ctx, "local-account-123", nil)
		if err != nil {
			resultCh <- accessResult{err: err}
			return
		}
		resultCh <- accessResult{
			acc: accIface.(*ProviderAccount),
			rel: accRel,
		}
	}()

	select {
	case <-discoveryStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("linked-cloud discovery did not start")
	}

	var result accessResult
	select {
	case result = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("provider account access waited for linked-cloud discovery")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	defer result.rel()

	if state := result.acc.accountSettingsCloudSync.GetState(); state != "" {
		t.Fatalf("linked cloud state before discovery release = %q, want empty", state)
	}

	mountCtx, mountCancel := context.WithTimeout(ctx, 5*time.Second)
	defer mountCancel()
	sess, sessRef, err := session.ExMountSession(mountCtx, tb.Bus, &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "local-session-123",
			ProviderAccountId: "local-account-123",
			ProviderId:        ProviderID,
		},
	}, false, nil)
	if err != nil {
		t.Fatalf("mount session while linked-cloud discovery is blocked: %v", err)
	}
	defer sessRef.Release()
	if got := sess.GetSessionRef().GetProviderResourceRef().GetId(); got != "local-session-123" {
		t.Fatalf("mounted session id = %q, want local-session-123", got)
	}

	close(discoveryRelease)
	select {
	case <-discoveryDone:
	case <-time.After(5 * time.Second):
		t.Fatal("linked-cloud discovery did not finish")
	}

	deadline := time.After(5 * time.Second)
	for {
		if state := result.acc.accountSettingsCloudSync.GetState(); state == "cloud-account-123" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf(
				"linked cloud state after discovery release = %q, want %q",
				result.acc.accountSettingsCloudSync.GetState(),
				"cloud-account-123",
			)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
