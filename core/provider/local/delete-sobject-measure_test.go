//go:build !js && !wasip1

package provider_local

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/core/provider"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// MeasureDeleteStateEnv names the environment variable that, set to 1, runs
// TestMeasureDeleteStateWriterHold.
const MeasureDeleteStateEnv = "SPACEWAVE_MEASURE_DELETE_STATE"

// measureDeleteMembers is the member count of the measured config.
const measureDeleteMembers = 400

// TestMeasureDeleteStateWriterHold measures how long the delete of an object's
// stored state holds the account store's writer on the desktop's on-disk
// volume, for objects with 500 and 5,000 config-history entries, each holding
// a full config of measureDeleteMembers members. The entries are written,
// made durable, and the volume reopened before the delete, as for a
// long-lived object. It runs only when MeasureDeleteStateEnv is 1.
func TestMeasureDeleteStateWriterHold(t *testing.T) {
	if os.Getenv(MeasureDeleteStateEnv) != "1" {
		t.Skip("set " + MeasureDeleteStateEnv + "=1 to measure")
	}
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	counts := []int{500, 5000}

	// Fill one object per count with its entries, durably.
	value := measureConfigChange(t, 0, nil)
	t.Logf("config-history entry: %d bytes for %d members", len(value), measureDeleteMembers)
	vol, store, release := openMeasureStore(ctx, t, le, conf)
	for _, n := range counts {
		fillMeasureHistory(ctx, t, store, fmt.Sprintf("measure%d", n), n)
	}
	if err := volume.WaitDurable(ctx, vol); err != nil {
		t.Fatal(err)
	}
	release()

	// Reopen the volume and time each delete transaction from open to commit.
	_, store, release = openMeasureStore(ctx, t, le, conf)
	defer release()
	for _, n := range counts {
		prefix := SobjectObjectStorePrefix(fmt.Sprintf("measure%d", n))
		start := time.Now()
		err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			return deleteKeysWithPrefix(ctx, tx, prefix)
		})
		held := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("delete of %d config-history entries held the writer %v", n, held)
	}
}

// openMeasureStore opens the on-disk volume at conf and an object store in it.
func openMeasureStore(ctx context.Context, t *testing.T, le *logrus.Entry, conf *volume_s4db.Config) (volume.Volume, object.ObjectStore, func()) {
	t.Helper()
	vol, err := volume_s4db.NewVolume(ctx, le, conf)
	if err != nil {
		t.Fatal(err)
	}
	store, rel, err := vol.AccessObjectStore(ctx, "measure", nil)
	if err != nil {
		_ = vol.Close()
		t.Fatal(err)
	}
	return vol, store, func() {
		rel()
		if err := vol.Close(); err != nil {
			t.Error(err)
		}
	}
}

