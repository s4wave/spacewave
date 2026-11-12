//go:build !js

package sobject_world_engine_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
)

// TestSetRootsAcrossBuilds checks that a shared object World written by one
// build replays to the same World on another, so builds with and without set
// roots can share it. SPACEWAVE_SR_COMPAT_ROLE runs one step on the s4db
// store under SPACEWAVE_SR_COMPAT_DIR: "write" stores keypairs below and
// above a checkpoint and records every object root; "verify" replays,
// requires the same roots and keypairs, and requires each stored operation to
// re-encode to its stored bytes. SPACEWAVE_SR_COMPAT_ABOVE_KEYPAIR=0 leaves
// out the keypair above the checkpoint, whose staged block a build without
// set roots does not store. Run the steps with test binaries of the two
// builds.
func TestSetRootsAcrossBuilds(t *testing.T) {
	role, dir := os.Getenv("SPACEWAVE_SR_COMPAT_ROLE"), os.Getenv("SPACEWAVE_SR_COMPAT_DIR")
	if role == "" || dir == "" {
		t.Skip("runs as a step of a cross-build check")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	refPath, rootsPath := filepath.Join(dir, "so-ref"), filepath.Join(dir, "roots")

	// Open the account on the store, creating the shared object on write.
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
	storage := filepath.Join(dir, "storage")
	if err := os.MkdirAll(storage, 0o700); err != nil {
		t.Fatal(err)
	}
	tb, _, soRef := openLocalAccount(t, storage, soRef)
	relOpc, err := tb.Bus.AddController(ctx, world.NewLookupOpController("compat-ops", "compat-engine", optypes.LookupWorldOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relOpc()
	so, soMountRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer soMountRef.Release()
	eng, engRef, err := world.StartEngine(ctx, tb.Bus, sobject_world_engine.NewConfig("compat-engine", soRef), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer engRef.Release()

	if role == "verify" {
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
		t.Logf("verify: checkpoint %d, %d objects and keypairs match, %s", compatCheckpoint(ctx, t, so, false), strings.Count(got, "\n"), compatReencode(ctx, t, so))
		return
	}

	// Store a keypair below a checkpoint, then one above it.
	compatStoreKeypair(ctx, t, eng)
	for i := range sobject.MinCheckpointOperations + 1 {
		compatCreate(ctx, t, eng, "fill-"+strconv.Itoa(i))
	}
	height := compatCheckpoint(ctx, t, so, true)
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
	t.Logf("write: checkpoint %d, %d objects, %s", height, strings.Count(roots, "\n"), compatReencode(ctx, t, so))
}

// compatCheckpoint returns the height of so's checkpoint, first waiting for
// one above genesis when wait is set.
func compatCheckpoint(ctx context.Context, t *testing.T, so sobject.SharedObject, wait bool) uint64 {
	t.Helper()
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var height uint64
	_, err = ctr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		checkpoint, err := snap.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		height = checkpoint.GetHeight()
		return !wait || height != 0, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return height
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
		op := &sobject_world_engine.SOWorldOp{}
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
