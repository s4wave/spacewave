package provider_transfer_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_engine "github.com/s4wave/spacewave/core/space/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/object"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// orderTestSource lists list and replays an empty World. A cloud source
// does not leave its shared Spaces to their other members.
type orderTestSource struct {
	list  *sobject.SharedObjectList
	cloud bool
}

func (s *orderTestSource) GetSharedObjectList(context.Context) (*sobject.SharedObjectList, error) {
	return s.list, nil
}

func (s *orderTestSource) ReplaySharedObject(context.Context, *logrus.Entry, *sobject.SharedObjectRef) (*space_world.InnerState, func(), error) {
	return &space_world.InnerState{}, func() {}, nil
}

func (s *orderTestSource) GetBlockStore(context.Context, *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	return nil, func() {}, nil
}

func (s *orderTestSource) GetOrigin() *provider_transfer.TransferOrigin {
	return &provider_transfer.TransferOrigin{SourceProviderId: "order", SourceAccountId: "source"}
}

func (s *orderTestSource) LeavesSharedObjects() bool {
	return !s.cloud
}

// orderTestTarget records its calls and reads holds under every id.
type orderTestTarget struct {
	calls []string
	holds provider_transfer.TargetHolds
}

func (t *orderTestTarget) GetBlockStore(context.Context, *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	return nil, func() {}, nil
}

func (t *orderTestTarget) ReadSharedObject(context.Context, *logrus.Entry, string, *provider_transfer.TransferOrigin) (*provider_transfer.TargetSpace, error) {
	return &provider_transfer.TargetSpace{Holds: t.holds}, nil
}

func (t *orderTestTarget) WaitAuthor(context.Context, string, string, uint64) (bool, error) {
	return false, nil
}

func (t *orderTestTarget) AddSharedObject(_ context.Context, ref *sobject.SharedObjectRef, _ *sobject.SharedObjectMeta, _ *provider_transfer.TransferOrigin) (*provider_transfer.TargetSpace, error) {
	t.calls = append(t.calls, "add:"+ref.GetProviderResourceRef().GetId())
	return &provider_transfer.TargetSpace{Holds: provider_transfer.TargetHoldsArriving}, nil
}

func (t *orderTestTarget) WriteSharedObjectState(_ context.Context, _ *logrus.Entry, sharedObjectID string, _ crypto.PrivKey, _ []byte, _ *provider_transfer.TransferOrigin, _ *provider_transfer.TargetSpace) error {
	t.calls = append(t.calls, "write:"+sharedObjectID)
	return nil
}

func TestTransferWatchStatePairsSnapshotAndWaitChannel(t *testing.T) {
	// Start an idle transfer.
	xfer := provider_transfer.NewTransfer(
		nil,
		provider_transfer.TransferMode_TransferMode_MERGE,
		nil,
		nil,
		nil,
		1,
		2,
		nil,
	)

	// Failing it closes the watch channel.
	state, ch := xfer.WatchState()
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_IDLE {
		t.Fatalf("initial phase = %s, want IDLE", state.GetPhase())
	}
	if err := xfer.Fail(errors.New("stop")); err == nil {
		t.Fatal("expected failure error")
	}
	select {
	case <-ch:
	default:
		t.Fatal("watch channel did not close after state change")
	}

	// A new watch reports the failure.
	state, _ = xfer.WatchState()
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_FAILED {
		t.Fatalf("final phase = %s, want FAILED", state.GetPhase())
	}
	if state.GetErrorMessage() != "stop" {
		t.Fatalf("error message = %q, want stop", state.GetErrorMessage())
	}
}

// setupLocalProvider starts a local provider on the testbed.
// Returns the provider and release functions.
func setupLocalProvider(ctx context.Context, t *testing.T, tb *testbed.Testbed, providerID string) (*provider_local.Provider, func()) {
	// Mark test helper failures at the caller.
	t.Helper()

	// Register the local provider factory and peer.
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	peerID := tb.Volume.GetPeerID()

	// Start the local provider controller with its configuration.
	_, provCtrlRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the running local provider.
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		t.Fatal(err)
	}

	// Retain the provider and controller release functions.
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
	// Mark test helper failures at the caller.
	t.Helper()

	// Create a local account and session.
	sessRef, err := prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the account and retain its release function.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Return the typed local account and its release function.
	acc := accIface.(*provider_local.ProviderAccount)
	return acc, accountID, accRel
}

// createTestSpace creates a shared object on the account for testing.
func createTestSpace(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, soID string) *sobject.SharedObjectRef {
	// Mark test helper failures at the caller and create the shared object.
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
	// Start a testbed with a local provider.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()

	// Create an account on it.
	acc, _, accRel := createAccount(ctx, t, prov)
	defer accRel()

	// Create two shared objects.
	createTestSpace(ctx, t, acc, "space-a")
	createTestSpace(ctx, t, acc, "space-b")

	// Build the transfer source and read the SO list.
	src := provider_transfer.NewLocalTransferSource(acc, tb.Bus)
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
	// Transfer one Space to a target that records its calls.
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
	}
	tgt := &orderTestTarget{}
	xfer := provider_transfer.NewTransfer(
		logrus.NewEntry(logrus.StandardLogger()),
		provider_transfer.TransferMode_TransferMode_MERGE,
		src,
		tgt,
		nil,
		1,
		2,
		nil,
	)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// The target adds the Space before writing its state.
	got := strings.Join(tgt.calls, ",")
	want := "add:space-a,write:space-a"
	if got != want {
		t.Fatalf("expected call order %q, got %q", want, got)
	}
}

// storeTestSource replays a World whose root is ref, served from store.
type storeTestSource struct {
	orderTestSource
	store block.StoreOps
	ref   *block.BlockRef
}

func (s *storeTestSource) GetBlockStore(context.Context, *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	return s.store, func() {}, nil
}

func (s *storeTestSource) ReplaySharedObject(context.Context, *logrus.Entry, *sobject.SharedObjectRef) (*space_world.InnerState, func(), error) {
	return &space_world.InnerState{HeadRef: &bucket.ObjectRef{RootRef: s.ref}}, func() {}, nil
}