// fillMeasureHistory writes n config-history entries for id, in batches.
func fillMeasureHistory(ctx context.Context, t *testing.T, store object.ObjectStore, id string, n int) {
	t.Helper()
	config := measureConfig(t)
	const batch = 500
	for first := 0; first < n; first += batch {
		err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return store.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			for i := first; i < min(first+batch, n); i++ {
				sum := sha256.Sum256(fmt.Appendf(nil, "%s/%d", id, i))
				value := measureConfigChange(t, uint64(i), config)
				if err := tx.Set(ctx, SOConfigHistoryEntryKey(id, sum[:]), value); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// measureConfig builds a config of measureDeleteMembers members with fake
// identities.
func measureConfig(t *testing.T) *sobject.SharedObjectConfig {
	t.Helper()
	config := &sobject.SharedObjectConfig{}
	for i := range measureDeleteMembers {
		priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			t.Fatal(err)
		}
		config.Participants = append(config.Participants, &sobject.SOParticipantConfig{
			PeerId:       pid.String(),
			Role:         sobject.SOParticipantRole_SOParticipantRole_WRITER,
			EntityId:     fmt.Sprintf("fake-entity-%04d", i),
			Username:     fmt.Sprintf("fake-member-%04d", i),
			VotingWeight: 1,
		})
	}
	return config
}

// measureConfigChange returns a serialized config change at seqno carrying
// config and one signature, building the config when it is nil.
func measureConfigChange(t *testing.T, seqno uint64, config *sobject.SharedObjectConfig) []byte {
	t.Helper()
	if config == nil {
		config = measureConfig(t)
	}
	prev := sha256.Sum256(fmt.Appendf(nil, "fake-previous-%d", seqno))
	change := &sobject.SOConfigChange{
		ConfigSeqno:    seqno,
		Config:         config,
		PreviousHash:   prev[:],
		ChangeType:     sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
		SharedObjectId: "fake-object",
		Signatures: []*peer.Signature{{
			PubKey:   make([]byte, 32),
			HashType: hash.HashType_HashType_SHA256,
			SigData:  make([]byte, 64),
		}},
	}
	data, err := change.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestDeleteSharedObjectRehearsal rehearses the delete of a shared object on
// an on-disk account volume, with the state delete on and off: it creates
// three objects, two sharing an id prefix and two sharing a block, deletes
// one, closes and reopens the volume, runs the account's collection, and
// checks what each mode must leave behind.
func TestDeleteSharedObjectRehearsal(t *testing.T) {
	t.Run("on", func(t *testing.T) { rehearseDeleteSharedObject(t, false) })
	t.Run("off", func(t *testing.T) { rehearseDeleteSharedObject(t, true) })
}

// rehearseDeleteSharedObject runs the rehearsal with the state delete off or
// on. In both modes the deleted object is unlisted, every other so/ key
// survives, and the survivors mount, read their state and reach every block
// of their roots. With the delete on, the deleted object's keys and the
// blocks only it reached are gone and its mount is refused; with it off, its
// keys stay. Off still releases the object's roots, as the delete did before
// the state delete existed, so its blocks are freed wherever collector frees
// are on.
func rehearseDeleteSharedObject(t *testing.T, off bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	setTestDeleteStateOff(t, off)
	dir := t.TempDir()

	// Create the objects, each with state, local keys and a block tree.
	a, sessRef, release := openRehearsalAccount(ctx, t, dir, nil)
	ids := []string{"so1", "so1x", "so2"}
	refs := make(map[string]*sobject.SharedObjectRef, len(ids))
	own := make(map[string][]*block.BlockRef, len(ids))
	var sharedRef *block.BlockRef
	for _, id := range ids {
		refs[id] = createTestSharedObject(ctx, t, a, id)
		so, rel := mountTestSharedObject(ctx, t, a, refs[id])
		writeTestStateData(ctx, t, so, []byte("fake-state-"+id))
		if err := tryWriteTestLocalKeys(ctx, so, 10); err != nil {
			rel()
			t.Fatal(err)
		}
		own[id], sharedRef = putRehearsalTree(ctx, t, so, id)
		rel()
	}

	// Record the keys, delete the first object, and close the volume.
	deletedKeys := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("so1"))
	if deletedKeys == 0 {
		t.Fatal("the object to delete holds no keys")
	}
	kept := listRehearsalKeys(ctx, t, a)
	if n := rehearsalBlocksHeld(ctx, t, a.GetVolume(), own["so1"]); n != len(own["so1"]) {
		t.Fatalf("the volume holds %d of the object's %d blocks before the delete", n, len(own["so1"]))
	}
	if err := a.DeleteSharedObject(ctx, "so1"); err != nil {
		t.Fatal(err)
	}
	if err := volume.WaitDurable(ctx, a.GetVolume()); err != nil {
		t.Fatal(err)
	}
	release()

	// Reopen the volume and collect at least once, until the deleted
	// object's blocks are freed or a fixed number of passes ran. The
	// delete's own collection may already have freed them.
	a, _, release = openRehearsalAccount(ctx, t, dir, sessRef)
	defer release()
	vol := a.GetVolume()
	heldAtOpen := rehearsalBlocksHeld(ctx, t, vol, own["so1"])
	held, passes := heldAtOpen, 0
	for passes == 0 || (held != 0 && passes < 8) {
		a.triggerGCCleanup()
		if err := a.WaitGCCleanup(ctx); err != nil {
			t.Fatal(err)
		}
		passes++
		held = rehearsalBlocksHeld(ctx, t, vol, own["so1"])
	}
	t.Logf("delete state off=%v: %d of %d deleted-only blocks held at reopen, %d after %d collection passes", off, heldAtOpen, len(own["so1"]), held, passes)

	// Check the deleted object's keys for the mode.
	n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("so1"))
	if !off && n != 0 {
		t.Fatalf("delete left %d keys under so/so1/ across the restart", n)
	}
	if off && n != deletedKeys {
		t.Fatalf("delete with the state delete off changed so/so1/ from %d keys to %d", deletedKeys, n)
	}

	// Every other so/ key survives the delete, restart and collection.
	after := listRehearsalKeys(ctx, t, a)
	for key := range kept {
		if _, ok := after[key]; !ok {
			t.Fatalf("the key %q is gone after the delete", key)
		}
	}

	// The survivors mount, read their state and reach every block.
	for _, id := range ids[1:] {
		so, rel := mountTestSharedObject(ctx, t, a, refs[id])
		if got := readTestStateData(ctx, t, so); !bytes.Equal(got, []byte("fake-state-"+id)) {
			rel()
			t.Fatalf("%s reads %q after the delete", id, got)
		}
		reached := walkRehearsalRoots(ctx, t, so, vol)
		rel()
		for _, ref := range append(own[id], sharedRef) {
			if _, ok := reached[ref.MarshalString()]; !ok {
				t.Fatalf("%s no longer reaches its block %s", id, ref.MarshalString())
			}
		}
	}
	if testListed(a, "so1") || !testListed(a, "so1x") || !testListed(a, "so2") {
		t.Fatal("delete did not unlist exactly the named object")
	}

	// With the delete on, the deleted object's blocks are freed and its
	// mount is refused.
	if !off {
		if held != 0 {
			t.Fatalf("%d blocks only the deleted object reached are still held", held)
		}
		expectTestMountNotFound(ctx, t, a, refs["so1"])
	}
}

// freeingS4db is on-disk s4db storage whose Volumes free the blocks their
// collector finds unreachable once a journal record is a nanosecond old.
// Production s4db Volumes keep frees off.
type freeingS4db struct {
	storage.Storage
}

// BuildVolumeConfig builds the s4db Volume config with frees on.
func (s freeingS4db) BuildVolumeConfig(id string, base *volume_controller.Config) (config.Config, error) {
	conf, err := s.Storage.BuildVolumeConfig(id, base)
	if err != nil {
		return nil, err
	}
	s4dbConf := conf.(*volume_s4db.Config)
	s4dbConf.StoreConfig = &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}
	return s4dbConf, nil
}

