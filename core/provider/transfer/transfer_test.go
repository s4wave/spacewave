package provider_transfer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

type orderTestSource struct {
	list  *sobject.SharedObjectList
	state *sobject.SOState
}

func (s *orderTestSource) GetSharedObjectList(context.Context) (*sobject.SharedObjectList, error) {
	return s.list, nil
}

func (s *orderTestSource) GetSharedObjectState(context.Context, string) (*sobject.SOState, error) {
	return s.state, nil
}

func (s *orderTestSource) GetBlockStore(context.Context, *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	return nil, func() {}, nil
}

func (s *orderTestSource) GetBlockRefs(context.Context, *sobject.SharedObjectRef) ([]*block.BlockRef, error) {
	return nil, nil
}

type orderTestTarget struct {
	calls []string
}

func (t *orderTestTarget) GetBlockStore(context.Context, *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	return nil, func() {}, nil
}

func (t *orderTestTarget) AddSharedObject(_ context.Context, ref *sobject.SharedObjectRef, _ *sobject.SharedObjectMeta) error {
	t.calls = append(t.calls, "add:"+ref.GetProviderResourceRef().GetId())
	return nil
}

func (t *orderTestTarget) WriteSharedObjectState(_ context.Context, sharedObjectID string, _ *sobject.SOState) error {
	t.calls = append(t.calls, "write:"+sharedObjectID)
	return nil
}

// setupLocalProvider starts a local provider on the testbed.
// Returns the provider and release functions.
func setupLocalProvider(ctx context.Context, t *testing.T, tb *testbed.Testbed, providerID string) (*provider_local.Provider, func()) {
	t.Helper()

	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	peerID := tb.Volume.GetPeerID()

	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		t.Fatal(err)
	}

	localProv := prov.(*provider_local.Provider)
	release := func() {
		provRef.Release()
		provCtrlRef.Release()
	}
	return localProv, release
}

// createAccount creates a new account on the local provider.
// Returns the account, account ID, and release function.
func createAccount(ctx context.Context, t *testing.T, prov *provider_local.Provider) (*provider_local.ProviderAccount, string, func()) {
	t.Helper()

	sessRef, err := prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}

	acc := accIface.(*provider_local.ProviderAccount)
	return acc, accountID, accRel
}