// storeTestTarget writes blocks to store.
type storeTestTarget struct {
	orderTestTarget
	store block.StoreOps
}

func (t *storeTestTarget) GetBlockStore(context.Context, *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	return t.store, func() {}, nil
}

// TestTransferSharedSpaces checks each row of the shared Space table: a
// shared Space counts as moved only when the target already lists it as
// shared and the source leaves it to its other members, and the transfer
// never adds or writes one.
func TestTransferSharedSpaces(t *testing.T) {
	ref := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{Id: "space-a", ProviderId: "local", ProviderAccountId: "source-account"},
		BlockStoreId:        "space-a",
	}
	for _, tc := range []struct {
		name  string
		cloud bool
		holds provider_transfer.TargetHolds
		want  provider_transfer.TransferPhase
	}{
		{"target not a member", false, provider_transfer.TargetHoldsNothing, provider_transfer.TransferPhase_TransferPhase_NOT_MOVED},
		{"target a member, local source", false, provider_transfer.TargetHoldsShared, provider_transfer.TransferPhase_TransferPhase_COMPLETE},
		{"target a member, cloud source", true, provider_transfer.TargetHoldsShared, provider_transfer.TransferPhase_TransferPhase_NOT_MOVED},
		{"target holds its own Space", false, provider_transfer.TargetHoldsOther, provider_transfer.TransferPhase_TransferPhase_NOT_MOVED},
		{"target holds a moved Space", false, provider_transfer.TargetHoldsWritten, provider_transfer.TransferPhase_TransferPhase_NOT_MOVED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := &orderTestSource{
				list: &sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{{
					Ref:    ref,
					Meta:   &sobject.SharedObjectMeta{BodyType: "space"},
					Source: "shared",
				}}},
				cloud: tc.cloud,
			}
			tgt := &orderTestTarget{holds: tc.holds}
			xfer := provider_transfer.NewTransfer(logrus.NewEntry(logrus.StandardLogger()), provider_transfer.TransferMode_TransferMode_MIRROR, src, tgt, nil, 1, 2, nil)
			if err := xfer.Execute(t.Context()); err != nil {
				t.Fatal(err)
			}
			state := xfer.GetState()
			if state.GetPhase() != tc.want {
				t.Fatalf("phase = %s, want %s", state.GetPhase(), tc.want)
			}
			space := state.GetSpaces()[0]
			if space.GetPhase() != tc.want || (tc.want == provider_transfer.TransferPhase_TransferPhase_NOT_MOVED) != (space.GetNotMovedReason() != "") {
				t.Fatalf("space = %s %q, want %s with a reason only when not moved", space.GetPhase(), space.GetNotMovedReason(), tc.want)
			}
			if len(tgt.calls) != 0 {
				t.Fatalf("target calls %v, want none for a shared Space", tgt.calls)
			}
		})
	}
}

// TestTransferRejectsUnknownRefs checks that a block the source holds without
// its refs fails the transfer naming the block, instead of reaching the target
// as a leaf.
func TestTransferRejectsUnknownRefs(t *testing.T) {
	// Serve a block from a store that retains only its bytes.
	ctx := context.Background()
	newStore := func() block.StoreOps {
		return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	}
	source := newStore()
	ref, _, err := source.PutBlock(ctx, []byte("untracked"), nil)
	if err != nil {
		t.Fatal(err)
	}
	soRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{Id: "space-a", ProviderId: "spacewave"},
		BlockStoreId:        "space-a",
	}
	src := &storeTestSource{
		orderTestSource: orderTestSource{list: &sobject.SharedObjectList{
			SharedObjects: []*sobject.SharedObjectListEntry{{Ref: soRef, Meta: &sobject.SharedObjectMeta{BodyType: "space"}}},
		}},
		store: source,
		ref:   ref,
	}
	target := newStore()
	tgt := &storeTestTarget{store: target}

	// The transfer fails before writing the block.
	xfer := provider_transfer.NewTransfer(
		logrus.NewEntry(logrus.StandardLogger()),
		provider_transfer.TransferMode_TransferMode_MERGE,
		src,
		tgt,
		nil,
		1,
		2,
		nil,
	)
	err = xfer.Execute(ctx)
	if !errors.Is(err, block.ErrRefsUnknown) || !strings.Contains(err.Error(), ref.MarshalString()) {
		t.Fatalf("transfer = %v, want ErrRefsUnknown naming %s", err, ref.MarshalString())
	}
	found, err := target.GetBlockExists(ctx, ref)
	if err != nil || found {
		t.Fatalf("target = %v/%v, want absent", found, err)
	}
}

// TestLocalMergeBlockCopy verifies that the transfer copies blocks from the
// source account's block store to the target account's block store.
func TestLocalMergeBlockCopy(t *testing.T) {
	// Start a testbed with a local provider.
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

	// Write three test blocks, each held by a root of the store, and replay a
	// World whose head is the first and which retains the other two.
	testData := [][]byte{
		[]byte("block-data-one"),
		[]byte("block-data-two"),
		[]byte("block-data-three"),
	}
	var blockRefs []string
	var entries []*block.PutBatchEntry
	var roots []block.NamedRoot
	world := &space_world.InnerState{}
	for i, data := range testData {
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			srcBSRel()
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		roots = append(roots, block.NamedRoot{Name: fmt.Sprintf("test/%d", i), Ref: ref})
		blockRefs = append(blockRefs, ref.MarshalString())
		if i == 0 {
			world.HeadRef = &bucket.ObjectRef{RootRef: ref}
		} else {
			world.RetainedRoots = append(world.RetainedRoots, &space_world.RetainedRoot{Name: fmt.Sprintf("test/%d", i), RootRef: ref})
		}
	}
	if err := srcBS.SetRoots(ctx, entries, roots); err != nil {
		srcBSRel()
		t.Fatal(err)
	}
	srcBSRel()

	// Build source and target.
	le := logrus.NewEntry(logrus.StandardLogger())
	src := &worldTestSource{LocalTransferSource: provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus), world: world}
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)

	// Run the transfer.
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, nil)
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

	// Each block reads back from the target with its data.
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
	// Start a testbed with a local provider.
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
	srcAcc, _, srcRel := createAccount(ctx, t, prov)
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
	src := provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, nil)
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// Read target SO list.
	tgtSrc := provider_transfer.NewLocalTransferSource(tgtAcc, tb.Bus)
	tgtList, err := tgtSrc.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 4 SOs: account-settings + tgt-existing + src-space-1 + src-space-2.
	entries := tgtList.GetSharedObjects()
	if len(entries) != 4 {
		t.Fatalf("expected 4 SOs on target, got %d", len(entries))
	}

	// Both the merged and the existing Spaces are present.
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