// openRehearsalAccount starts a testbed whose account volumes are s4db files
// under dir, loads the local provider, and opens an account with its Session
// mounted: a new account when sessRef is nil, else the account of sessRef.
// The returned release stops everything in reverse order.
func openRehearsalAccount(ctx context.Context, t *testing.T, dir string, sessRef *core_session.SessionRef) (*ProviderAccount, *core_session.SessionRef, func()) {
	t.Helper()
	tb, err := testbed.Default(ctx, testbed.WithStorages(freeingS4db{storage_native.NewS4db(false, dir)}))
	if err != nil {
		t.Fatal(err)
	}
	rels := []func(){tb.Release}
	release := func() {
		for _, rel := range slices.Backward(rels) {
			rel()
		}
	}
	fail := func(err error) {
		release()
		t.Fatal(err)
	}

	// Load and look up the local provider.
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		fail(err)
	}
	rels = append(rels, provCtrlRef.Release)
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		fail(err)
	}
	rels = append(rels, provRef.Release)

	// Create the account on the first open.
	localProv := prov.(*Provider)
	if sessRef == nil {
		sessRef, err = localProv.CreateLocalAccountAndSession(ctx, "")
		if err != nil {
			fail(err)
		}
	}

	// Open the account and mount its Session.
	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	acc, accRel, err := localProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		fail(err)
	}
	rels = append(rels, accRel)
	a := acc.(*ProviderAccount)
	_, sessRel, err := a.MountSession(ctx, sessRef, nil)
	if err != nil {
		fail(err)
	}
	rels = append(rels, sessRel)
	return a, sessRef, release
}