// createTestSpace creates a shared object on the account for testing.
func createTestSpace(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, soID string) *sobject.SharedObjectRef {
	t.Helper()
	meta := &sobject.SharedObjectMeta{BodyType: "space"}
	ref, err := acc.CreateSharedObject(ctx, soID, meta, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestLocalTransferSourceReadsSoList verifies that the local transfer source
// can read the shared object list from a provider account.
func TestLocalTransferSourceReadsSoList(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()

	acc, accountID, accRel := createAccount(ctx, t, prov)
	defer accRel()

	// Create two shared objects.
	createTestSpace(ctx, t, acc, "space-a")
	createTestSpace(ctx, t, acc, "space-b")

	// Build the transfer source and read the SO list.
	src := provider_transfer.NewLocalTransferSource(acc, providerID, accountID, tb.Bus)
	soList, err := src.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Expect 3 SOs: one account-private settings SO plus the two spaces.
	// The settings SO remains in the raw list but transfer filters exclude it
	// by policy metadata rather than identifier matching.
	if len(soList.GetSharedObjects()) != 3 {
		t.Fatalf("expected 3 shared objects, got %d", len(soList.GetSharedObjects()))
	}

	// Verify the SO IDs are present.
	ids := make(map[string]bool)
	for _, entry := range soList.GetSharedObjects() {
		ids[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}
	if !ids["space-a"] || !ids["space-b"] {
		t.Fatalf("expected space-a and space-b in list, got %v", ids)
	}
}

func TestTransferAddsTargetSharedObjectBeforeWritingState(t *testing.T) {
	ctx := context.Background()

	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "space-a",
			ProviderId:        "spacewave",
			ProviderAccountId: "target-account",
		},
		BlockStoreId: "space-a",
	}
	src := &orderTestSource{
		list: &sobject.SharedObjectList{
			SharedObjects: []*sobject.SharedObjectListEntry{{
				Ref:  ref,
				Meta: &sobject.SharedObjectMeta{BodyType: "space"},
			}},
		},
		state: &sobject.SOState{
			Root: &sobject.SORoot{InnerSeqno: 1},
		},
	}
	tgt := &orderTestTarget{}

	xfer := provider_transfer.NewTransfer(
		logrus.NewEntry(logrus.StandardLogger()),
		provider_transfer.TransferMode_TransferMode_MERGE,
		src,
		tgt,
		1,
		2,
		nil,
		nil,
		nil,
		nil,
	)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(tgt.calls, ",")
	want := "add:space-a,write:space-a"
	if got != want {
		t.Fatalf("expected call order %q, got %q", want, got)
	}
}

// TestLocalMergeBlockCopy verifies that the transfer copies blocks from the
// source account's block store to the target account's block store.
func TestLocalMergeBlockCopy(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()

	// Create source and target accounts.
	srcAcc, srcAccountID, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()

	// Create an SO on the source.
	soRef := createTestSpace(ctx, t, srcAcc, "test-space")

	// Mount the source block store and write test blocks.
	srcBlockStoreRef := provider_local.NewBlockStoreRef(providerID, srcAccountID, soRef.GetBlockStoreId())
	srcBS, srcBSRel, err := srcAcc.MountBlockStore(ctx, srcBlockStoreRef, nil)
	if err != nil {
		t.Fatal(err)
	}

	testData := [][]byte{
		[]byte("block-data-one"),
		[]byte("block-data-two"),
		[]byte("block-data-three"),
	}
	var blockRefs []string
	for _, data := range testData {
		ref, _, err := srcBS.PutBlock(ctx, data, nil)
		if err != nil {
			srcBSRel()
			t.Fatal(err)
		}
		blockRefs = append(blockRefs, ref.MarshalString())
	}
	srcBSRel()

	// Build source and target.
	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, providerID, srcAccountID, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)

	// Run the transfer.
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, 1, 2, nil, nil, nil, nil)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify the transfer completed.
	state := xfer.GetState()
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("expected COMPLETE phase, got %v", state.GetPhase())
	}
	if len(state.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space, got %d", len(state.GetSpaces()))
	}
	if state.GetSpaces()[0].GetBlocksCopied() != uint64(len(testData)) {
		t.Fatalf("expected %d blocks copied, got %d", len(testData), state.GetSpaces()[0].GetBlocksCopied())
	}

	// Verify blocks exist on the target by reading them.
	tgtBlockStoreRef := provider_local.NewBlockStoreRef(providerID, tgtAccountID, soRef.GetBlockStoreId())
	tgtBS, tgtBSRel, err := tgtAcc.MountBlockStore(ctx, tgtBlockStoreRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtBSRel()

	for i, data := range testData {
		// Re-put the same data to get the same ref and check it exists.
		exists, err := tgtBS.GetBlockExists(ctx, mustParseBlockRef(t, blockRefs[i]))
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("block %d not found on target", i)
		}
		readData, found, err := tgtBS.GetBlock(ctx, mustParseBlockRef(t, blockRefs[i]))
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("block %d not found on target (GetBlock)", i)
		}
		if string(readData) != string(data) {
			t.Fatalf("block %d data mismatch: got %q, want %q", i, readData, data)
		}
	}
}

// TestLocalMergeSoList verifies that after a merge transfer, the target SO list
// contains both the original target SOs and the merged source SOs.
func TestLocalMergeSoList(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()

	srcAcc, srcAccountID, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()

	// Create SOs on source.
	createTestSpace(ctx, t, srcAcc, "src-space-1")
	createTestSpace(ctx, t, srcAcc, "src-space-2")

	// Create an existing SO on target.
	createTestSpace(ctx, t, tgtAcc, "tgt-existing")

	// Run the transfer.
	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, providerID, srcAccountID, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, 1, 2, nil, nil, nil, nil)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// Read target SO list.
	tgtSrc := provider_transfer.NewLocalTransferSource(tgtAcc, providerID, tgtAccountID, tb.Bus)
	tgtList, err := tgtSrc.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 4 SOs: account-settings + tgt-existing + src-space-1 + src-space-2.
	entries := tgtList.GetSharedObjects()
	if len(entries) != 4 {
		t.Fatalf("expected 4 SOs on target, got %d", len(entries))
	}

	ids := make(map[string]bool)
	for _, entry := range entries {
		ids[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}
	for _, expected := range []string{"tgt-existing", "src-space-1", "src-space-2"} {
		if !ids[expected] {
			t.Fatalf("expected %q in target SO list, got %v", expected, ids)
		}
	}
}