// TestLocalMergeLeavesSpaceCreatedOnTarget checks that a Merge into a target
// that created a Space under the same id moves the other Spaces, leaves that
// one not moved with a reason, keeps the target's Space, and deletes nothing
// from the source.
func TestLocalMergeLeavesSpaceCreatedOnTarget(t *testing.T) {
	// Start a source with three Spaces and a target that created one of them.
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
	for _, id := range []string{"space-a", "space-b", "space-c"} {
		createTestSpace(ctx, t, srcAcc, id)
	}
	createTestSpace(ctx, t, tgtAcc, "space-a")
	tgtStore, tgtStoreRel, err := buildTestObjectStore(ctx, tb, tgtAcc, providerID, tgtAccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtStoreRel()
	created := targetSpaceState(ctx, t, tgtStore, "space-a", nil)

	// Merge with a checkpoint on the source.
	srcObjStore, srcObjRel, err := buildTestObjectStore(ctx, tb, srcAcc, providerID, srcAccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer srcObjRel()
	cpStore := provider_transfer.NewObjectStoreCheckpoint(srcObjStore)
	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{Checkpoint: cpStore, Cleanup: src})
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// The run finished with space-a not moved and the others moved, and left
	// nothing to resume.
	state := xfer.GetState()
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_NOT_MOVED {
		t.Fatalf("phase = %s, want NOT_MOVED", state.GetPhase())
	}
	for _, space := range state.GetSpaces() {
		want := provider_transfer.TransferPhase_TransferPhase_COMPLETE
		if space.GetSharedObjectId() == "space-a" {
			want = provider_transfer.TransferPhase_TransferPhase_NOT_MOVED
			if space.GetNotMovedReason() == "" {
				t.Fatal("space-a not moved without a reason")
			}
		}
		if space.GetPhase() != want {
			t.Fatalf("%s phase = %s, want %s", space.GetSharedObjectId(), space.GetPhase(), want)
		}
	}
	if loaded, err := cpStore.LoadCheckpoint(ctx); err != nil || loaded != nil {
		t.Fatalf("checkpoint after the run = %v, %v; want none", loaded, err)
	}

	// The target keeps its own space-a, and the source keeps every Space.
	if got := targetSpaceState(ctx, t, tgtStore, "space-a", nil); !bytes.Equal(got, created) {
		t.Fatal("the Merge replaced the Space the target created")
	}
	srcList, err := src.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool)
	for _, entry := range srcList.GetSharedObjects() {
		ids[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}
	for _, id := range []string{"space-a", "space-b", "space-c"} {
		if !ids[id] {
			t.Fatalf("the Merge deleted %s from the source", id)
		}
	}
}

// TestLocalMergeCleanup verifies that Merge and Migrate keep the source when
// they cannot move all of it, and that a Merge that moves every Space deletes
// the source SOs. The account delete that follows is the session package's.
func TestLocalMergeCleanup(t *testing.T) {
	// Start a testbed with a local provider.
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
	srcAcc, _, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()

	// Create two SOs on source.
	createTestSpace(ctx, t, srcAcc, "merge-space")
	createTestSpace(ctx, t, srcAcc, "kept-space")

	// Build the source and target.
	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	expectSourceSOs := func(want int) {
		t.Helper()
		srcList, err := src.GetSharedObjectList(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(srcList.GetSharedObjects()); got != want {
			t.Fatalf("expected %d SOs on source, got %d", want, got)
		}
	}

	// A Merge that leaves a Space out refuses before copying or deleting.
	partial := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{
		Cleanup:        src,
		FilterSpaceIDs: []string{"merge-space"},
	})
	if err := partial.Execute(ctx); err == nil {
		t.Fatal("expected a Merge that leaves a Space out to fail")
	}
	expectSourceSOs(3)

	// A Migrate whose identity copy fails keeps the source.
	migrate := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MIGRATE, src, tgt, ownerKey(ctx, t, srcAcc), 1, 2, &provider_transfer.TransferOptions{
		Cleanup: src,
		CopyIdentity: func(context.Context) error {
			return errors.New("identity copy failed")
		},
	})
	if err := migrate.Execute(ctx); err == nil {
		t.Fatal("expected Migrate to fail when the identity copy fails")
	}
	expectSourceSOs(3)

	// Run transfer WITH cleanup.
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{Cleanup: src})
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
	tgtSrc := provider_transfer.NewLocalTransferSource(tgtAcc, tb.Bus)
	tgtList, err := tgtSrc.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tgtList.GetSharedObjects()) != 3 {
		t.Fatalf("expected 3 SOs on target (two Spaces + account-settings), got %d", len(tgtList.GetSharedObjects()))
	}
	ids := make(map[string]bool)
	for _, entry := range tgtList.GetSharedObjects() {
		ids[entry.GetRef().GetProviderResourceRef().GetId()] = true
	}
	if !ids["merge-space"] || !ids["kept-space"] {
		t.Fatalf("expected both Spaces on target, got %v", ids)
	}
}

