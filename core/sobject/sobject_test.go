package sobject_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/testbed"
)

// TestSharedObject tests the shared object end to end.
func TestSharedObject(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	le := tb.Logger
	vol := tb.Volume
	peerID := vol.GetPeerID()
	// volumeID := vol.GetID()
	// bucketID := tb.EngineBucketID
	// engineID := tb.EngineID

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
	accountID := "test-account"
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
	sobjectID := "test-shared-object"
	createdSoRef, err := wsProv.CreateSharedObject(ctx, sobjectID, &sobject.SharedObjectMeta{
		BodyType: "test",
	}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = createdSoRef

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

	// Test the operation queue.
	testOp := []byte("mock operation")
	opID, err := so.QueueOperation(ctx, testOp)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Debugf("queued op with id: %v", opID)

	// Wait for the Execute loop to apply the operation to the SOHost.
	soStateCtr, relSoStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relSoStateCtr()

	// Wait for changes
	err = ccontainer.WatchChanges(
		ctx,
		nil,
		soStateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			ops, _, err := snap.GetOpQueue(ctx)
			if err != nil {
				return err
			}
			if len(ops) == 0 {
				// keep waiting
				return nil
			}
			if len(ops) > 1 {
				return errors.Errorf("expected 1 op but got %d", len(ops))
			}
			// done waiting
			return io.EOF
		},
		nil,
	)
	if err != nil && err != io.EOF {
		t.Fatal(err.Error())
	}

	// Check the state.
	stateSnap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check the op queue.
	opQueue, _, err := stateSnap.GetOpQueue(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(opQueue) != 1 {
		t.Fatalf("expected 1 op but got %d", len(opQueue))
	}
	le.Debugf("op %s was queued successfully", opID)

	// process operation (state transition logic)
	processOperationFn := func(
		ctx context.Context,
		snap sobject.SharedObjectStateSnapshot,
		currentStateData []byte,
		ops []*sobject.SOOperationInner,
	) (*[]byte, []*sobject.SOOperationResult, error) {
		nextStateData := currentStateData
		var opResults []*sobject.SOOperationResult

		for _, inner := range ops {
			opData := inner.GetOpData()
			if len(nextStateData) == 0 {
				nextStateData = opData
			} else {
				nextStateData = bytes.Join([][]byte{nextStateData, opData}, []byte(" "))
			}

			opResults = append(opResults, sobject.BuildSOOperationResult(
				inner.GetPeerId(),
				inner.GetNonce(),
				true,
				nil,
			))
		}

		return &nextStateData, opResults, nil
	}

	// Process the operation in another goroutine.
	go func() {
		if err := so.ProcessOperations(ctx, true, processOperationFn); err != nil {
			if ctx.Err() == nil {
				le.WithError(err).Fatal("error processing operations in test case")
			}
		}
	}()

	// Wait for the operation to be applied.
	appliedNonce, _, err := so.WaitOperation(ctx, opID)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Debugf("op applied at nonce: %v", appliedNonce)

	// Check the result
	stateSnap, err = so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	afterRootInner, err := stateSnap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(afterRootInner.GetStateData(), []byte("mock operation")) {
		t.Fatal("unexpected after state data")
	}

	// TODO test op queue, multi-validator, etc.
	_ = so
	le.Debug("shared object local storage tests successful")
}