// putRehearsalTree stores a two-block tree under the named root "rehearsal"
// of the object's block store: a parent naming id over a leaf naming id and a
// leaf every object shares. It returns the blocks only this object reaches,
// and the shared leaf.
func putRehearsalTree(ctx context.Context, t *testing.T, so *SharedObject, id string) ([]*block.BlockRef, *block.BlockRef) {
	t.Helper()
	leaf := []byte("fake-leaf-" + id)
	shared := []byte("fake-shared-leaf")
	parent := []byte("fake-parent-" + id)
	leafRef, err := block.BuildBlockRef(leaf, nil)
	if err != nil {
		t.Fatal(err)
	}
	sharedRef, err := block.BuildBlockRef(shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	parentRef, err := block.BuildBlockRef(parent, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = so.GetBlockStore().SetRoots(ctx, []*block.PutBatchEntry{
		{Ref: leafRef, Data: leaf, RefsKnown: true},
		{Ref: sharedRef, Data: shared, RefsKnown: true},
		{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef, sharedRef}, RefsKnown: true},
	}, []block.NamedRoot{{Name: "rehearsal", Ref: parentRef}})
	if err != nil {
		t.Fatal(err)
	}
	return []*block.BlockRef{parentRef, leafRef}, sharedRef
}

// listRehearsalKeys returns the account object store keys under so/, less
// those of the object so1.
func listRehearsalKeys(ctx context.Context, t *testing.T, a *ProviderAccount) map[string]struct{} {
	t.Helper()
	store, rel, err := a.buildSoObjectStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	deleted := SobjectObjectStorePrefix("so1")
	keys := make(map[string]struct{})
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		clear(keys)
		return tx.ScanPrefixKeys(ctx, []byte("so/"), func(key []byte) error {
			if !bytes.HasPrefix(key, deleted) {
				keys[string(key)] = struct{}{}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// rehearsalBlocksHeld counts the refs the volume still holds.
func rehearsalBlocksHeld(ctx context.Context, t *testing.T, vol volume.Volume, refs []*block.BlockRef) int {
	t.Helper()
	var n int
	for _, ref := range refs {
		ok, err := vol.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			n++
		}
	}
	return n
}

// walkRehearsalRoots walks every named root of the object's block store
// through the volume, fails on a block the volume lacks or holds without its
// refs, and returns the blocks reached.
func walkRehearsalRoots(ctx context.Context, t *testing.T, so *SharedObject, vol volume.Volume) map[string]struct{} {
	t.Helper()
	var stack []*block.BlockRef
	err := so.GetBlockStore().ListRoots(ctx, "", func(root block.NamedRoot) error {
		if !root.Ref.GetEmpty() {
			stack = append(stack, root.Ref)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reached := make(map[string]struct{})
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		key := ref.MarshalString()
		if _, ok := reached[key]; ok {
			continue
		}
		reached[key] = struct{}{}
		if ok, err := vol.GetBlockExists(ctx, ref); err != nil || !ok {
			t.Fatalf("a survivor reaches %s, which the volume lacks: %v", key, err)
		}
		stored, err := vol.GetStoredBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if !stored.RefsKnown {
			t.Fatalf("the volume holds %s without its refs", key)
		}
		stack = append(stack, stored.Refs...)
	}
	return reached
}
