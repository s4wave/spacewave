//go:build !js

package space_world_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
)

// TestSetRootsAcrossBuilds checks that a Space written by one build replays
// to the same World on another, so builds with and without set roots can
// share a Space. SPACEWAVE_SR_COMPAT_ROLE runs one step on the s4db store
// under SPACEWAVE_SR_COMPAT_DIR: "write" stores keypairs below and above a
// checkpoint and records every object root; "verify" replays, requires the
// same roots and keypairs, and requires each stored operation to re-encode to
// its stored bytes. SPACEWAVE_SR_COMPAT_ABOVE_KEYPAIR=0 leaves out the
// keypair above the checkpoint, whose staged block a build without set roots
// does not store. Run the steps with test binaries of the two builds.
func TestSetRootsAcrossBuilds(t *testing.T) {
	role, dir := os.Getenv("SPACEWAVE_SR_COMPAT_ROLE"), os.Getenv("SPACEWAVE_SR_COMPAT_DIR")
	if role == "" || dir == "" {
		t.Skip("runs as a step of a cross-build check")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	refPath, rootsPath := filepath.Join(dir, "so-ref"), filepath.Join(dir, "roots")

	// Open the account on the store, creating the Space on write.
	var soRef *sobject.SharedObjectRef
	if role == "verify" {
		data, err := os.ReadFile(refPath)
		if err != nil {
			t.Fatal(err)
		}
		soRef = &sobject.SharedObjectRef{}
		if err := soRef.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
	}
	tb, soRef := compatOpenSpace(ctx, t, filepath.Join(dir, "storage"), soRef)
	so, soMountRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer soMountRef.Release()
	eng, engRef, err := world.StartEngine(ctx, tb.Bus, space_world.NewConfig("compat-world", soRef), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engRef.Release()

	if role == "verify" {
		// Replay runs at start: no cursor is saved at a clean stop.
		want, err := os.ReadFile(rootsPath)
		if err != nil {
			t.Fatal(err)
		}
		got := compatRoots(ctx, t, eng)
		if got != string(want) {
			t.Fatalf("verify: roots after replay differ\nwrote:\n%s\nreplayed:\n%s", want, got)
		}
		for line := range strings.SplitSeq(got, "\n") {
			if key, _, ok := strings.Cut(line, " "); ok && strings.HasPrefix(key, "keypair/") {
				compatRequireKeypair(ctx, t, eng, key)
			}
		}
		t.Logf("verify: checkpoint %d, %d objects and keypairs match, %s", checkpointHeight(ctx, t, so), strings.Count(got, "\n"), compatReencode(ctx, t, so))
		return
	}

	// Store a keypair below a checkpoint, then one above it.
	compatStoreKeypair(ctx, t, eng)
	for i := range sobject.MinCheckpointOperations + 1 {
		compatCreate(ctx, t, eng, "fill-"+strconv.Itoa(i))
	}
	waitCheckpoint(ctx, t, so)
	if os.Getenv("SPACEWAVE_SR_COMPAT_ABOVE_KEYPAIR") != "0" {
		compatStoreKeypair(ctx, t, eng)
	}
	compatCreate(ctx, t, eng, "above")
	data, err := soRef.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	roots := compatRoots(ctx, t, eng)
	if err := os.WriteFile(refPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootsPath, []byte(roots), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("write: checkpoint %d, %d objects, %s", checkpointHeight(ctx, t, so), strings.Count(roots, "\n"), compatReencode(ctx, t, so))
}

// compatOpenSpace opens the local account on s4db at dir and returns the
// testbed and the Space, created when soRef is nil.
func compatOpenSpace(ctx context.Context, t *testing.T, dir string, soRef *sobject.SharedObjectRef) (*testbed.Testbed, *sobject.SharedObjectRef) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tb, err := testbed.Default(ctx, testbed.WithStorages(storage_native.NewS4db(false, dir)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(space_world.NewFactory(tb.Bus, space_world.OpTypes{Lookup: optypes.LookupWorldOp}))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, "local", "compat-account", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRef.Release)
	if soRef != nil {
		return tb, soRef
	}
	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	soRef, err = soProv.CreateSharedObject(ctx, "compat-space", &sobject.SharedObjectMeta{BodyType: "test"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return tb, soRef
}

// compatCreate commits an empty object at key.
func compatCreate(ctx context.Context, t *testing.T, eng world.Engine, key string) {
	t.Helper()
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// compatStoreKeypair stores a new keypair through KeypairUpdateOp.
func compatStoreKeypair(ctx context.Context, t *testing.T, eng world.Engine) {
	t.Helper()
	_, _, pub, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	kp, err := identity.NewKeypair(pub, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := identity_world.StoreKeypair(ctx, ws, "", kp, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// compatRequireKeypair fails unless the keypair at key reads back whole.
func compatRequireKeypair(ctx context.Context, t *testing.T, eng world.Engine, key string) {
	t.Helper()
	if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		body, err := identity_world.LookupKeypairBody(ctx, ws, key)
		if err == nil && identity_world.NewKeypairKey(body.GetPeerId()) != key {
			t.Errorf("verify: keypair %s read back peer %q", key, body.GetPeerId())
		}
		return err
	}); err != nil {
		t.Fatalf("verify: keypair %s: %v", key, err)
	}
}

// compatRoots lists every object key with its root ref, one per line.
func compatRoots(ctx context.Context, t *testing.T, eng world.Engine) string {
	t.Helper()
	var out strings.Builder
	if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		iter := ws.IterateObjects(ctx, "", false)
		defer iter.Close()
		for iter.Next() {
			obj, _, err := ws.GetObject(ctx, iter.Key())
			if err != nil {
				world.ReleaseObjectState(obj)
				return err
			}
			root, _, err := obj.GetRootRef(ctx)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			out.WriteString(iter.Key() + " " + root.GetRootRef().MarshalString() + "\n")
		}
		return iter.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// compatReencode decodes each stored World operation above the checkpoint
// with this build's types, encodes it again, and reports how many encode to
// their stored bytes.
func compatReencode(ctx context.Context, t *testing.T, so sobject.SharedObject) string {
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var total, same int
	for _, h := range set.Order() {
		inner := set.Get(h)
		if inner.IsAcknowledgment() {
			continue
		}
		_, data, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
		if err != nil || reason != "" {
			t.Fatalf("stored operation %d: %v %s", total, err, reason)
		}
		op := &space_world.WorldOp{}
		if err := op.UnmarshalVT(data); err != nil {
			t.Fatalf("stored operation %d: decode: %v", total, err)
		}
		again, err := op.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		total++
		if bytes.Equal(again, data) {
			same++
		}
	}
	return strconv.Itoa(same) + "/" + strconv.Itoa(total) + " stored operations re-encode to their bytes"
}