// buildTestObjectStore builds an object store for testing checkpoint persistence.
func buildTestObjectStore(
	ctx context.Context,
	tb *testbed.Testbed,
	acc *provider_local.ProviderAccount,
	providerID, accountID string,
) (object.ObjectStore, func(), error) {
	// Build an object-store handle for the test account.
	objStoreID := provider_local.SobjectObjectStoreID(providerID, accountID)
	volID := acc.GetVolume().GetID()
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, tb.Bus, false, objStoreID, volID, nil)
	if err != nil {
		return nil, nil, err
	}
	return handle.GetObjectStore(), diRef.Release, nil
}

// ownerKey returns the private key of the account's volume peer, which owns
// the Spaces a transfer writes to the account.
func ownerKey(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount) crypto.PrivKey {
	// Read the private key of the volume peer.
	t.Helper()
	p, err := acc.GetVolume().GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return priv
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

// worldTestSource is a local source whose Spaces replay world. released, if
// set, runs when the transfer releases a replay, after the copy and before
// the hold, while the copy's stage alone holds the World on the target.
type worldTestSource struct {
	*provider_transfer.LocalTransferSource
	world    *space_world.InnerState
	released func()
}

func (s *worldTestSource) ReplaySharedObject(context.Context, *logrus.Entry, *sobject.SharedObjectRef) (*space_world.InnerState, func(), error) {
	release := func() {}
	if s.released != nil {
		release = s.released
	}
	return s.world.CloneVT(), release, nil
}

// collectVolume runs two full collections of the account's Volume, so a
// record journaled before the first is past a 1ns grace by the second.
func collectVolume(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount) {
	t.Helper()
	collector, ok := acc.GetVolume().(volume.Collector)
	if !ok {
		t.Fatal("account volume does not collect")
	}
	for range 2 {
		for {
			res, err := collector.Collect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !res.More {
				break
			}
		}
	}
}

// putTestWorld stores a World of a root and a leaf holding payload in the
// block store of the Space soRef on acc, held by a root, and returns the root
func putTestWorld(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, providerID, accountID string, soRef *sobject.SharedObjectRef, payload []byte) (*block.BlockRef, *block.BlockRef) {
	t.Helper()
	bs, rel, err := acc.MountBlockStore(ctx, provider_local.NewBlockStoreRef(providerID, accountID, soRef.GetBlockStoreId()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	leaf, _, err := bs.PutBlock(ctx, payload, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := block.PutBlock(ctx, bs, &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: leaf}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bs.SetRoots(ctx, nil, []block.NamedRoot{{Name: "test/world", Ref: root}}); err != nil {
		t.Fatal(err)
	}
	return root, leaf
}

// TestLocalTransferHoldsWorldPastGrace checks, on a target Volume that frees
// at once, that a transferred Space keeps its World through a collection
// between the copy and the hold, and through collections after the transfer
// while nobody mounts the Space.
func TestLocalTransferHoldsWorldPastGrace(t *testing.T) {
	// Start a testbed whose Volumes free unheld blocks at once.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	storeConf := &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	tb, err := testbed.Default(ctx, testbed.WithStorages(storage_inmem.NewInmemStorageWithStoreConfig("default", storeConf)))
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

	// Store a World of a root and a leaf on the source, held by a root.
	soRef := createTestSpace(ctx, t, srcAcc, "held-space")
	payload := []byte("the transferred World's leaf")
	root, leaf := putTestWorld(ctx, t, srcAcc, providerID, srcAccountID, soRef, payload)

	// Transfer the Space, collecting the target after the copy.
	le := logrus.NewEntry(logrus.StandardLogger())
	src := &worldTestSource{
		LocalTransferSource: provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus),
		world:               &space_world.InnerState{HeadRef: &bucket.ObjectRef{RootRef: root}},
		released:            func() { collectVolume(ctx, t, tgtAcc) },
	}
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MIRROR, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{FilterSpaceIDs: []string{"held-space"}})
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// Without mounting the Space, store a block nothing holds beside its
	// World, and collect.
	tgtBS, tgtBSRel, err := tgtAcc.MountBlockStore(ctx, provider_local.NewBlockStoreRef(providerID, tgtAccountID, soRef.GetBlockStoreId()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtBSRel()
	unheld, _, err := tgtBS.PutBlock(ctx, []byte("unheld beside the transferred World"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	collectVolume(ctx, t, tgtAcc)

	// The unheld block is freed, the World reads, and the import root holds it.
	if _, found, err := tgtBS.GetBlock(ctx, unheld); err != nil || found {
		t.Fatalf("collection kept a block nothing holds: found=%v err=%v", found, err)
	}
	if _, found, err := tgtBS.GetBlock(ctx, root); err != nil || !found {
		t.Fatalf("collection freed the transferred World's root: found=%v err=%v", found, err)
	}
	data, found, err := tgtBS.GetBlock(ctx, leaf)
	if err != nil || !found || !bytes.Equal(data, payload) {
		t.Fatalf("collection freed the transferred World's leaf: found=%v err=%v", found, err)
	}
	held, err := tgtBS.GetRoot(ctx, space_world.RootImport)
	if err != nil || held.GetEmpty() {
		t.Fatalf("import root after the transfer = %v, %v; want it to hold the World", held, err)
	}
}

// putObjectContent stores content for key on store as a two-block object, a
// root pointing at a leaf holding the content, and returns the root.
func putObjectContent(ctx context.Context, t *testing.T, store block.StoreOps, key string) *block.BlockRef {
	t.Helper()
	leaf, _, err := store.PutBlock(ctx, objectContent(key), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := block.PutBlock(ctx, store, &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{ExamplePtr: leaf}})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// objectContent returns the content putObjectContent stores for key.
func objectContent(key string) []byte {
	return bytes.Repeat([]byte("content of "+key+"\n"), 512)
}

// readObjectContent reads the content of the object key in the World rooted
// at worldRef on store, following the object's root to its leaf.
func readObjectContent(ctx context.Context, t *testing.T, store block.StoreOps, worldRef *block.BlockRef, key string) []byte {
	// Open the World read-only on store.
	t.Helper()
	le := logrus.NewEntry(logrus.StandardLogger())
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, store, nil, &bucket.ObjectRef{RootRef: worldRef}, nil, nil)
	defer cursor.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, false, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Discard()

	// Look up the object's root.
	obj, found, err := ws.GetObject(ctx, key)
	if err != nil || !found {
		t.Fatalf("object %q in World %s: found=%v err=%v", key, worldRef.MarshalString(), found, err)
	}
	ref, _, err := obj.GetRootRef(ctx)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Follow the root to its leaf.
	data, found, err := store.GetBlock(ctx, ref.GetRootRef())
	if err != nil || !found {
		t.Fatalf("root of %q: found=%v err=%v", key, found, err)
	}
	root := &block_mock.Root{}
	if err := root.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	data, found, err = store.GetBlock(ctx, root.GetExampleSubBlock().GetExamplePtr())
	if err != nil || !found {
		t.Fatalf("leaf of %q: found=%v err=%v", key, found, err)
	}
	return data
}

// TestLocalMergeCopiesWorldContent merges a Space holding a block World of
// several objects, each a root and a leaf, and an older World it retains,
// with cleanup, so the source Space is deleted. Every object of both Worlds
// reads its content through the target's World, including the object only
// the retained World holds, and a block no World reaches is not copied.
func TestLocalMergeCopiesWorldContent(t *testing.T) {
	// Start a testbed with a local provider and two accounts.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
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

	// Mount the source Space's block store.
	soRef := createTestSpace(ctx, t, srcAcc, "content-space")
	srcBS, srcBSRel, err := srcAcc.MountBlockStore(ctx, provider_local.NewBlockStoreRef(providerID, srcAccountID, soRef.GetBlockStoreId()), nil)
	if err != nil {
		t.Fatal(err)
	}
	le := logrus.NewEntry(logrus.StandardLogger())
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, srcBS, nil, nil, nil, nil)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	commit := func(create []string, remove ...string) *block.BlockRef {
		t.Helper()
		for _, key := range create {
			obj, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{RootRef: putObjectContent(ctx, t, srcBS, key)})
			world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, key := range remove {
			if _, err := ws.DeleteObject(ctx, key); err != nil {
				t.Fatal(err)
			}
		}
		if err := ws.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return ws.GetRootRef().CloneVT()
	}

	// Write the retained World holding a and b, then the head, which drops a
	// and adds c, and a stray block no World reaches. Roots of the store
	// hold both Worlds.
	retained := commit([]string{"a", "b"})
	head := commit([]string{"c"}, "a")
	ws.Discard()
	cursor.Release()
	stray, _, err := srcBS.PutBlock(ctx, []byte("stray block no World reaches"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := srcBS.SetRoots(ctx, nil, []block.NamedRoot{{Name: "test/head", Ref: head}, {Name: "test/retained", Ref: retained}}); err != nil {
		t.Fatal(err)
	}
	srcBSRel()

	// Merge with cleanup.
	src := &worldTestSource{
		LocalTransferSource: provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus),
		world: &space_world.InnerState{
			HeadRef:       &bucket.ObjectRef{RootRef: head},
			RetainedRoots: []*space_world.RetainedRoot{{Name: "test/retained", RootRef: retained}},
		},
	}
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{Cleanup: src})
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}

	// The source lists only its account settings.
	srcList, err := src.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(srcList.GetSharedObjects()); n != 1 {
		t.Fatalf("source lists %d objects after cleanup, want its account settings", n)
	}

	// Every object of both Worlds reads its content on the target, and the
	// stray block is absent.
	tgtBS, tgtBSRel, err := tgtAcc.MountBlockStore(ctx, provider_local.NewBlockStoreRef(providerID, tgtAccountID, soRef.GetBlockStoreId()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtBSRel()
	for _, read := range []struct {
		world *block.BlockRef
		key   string
	}{{head, "b"}, {head, "c"}, {retained, "a"}, {retained, "b"}} {
		if got := readObjectContent(ctx, t, tgtBS, read.world, read.key); !bytes.Equal(got, objectContent(read.key)) {
			t.Fatalf("object %q reads %d bytes on the target, want its content", read.key, len(got))
		}
	}
	if found, err := tgtBS.GetBlockExists(ctx, stray); err != nil || found {
		t.Fatalf("stray block on the target: found=%v err=%v", found, err)
	}
}

// faultTarget is a local target whose next state write fails, before writing
// when failBefore is set, as a crash between the hold and the write does, or
// after writing when failAfter is set, as a crash before the checkpoint does.
type faultTarget struct {
	*provider_transfer.LocalTransferTarget
	failBefore, failAfter bool
}

func (f *faultTarget) WriteSharedObjectState(ctx context.Context, le *logrus.Entry, id string, owner crypto.PrivKey, stateData []byte, origin *provider_transfer.TransferOrigin, expect *provider_transfer.TargetSpace) error {
	if f.failBefore {
		f.failBefore = false
		return errors.New("crash before the state write")
	}
	if err := f.LocalTransferTarget.WriteSharedObjectState(ctx, le, id, owner, stateData, origin, expect); err != nil {
		return err
	}
	if f.failAfter {
		f.failAfter = false
		return errors.New("crash after the state write")
	}
	return nil
}

// targetSpaceState reads, or with set writes, the stored state of the Space
// id on the target, where every edit of the Space lands.
func targetSpaceState(ctx context.Context, t *testing.T, store object.ObjectStore, id string, set []byte) []byte {
	t.Helper()
	tx, err := store.NewTransaction(ctx, set != nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	key := provider_local.SobjectObjectStoreHostStateKey(id)
	if set != nil {
		if err := tx.Set(ctx, key, set); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return set
	}
	data, found, err := tx.Get(ctx, key)
	if err != nil || !found {
		t.Fatalf("read target state of %s: found=%v err=%v", id, found, err)
	}
	return bytes.Clone(data)
}

// editTargetSpace replaces the stored state of the Space id on the target
// with a new valid state, as an edit made on the target does, and returns it.
func editTargetSpace(ctx context.Context, t *testing.T, store object.ObjectStore, acc *provider_local.ProviderAccount, id string) []byte {
	t.Helper()
	state, _, err := sobject.BuildGenesisSOState(logrus.NewEntry(logrus.StandardLogger()), nil, id, ownerKey(ctx, t, acc), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return targetSpaceState(ctx, t, store, id, data)
}

// TestLocalTransferRetryKeepsWrittenState retries a run through both crash
// windows: between the hold and the state write the retry copies the Space
// again, and after the state write it finds the Space written at the source's
// head and skips it. An edit made on the target after that leaves the Space
// not moved on the next run, with the edit kept.
func TestLocalTransferRetryKeepsWrittenState(t *testing.T) {
	// Start a source with one Space and a target.
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
	soRef := createTestSpace(ctx, t, srcAcc, "space-a")
	root, _ := putTestWorld(ctx, t, srcAcc, providerID, srcAccountID, soRef, []byte("space-a's leaf"))
	tgtStore, tgtStoreRel, err := buildTestObjectStore(ctx, tb, tgtAcc, providerID, tgtAccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtStoreRel()
	le := logrus.NewEntry(logrus.StandardLogger())
	src := &worldTestSource{
		LocalTransferSource: provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus),
		world:               &space_world.InnerState{HeadRef: &bucket.ObjectRef{RootRef: root}},
	}
	tgt := &faultTarget{LocalTransferTarget: provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)}
	run := func() (provider_transfer.TransferPhase, error) {
		xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MIRROR, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, nil)
		err := xfer.Execute(ctx)
		return xfer.GetState().GetPhase(), err
	}

	// The run stops before the state write; the retry copies the Space again,
	// writes it, and stops after the write.
	tgt.failBefore = true
	if _, err := run(); err == nil {
		t.Fatal("expected the run to stop before the state write")
	}
	tgt.failAfter = true
	if _, err := run(); err == nil {
		t.Fatal("expected the retry to stop after the state write")
	}
	written := targetSpaceState(ctx, t, tgtStore, "space-a", nil)

	// The next retry finds the Space written at the source's head and
	// finishes without writing it again.
	if phase, err := run(); err != nil || phase != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("retry after the write = %s, %v; want COMPLETE", phase, err)
	}
	if got := targetSpaceState(ctx, t, tgtStore, "space-a", nil); !bytes.Equal(got, written) {
		t.Fatal("the retry wrote a Space the target held at the source's head")
	}

	// The Space is edited on the target: the next run leaves it not moved and
	// keeps the edit.
	edited := editTargetSpace(ctx, t, tgtStore, tgtAcc, "space-a")
	if phase, err := run(); err != nil || phase != provider_transfer.TransferPhase_TransferPhase_NOT_MOVED {
		t.Fatalf("run after the edit = %s, %v; want NOT_MOVED", phase, err)
	}
	if got := targetSpaceState(ctx, t, tgtStore, "space-a", nil); !bytes.Equal(got, edited) {
		t.Fatal("the run replaced the edit made on the target")
	}
}

// TestLocalMergeKeepsEditOfMovedSpace runs a Merge that writes a Space and
// stops, after which the Space is edited on the target: a new Merge from the
// same source leaves the Space not moved, keeps the edit, and deletes nothing
// from the source.
func TestLocalMergeKeepsEditOfMovedSpace(t *testing.T) {
	// Start a source with one Space and a target.
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
	soRef := createTestSpace(ctx, t, srcAcc, "space-a")
	root, _ := putTestWorld(ctx, t, srcAcc, providerID, srcAccountID, soRef, []byte("space-a's leaf"))
	tgtStore, tgtStoreRel, err := buildTestObjectStore(ctx, tb, tgtAcc, providerID, tgtAccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtStoreRel()
	le := logrus.NewEntry(logrus.StandardLogger())
	src := &worldTestSource{
		LocalTransferSource: provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus),
		world:               &space_world.InnerState{HeadRef: &bucket.ObjectRef{RootRef: root}},
	}
	tgt := &faultTarget{LocalTransferTarget: provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)}
	run := func() (provider_transfer.TransferPhase, error) {
		xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{Cleanup: src})
		err := xfer.Execute(ctx)
		return xfer.GetState().GetPhase(), err
	}

	// A run writes the Space and stops; the Space is edited on the target.
	tgt.failAfter = true
	if _, err := run(); err == nil {
		t.Fatal("expected the run to stop after the state write")
	}
	edited := editTargetSpace(ctx, t, tgtStore, tgtAcc, "space-a")

	// A new run leaves the Space not moved and keeps the edit and the source.
	if phase, err := run(); err != nil || phase != provider_transfer.TransferPhase_TransferPhase_NOT_MOVED {
		t.Fatalf("run after the edit = %s, %v; want NOT_MOVED", phase, err)
	}
	if got := targetSpaceState(ctx, t, tgtStore, "space-a", nil); !bytes.Equal(got, edited) {
		t.Fatal("the new run replaced the state the target held")
	}
	srcList, err := src.GetSharedObjectList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range srcList.GetSharedObjects() {
		found = found || entry.GetRef().GetProviderResourceRef().GetId() == "space-a"
	}
	if !found {
		t.Fatal("the run deleted the source Space")
	}
}

// TestLocalMirrorKeepsTargetInvite checks that a Mirror leaves a Space not
// moved, and its target state unchanged, once the target added an invite.
func TestLocalMirrorKeepsTargetInvite(t *testing.T) {
	testLocalMirrorKeepsMembership(t, func(ctx context.Context, t *testing.T, _ *provider_local.ProviderAccount, store object.ObjectStore) {
		// Add an invite to the stored state, leaving its World alone.
		state := &sobject.SOState{}
		if err := state.UnmarshalVT(targetSpaceState(ctx, t, store, "space-a", nil)); err != nil {
			t.Fatal(err)
		}
		state.Invites = append(state.Invites, &sobject.SOInvite{InviteId: "invite-1", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER})
		data, err := state.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		targetSpaceState(ctx, t, store, "space-a", data)
	})
}

// TestLocalMirrorKeepsTargetGrant checks the same for a grant to a second
// device, the config change account member enrollment makes, which advances
// the config chain.
func TestLocalMirrorKeepsTargetGrant(t *testing.T) {
	testLocalMirrorKeepsMembership(t, func(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, store object.ObjectStore) {
		// Find the target's Space and mount it.
		list, err := provider_transfer.NewLocalTransferSource(acc, nil).GetSharedObjectList(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var ref *sobject.SharedObjectRef
		for _, entry := range list.GetSharedObjects() {
			if entry.GetRef().GetProviderResourceRef().GetId() == "space-a" {
				ref = entry.GetRef()
			}
		}
		if ref == nil {
			t.Fatal("the target does not list space-a")
		}
		so, release, err := acc.MountSharedObject(ctx, ref, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer release()

		// Grant a second device as an owner, as enrollment does, and wait
		// until the grant is stored.
		device, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatal(err)
		}
		host := so.(sobject.InviteHost)
		_, outcome, _, err := sobject.AddSOParticipant(ctx, host.GetSOHost(), so.GetSharedObjectID(), host.GetPrivKey(), so.GetPeerID().String(), device.GetPeerID().String(), device.GetPubKey(), sobject.SOParticipantRole_SOParticipantRole_OWNER, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if outcome != sobject.SOParticipantAddOutcome_SOParticipantAddOutcome_ADDED {
			t.Fatalf("grant outcome = %s; want ADDED", outcome)
		}
		if err := host.GetSOHost().WaitDurable(ctx); err != nil {
			t.Fatal(err)
		}

		// The grant advanced the stored config chain.
		state := &sobject.SOState{}
		if err := state.UnmarshalVT(targetSpaceState(ctx, t, store, "space-a", nil)); err != nil {
			t.Fatal(err)
		}
		if state.GetConfig().GetConfigChainSeqno() == 0 {
			t.Fatal("the grant left the config chain at seqno 0")
		}
	})
}

// testLocalMirrorKeepsMembership mirrors a Space twice, checking that the
// second run takes the newer World, applies change to the target's Space,
// then mirrors a newer source World and checks that the run ends not moved
// with the target's state byte for byte unchanged, rather than restarting the
// Space at a new genesis.
func testLocalMirrorKeepsMembership(t *testing.T, change func(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, store object.ObjectStore)) {
	// Start a source with one Space and a target.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()
	srcAcc, _, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()
	soRef := createTestSpace(ctx, t, srcAcc, "space-a")
	tgtStore, tgtStoreRel, err := buildTestObjectStore(ctx, tb, tgtAcc, providerID, tgtAccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer tgtStoreRel()
	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	mirror := func(leaf string) provider_transfer.TransferPhase {
		t.Helper()
		writeSourceObject(ctx, t, tb, srcAcc, soRef, leaf)
		xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MIRROR, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, nil)
		if err := xfer.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		return xfer.GetState().GetPhase()
	}

	// The first Mirror writes the Space; the second takes the newer World.
	if phase := mirror("first leaf"); phase != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("first mirror = %s; want COMPLETE", phase)
	}
	first := targetSpaceState(ctx, t, tgtStore, "space-a", nil)
	if phase := mirror("second leaf"); phase != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("second mirror = %s; want COMPLETE", phase)
	}
	if bytes.Equal(targetSpaceState(ctx, t, tgtStore, "space-a", nil), first) {
		t.Fatal("the second mirror left the older World on the target")
	}

	// The target changes the Space's membership, leaving its World alone.
	change(ctx, t, tgtAcc, tgtStore)
	changed := targetSpaceState(ctx, t, tgtStore, "space-a", nil)

	// The next Mirror leaves the Space not moved and keeps the change.
	if phase := mirror("third leaf"); phase != provider_transfer.TransferPhase_TransferPhase_NOT_MOVED {
		t.Fatalf("mirror after the change = %s; want NOT_MOVED", phase)
	}
	if !bytes.Equal(targetSpaceState(ctx, t, tgtStore, "space-a", nil), changed) {
		t.Fatal("the mirror replaced a Space whose membership changed on the target")
	}
}

// writeSourceObject creates the World object key in the Space soRef through
// the Space's own World engine, then retires the engine and waits until the
// Space's operation order holds the write, so a transfer that replays the
// Space reads the new World and nothing else writes it meanwhile.
func writeSourceObject(ctx context.Context, t *testing.T, tb *testbed.Testbed, acc *provider_local.ProviderAccount, soRef *sobject.SharedObjectRef, key string) {
	// Count the Space's operations before the write.
	t.Helper()
	so, relSO, err := acc.MountSharedObject(ctx, soRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relSO()
	n := sourceOpCount(ctx, t, so)

	// Start the Space's World engine and create the object.
	engineID := space.SpaceEngineId(soRef)
	busEngine, engRef, err := world.StartEngine(ctx, tb.Bus, space_world.NewConfig(engineID, soRef), space_world.NewFactory(tb.Bus, space_world_engine.OpTypes))
	if err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		engRef.Release()
		t.Fatal(err)
	}

	// Retire the engine, so the next start runs a new one.
	var served *space_world.Engine
	if err := world.AccessEngine(ctx, busEngine, func(_ context.Context, eng world.Engine) error {
		served = eng.(*space_world.Engine)
		return nil
	}); err != nil {
		engRef.Release()
		t.Fatal(err)
	}
	engRef.Release()
	if _, err := served.WaitSeqno(ctx, math.MaxUint64); !world.IsSuperseded(err) {
		t.Fatalf("waiting for the released engine to retire: %v", err)
	}

	// Wait for the write to reach the operation order.
	ctr, relState, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relState()
	if _, err := ctr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		return len(set.Order()) > n, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
}

// sourceOpCount returns the number of operations in so's order now.
func sourceOpCount(ctx context.Context, t *testing.T, so sobject.SharedObject) int {
	// Read the current order.
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return len(set.Order())
}

// mergeWithHook runs a Merge of a source holding the Space "moved-space"
// into a second account, with hook run between the moves and cleanup, and a
// source delete that records whether it got past its keep check. It returns
// whether the delete ran, the source, and the run's error.
func mergeWithHook(t *testing.T, hook func(ctx context.Context, src, tgt *provider_local.ProviderAccount)) (bool, *provider_transfer.LocalTransferSource, error) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	t.Cleanup(provRel)
	srcAcc, _, srcRel := createAccount(ctx, t, prov)
	t.Cleanup(srcRel)
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	t.Cleanup(tgtRel)
	createTestSpace(ctx, t, srcAcc, "moved-space")

	le := logrus.NewEntry(logrus.StandardLogger())
	src := provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus)
	tgt := provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)
	var deleted bool
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{
		Cleanup: src,
		CopyIdentity: func(ctx context.Context) error {
			hook(ctx, srcAcc, tgtAcc)
			return nil
		},
		DeleteSource: func(ctx context.Context, keep func(context.Context) error) error {
			if err := keep(ctx); err != nil {
				return err
			}
			deleted = true
			return nil
		},
	})
	err = xfer.Execute(ctx)
	return deleted, src, err
}

