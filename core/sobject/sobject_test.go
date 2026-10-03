package sobject_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/testbed"
)

// TestSharedObject tests the shared object end to end.
func TestSharedObject(t *testing.T) {
	// Start a testbed.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Name the test volume's peer.
	le := tb.Logger
	vol := tb.Volume
	peerID := vol.GetPeerID()

	// Create the provider controller
	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provCtrlRef.Release()

	// Check LookupProvider works.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	provInfo := prov.GetProviderInfo()
	provRef.Release()
	_ = provInfo

	// Acquire a provider account handle.
	accountID := "test-account-" + sobject.NewSOOperationLocalID()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer provAccRef.Release()

	// Get the provider account feature.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the shared object.
	sobjectID := "test-shared-object-" + sobject.NewSOOperationLocalID()
	createdSoRef, err := wsProv.CreateSharedObject(ctx, sobjectID, &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Log the created object.
	tb.Logger.Infof(
		"created shared object with provider %s id %s",
		createdSoRef.GetProviderResourceRef().GetProviderId(),
		createdSoRef.GetProviderResourceRef().GetId(),
	)

	// Mount the shared object.
	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, createdSoRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer soRef.Release()

	// Test the state store
	testStoreID := "test-local-store"
	le.Debug("testing shared object local storage")
	stateStoreRc := sobject.NewLocalStateStoreRefcount(testStoreID, so.AccessLocalStateStore)
	stateStoreRc.SetContext(ctx)
	err = stateStoreRc.Access(ctx, func(ctx context.Context, val kvtx.Store) error {
		// test all
		return kvtx_kvtest.TestAll(ctx, val)
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write an operation and replay it through a processor that joins the data.
	processOperationFn := func(
		_ context.Context,
		_ sobject.SharedObjectStateSnapshot,
		currentStateData []byte,
		ops []*sobject.SOOperationInner,
	) (*[]byte, []*sobject.SOOperationResult, error) {
		nextStateData := currentStateData
		for _, inner := range ops {
			if len(nextStateData) == 0 {
				nextStateData = inner.GetOpData()
			} else {
				nextStateData = bytes.Join([][]byte{nextStateData, inner.GetOpData()}, []byte(" "))
			}
		}
		return &nextStateData, nil, nil
	}
	res, err := sobject.WriteOperation(ctx, so, []byte("mock operation"), processOperationFn)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(res.StateData, []byte("mock operation")) {
		t.Fatalf("unexpected state data: %q", string(res.StateData))
	}

	// The operation set holds exactly the written operation.
	stateSnap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	set, err := stateSnap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if set.Len() != 1 {
		t.Fatalf("expected 1 op but got %d", set.Len())
	}
}
