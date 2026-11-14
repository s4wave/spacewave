//go:build !js && !wasip1

package provider_local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/volume"
)

// TestMountRefusesDeletingAccount checks that a session of an account whose
// delete started never mounts and never gets a key: a delete that stopped
// after removing a session's key must not leave a mount that writes a new one.
func TestMountRefusesDeletingAccount(t *testing.T) {
	// Open an account and mark its delete as started.
	ctx := context.Background()
	a, ref, release := openRehearsalAccount(ctx, t, t.TempDir(), nil)
	defer release()
	provRef := ref.GetProviderResourceRef()
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.t.p.b, false, SessionObjectStoreID(provRef.GetProviderId(), provRef.GetProviderAccountId()), a.vol.GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer diRef.Release()
	store := handle.GetObjectStore()
	if err := MarkAccountDeleting(ctx, store, DeletingAccount); err != nil {
		t.Fatal(err)
	}

	// A session with no key, as the delete leaves one, does not mount.
	keyless := ref.CloneVT()
	keyless.GetProviderResourceRef().Id = ulid.NewULID()
	mctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, rel, err := a.MountSession(mctx, keyless, nil); err == nil {
		rel()
		t.Fatal("a session of a deleting account mounted")
	}

	// And no key was written for it.
	_, found, err := session_lock.ReadAutoUnlockKey(ctx, store, keyless.GetProviderResourceRef().GetId())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("the mount wrote a new key under the deleting mark")
	}
}

// TestFenceRefusesSharedObjectCreate checks that a create on a fenced account
// is refused and lists nothing, since a transfer's delete would take a Space
// its list missed with the volume, and that the account creates again once
// the fence is released.
func TestFenceRefusesSharedObjectCreate(t *testing.T) {
	// Open an account and fence it.
	ctx := context.Background()
	a, _, release := openRehearsalAccount(ctx, t, t.TempDir(), nil)
	defer release()
	before := len(a.soListCtr.GetValue().GetSharedObjects())
	unfence := a.FenceSharedObjects()

	// A create under the fence is refused and lists nothing.
	meta := &sobject.SharedObjectMeta{BodyType: "space"}
	if _, err := a.CreateSharedObject(ctx, ulid.NewULID(), meta, "", ""); !errors.Is(err, ErrSharedObjectsFenced) {
		unfence()
		t.Fatalf("expected ErrSharedObjectsFenced, got %v", err)
	}
	if n := len(a.soListCtr.GetValue().GetSharedObjects()); n != before {
		unfence()
		t.Fatalf("the fenced create listed a shared object: %d, was %d", n, before)
	}

	// Released twice, as the transfer's exit and cancel paths may, the fence
	// lets creates through.
	unfence()
	unfence()
	if _, err := a.CreateSharedObject(ctx, ulid.NewULID(), meta, "", ""); err != nil {
		t.Fatal(err)
	}
}
