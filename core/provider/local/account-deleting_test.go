//go:build !js && !wasip1

package provider_local

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
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
