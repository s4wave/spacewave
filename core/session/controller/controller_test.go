package session_controller_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/testbed"
)

// TestRegisterSessionSetsCreatedAt verifies that RegisterSession injects
// created_at when the caller provides zero.
func TestRegisterSessionSetsCreatedAt(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	peerID := tb.Volume.GetPeerID()

	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessCtrlRef.Release()

	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provCtrlRef.Release()

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	localProv := prov.(*provider_local.Provider)
	sessRef, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	sessCtrl, sessCtrlLookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sessCtrlLookupRef.Release()

	// Register with metadata that omits CreatedAt.
	before := time.Now().UnixMilli()
	meta := &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   sessRef.GetProviderResourceRef().GetProviderAccountId(),
	}
	entry, err := sessCtrl.RegisterSession(ctx, sessRef, meta)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().UnixMilli()

	// Verify created_at was injected.
	stored, err := sessCtrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil {
		t.Fatal("expected metadata, got nil")
	}
	if stored.GetCreatedAt() < before || stored.GetCreatedAt() > after {
		t.Fatalf("created_at %d not in range [%d, %d]", stored.GetCreatedAt(), before, after)
	}

	// Register a second session with explicit CreatedAt; verify it is preserved.
	sessRef2, err := localProv.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	explicitTS := int64(1700000000000)
	meta2 := &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   sessRef2.GetProviderResourceRef().GetProviderAccountId(),
		CreatedAt:           explicitTS,
	}
	entry2, err := sessCtrl.RegisterSession(ctx, sessRef2, meta2)
	if err != nil {
		t.Fatal(err)
	}
	stored2, err := sessCtrl.GetSessionMetadata(ctx, entry2.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if stored2.GetCreatedAt() != explicitTS {
		t.Fatalf("expected explicit created_at %d, got %d", explicitTS, stored2.GetCreatedAt())
	}
}