// TestLocalMergeResume verifies that a transfer can resume from a checkpoint
// after being interrupted mid-copy.
func TestLocalMergeResume(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()

	srcAcc, srcAccountID, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()

	// Create 3 SOs on source.
	createTestSpace(ctx, t, srcAcc, "space-a")
	createTestSpace(ctx, t, srcAcc, "space-b")
	createTestSpace(ctx, t, srcAcc, "space-c")

	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, providerID, srcAccountID, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)

	// Build a checkpoint store using the source account's object store.
	srcObjStore, srcObjRel, err := buildTestObjectStore(ctx, tb, srcAcc, providerID, srcAccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer srcObjRel()
	cpStore := provider_transfer.NewObjectStoreCheckpoint(srcObjStore)

	// Simulate a partial transfer: save a checkpoint at index 1 (space-a done).
	cp := &provider_transfer.TransferCheckpoint{
		State: &provider_transfer.TransferState{
			Mode:  provider_transfer.TransferMode_TransferMode_MERGE,
			Phase: provider_transfer.TransferPhase_TransferPhase_COPYING_SO,
		},
		SpaceIds:          []string{"space-a", "space-b", "space-c"},
		CurrentSpaceIndex: 1,
	}
	if err := cpStore.SaveCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}

	// Manually add space-a to target (simulating it was done before interrupt).
	createTestSpace(ctx, t, tgtAcc, "space-a")

	// Run the transfer with the checkpoint. It should skip space-a.
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, 1, 2, nil, cpStore, nil, nil)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify the checkpoint was deleted on success.
	loaded, err := cpStore.LoadCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Fatal("expected checkpoint to be deleted after successful transfer")
	}

	// Verify target has all 3 SOs (space-a from manual add + space-b, space-c from resume).
	tgtSrc := provider_transfer.NewLocalTransferSource(tgtAcc, providerID, tgtAccountID, tb.Bus)
	tgtList, err := tgtSrc.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	entries := tgtList.GetSharedObjects()
	if len(entries) != 4 {
		t.Fatalf("expected 4 SOs on target (3 spaces + account-settings), got %d", len(entries))
	}

	ids := make(map[string]bool)
	for _, entry := range entries {
		ids[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}
	for _, expected := range []string{"space-a", "space-b", "space-c"} {
		if !ids[expected] {
			t.Fatalf("expected %q in target SO list, got %v", expected, ids)
		}
	}
}

// TestLocalMergeCleanup verifies that after a merge, the source session's SOs
// are deleted and the volume is removed.
func TestLocalMergeCleanup(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()

	srcAcc, srcAccountID, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()

	// Create an SO on source.
	createTestSpace(ctx, t, srcAcc, "merge-space")

	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, providerID, srcAccountID, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)

	// Run transfer WITH cleanup.
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, 1, 2, src, nil, nil, nil)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify the source SO list has only account-settings (infrastructure, not transferred/cleaned).
	srcList, err := src.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(srcList.GetSharedObjects()) != 1 {
		t.Fatalf("expected 1 SO on source after cleanup (account-settings), got %d", len(srcList.GetSharedObjects()))
	}

	// Verify target has the transferred SO + its own account-settings.
	tgtSrc := provider_transfer.NewLocalTransferSource(tgtAcc, providerID, tgtAccountID, tb.Bus)
	tgtList, err := tgtSrc.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tgtList.GetSharedObjects()) != 2 {
		t.Fatalf("expected 2 SOs on target (merge-space + account-settings), got %d", len(tgtList.GetSharedObjects()))
	}
	ids := make(map[string]bool)
	for _, entry := range tgtList.GetSharedObjects() {
		ids[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}
	if !ids["merge-space"] {
		t.Fatalf("expected merge-space on target, got %v", ids)
	}
}

// buildTestObjectStore builds an object store for testing checkpoint persistence.
func buildTestObjectStore(
	ctx context.Context,
	tb *testbed.Testbed,
	acc *provider_local.ProviderAccount,
	providerID, accountID string,
) (object.ObjectStore, func(), error) {
	objStoreID := provider_local.SobjectObjectStoreID(providerID, accountID)
	volID := acc.GetVolume().GetID()
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, objStoreID, volID, nil)
	if err != nil {
		return nil, nil, err
	}
	return handle.GetObjectStore(), diRef.Release, nil
}

// mustParseBlockRef parses a b58 block ref string.
func mustParseBlockRef(t *testing.T, s string) *block.BlockRef {
	t.Helper()
	ref, err := block.UnmarshalBlockRefB58(s)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