// listsSpace reports whether src lists the Space id.
func listsSpace(t *testing.T, src *provider_transfer.LocalTransferSource, id string) bool {
	t.Helper()
	list, err := src.GetSharedObjectList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range list.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return true
		}
	}
	return false
}

// TestLocalMergeKeepsSourceWhenTargetLost checks that a Space deleted from
// the target after its move stays on the source, and the account with it.
func TestLocalMergeKeepsSourceWhenTargetLost(t *testing.T) {
	deleted, src, err := mergeWithHook(t, func(ctx context.Context, _, tgt *provider_local.ProviderAccount) {
		if err := tgt.DeleteSharedObject(ctx, "moved-space"); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("the run deleted the source account after the target lost the Space")
	}
	if !listsSpace(t, src, "moved-space") {
		t.Fatal("the run deleted the source's only copy of the Space")
	}
}

// TestLocalMergeKeepsSpaceCreatedOnSource checks that a Space created on the
// source during the run keeps the source account.
func TestLocalMergeKeepsSpaceCreatedOnSource(t *testing.T) {
	deleted, src, err := mergeWithHook(t, func(ctx context.Context, srcAcc, _ *provider_local.ProviderAccount) {
		createTestSpace(ctx, t, srcAcc, "new-space")
	})
	if err == nil || deleted {
		t.Fatalf("run = %v, deleted = %v; want an error and the account kept", err, deleted)
	}
	if !listsSpace(t, src, "new-space") {
		t.Fatal("the run deleted the Space created during it")
	}
}

// sharedTestSource lists every Space of a local source as shared.
type sharedTestSource struct {
	*provider_transfer.LocalTransferSource
}

func (s *sharedTestSource) GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error) {
	list, err := s.LocalTransferSource.GetSharedObjectList(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range list.GetSharedObjects() {
		if !entry.GetMeta().GetAccountPrivate() {
			entry.Source = "shared"
		}
	}
	return list, nil
}

// sharedTestTarget reads every Space as shared with the target account.
type sharedTestTarget struct {
	*provider_transfer.LocalTransferTarget
}

func (t *sharedTestTarget) ReadSharedObject(context.Context, *logrus.Entry, string, *provider_transfer.TransferOrigin) (*provider_transfer.TargetSpace, error) {
	return &provider_transfer.TargetSpace{Holds: provider_transfer.TargetHoldsShared}, nil
}

// TestLocalMergeKeepsSharedSpaceWithUnsentOps checks that a Merge keeps a
// shared Space, and the operation in it, on the source while the target's
// copy lacks an operation the source wrote: the source's copy may hold the
// only one.
func TestLocalMergeKeepsSharedSpaceWithUnsentOps(t *testing.T) {
	// Start a source and a target that each hold a copy of the Space.
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	providerID := "local"
	prov, provRel := setupLocalProvider(ctx, t, tb, providerID)
	defer provRel()
	srcAcc, _, srcRel := createAccount(ctx, t, prov)
	defer srcRel()
	tgtAcc, tgtAccountID, tgtRel := createAccount(ctx, t, prov)
	defer tgtRel()
	soRef := createTestSpace(ctx, t, srcAcc, "space-s")
	createTestSpace(ctx, t, tgtAcc, "space-s")

	// The source writes an operation the target never receives.
	so, soRel, err := srcAcc.MountSharedObject(ctx, soRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := so.QueueOperation(ctx, []byte("unsent")); err != nil {
		t.Fatal(err)
	}
	soRel()
	src := &sharedTestSource{provider_transfer.NewLocalTransferSource(srcAcc, tb.Bus)}
	_, wrote, err := src.AuthorHead(ctx, "space-s")
	if err != nil || wrote == 0 {
		t.Fatalf("source chain = %d, %v; want the written operation", wrote, err)
	}

	// The Merge waits for the target to receive it, then keeps the Space.
	defer provider_transfer.SetSendTimeout(time.Second)()
	tgt := &sharedTestTarget{provider_transfer.NewLocalTransferTarget(tgtAcc, providerID, tgtAccountID, tb.Bus)}
	le := logrus.NewEntry(logrus.StandardLogger())
	xfer := provider_transfer.NewTransfer(le, provider_transfer.TransferMode_TransferMode_MERGE, src, tgt, ownerKey(ctx, t, tgtAcc), 1, 2, &provider_transfer.TransferOptions{Cleanup: src})
	if err := xfer.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	state := xfer.GetState()
	spaces := state.GetSpaces()
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_NOT_MOVED || len(spaces) != 1 || !strings.HasPrefix(spaces[0].GetNotMovedReason(), "operations not sent") {
		t.Fatalf("transfer = %s %v; want the Space not moved, operations not sent", state.GetPhase(), spaces)
	}

	// The source still lists the Space and stores the operation.
	_, kept, err := src.AuthorHead(ctx, "space-s")
	if err != nil || kept != wrote {
		t.Fatalf("source chain after the Merge = %d, %v; want %d", kept, err, wrote)
	}
}
