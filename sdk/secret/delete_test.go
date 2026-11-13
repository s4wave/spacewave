package s4wave_secret_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/testbed"
)

// testSpaceID is the Space the tests' Secrets are created and deleted in.
const testSpaceID = "space-test"

// TestDeleteSecretRemovesNameAndPayload deletes one Secret, checks that its
// name and payload are gone and that a second Secret in the same Space stays
// readable, then deletes an empty key.
func TestDeleteSecretRemovesNameAndPayload(t *testing.T) {
	// Store two Secrets with fake values.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	gone := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/gone", "fake-value-gone")
	kept := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/kept", "fake-value-kept")

	// Delete the first and check neither its name nor its payload remains,
	// and that an unshared Secret held here reports no other copy.
	held := heldOf(gone)
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/gone", held)
	if err != nil || !res.Found || len(res.Removed) != 1 || res.OthersMayExist {
		t.Fatalf("DeleteSecret = %+v, %v; want found, one removed, no others", res, err)
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/gone") {
		t.Fatal("the deleted Secret is still named at its key")
	}
	if slices.Contains(sharedObjectIDs(ctx, t, soProvider), gone.GetNestedSharedObjectId()) {
		t.Fatal("the deleted Secret's payload SharedObject remains")
	}

	// The other Secret keeps its name, its payload and its value.
	expectReadable(ctx, t, tb, soProvider, "secrets/test/kept", kept, "fake-value-kept")

	// A key with nothing at it reports not found.
	res, err = s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/never", held)
	if err != nil || res.Found {
		t.Fatalf("DeleteSecret at an empty key = %+v, %v; want not found", res, err)
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/kept", kept, "fake-value-kept")
}

// TestDeleteSecretInterruptedCompletesOnRerun stops a delete after the payload
// is gone and checks that the Secret stays named, a read fails without the
// payload, and a second delete completes it.
func TestDeleteSecretInterruptedCompletesOnRerun(t *testing.T) {
	// Store two Secrets with fake values.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	cut := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/cut", "fake-value-cut")
	kept := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/kept", "fake-value-kept")

	// Interrupt the delete between the payload and the name.
	interrupted := errors.New("interrupted")
	held := heldOf(cut)
	_, err := s4wave_secret.DeleteSecretWithHook(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/cut", held, func(int) error {
		return interrupted
	})
	if !errors.Is(err, interrupted) {
		t.Fatalf("interrupted delete error = %v", err)
	}

	// The Secret is still named, its payload is gone, and a reader gets an
	// error rather than a value.
	if !hasObject(ctx, t, tb.Engine, "secrets/test/cut") {
		t.Fatal("an interrupted delete left the payload unnamed")
	}
	if slices.Contains(sharedObjectIDs(ctx, t, soProvider), cut.GetNestedSharedObjectId()) {
		t.Fatal("the payload SharedObject remains after step two")
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	payload, err := s4wave_secret.ReadSecretPayload(readCtx, tb.Bus, cut)
	cancel()
	if err == nil {
		t.Fatalf("a read in the window returned a payload of %d bytes", len(payload.GetValue()))
	}
	t.Logf("a read in the window fails with: %v", err)
	expectReadable(ctx, t, tb, soProvider, "secrets/test/kept", kept, "fake-value-kept")

	// Running the delete again completes it. With no copy left to read, it
	// cannot rule out copies elsewhere and says so.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/cut", held)
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("rerun DeleteSecret = %+v, %v; want found, none removed, others may exist", res, err)
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/cut") {
		t.Fatal("the rerun left the Secret named")
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/kept", kept, "fake-value-kept")
}

// TestDeleteSecretLeavesNewerSecretAtKey creates a new Secret at the key after
// the delete removed the old payload and checks that the delete refuses and
// the new Secret stays intact and readable.
func TestDeleteSecretLeavesNewerSecretAtKey(t *testing.T) {
	// Store the Secret the delete will read.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	old := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/raced", "fake-value-old")

	// Between the payload and the name, another writer replaces the Secret.
	var newer *s4wave_secret.Secret
	_, err := s4wave_secret.DeleteSecretWithHook(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/raced", heldOf(old), func(int) error {
		tx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		if _, err := tx.DeleteObject(ctx, "secrets/test/raced"); err != nil {
			tx.Discard()
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		newer = createFakeSecret(ctx, t, tb, soProvider, "secrets/test/raced", "fake-value-new")
		return nil
	})
	if !errors.Is(err, s4wave_secret.ErrSecretReplaced) {
		t.Fatalf("delete over a newer Secret error = %v; want ErrSecretReplaced", err)
	}

	// The newer Secret keeps its name, payload and value.
	expectReadable(ctx, t, tb, soProvider, "secrets/test/raced", newer, "fake-value-new")
}

// TestDeleteSecretRemovesEveryHeldCopy holds the Secret's payload in a
// second account, as an enrolled member's account does, and checks that the
// delete removes both copies, and that a device holding only the second
// account removes its copy and the name and reports that the creator's copy
// may remain.
func TestDeleteSecretRemovesEveryHeldCopy(t *testing.T) {
	// Store three Secrets and a second copy of each in another account.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	member, memberRef, releaseMember := openSecondAccount(ctx, t, tb)
	defer releaseMember()
	both := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/both", "fake-value-both")
	remote := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/remote", "fake-value-remote")
	kept := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/kept", "fake-value-kept")
	for _, secret := range []*s4wave_secret.Secret{both, remote, kept} {
		copyToAccount(ctx, t, tb, member, secret)
	}

	// A device holding both accounts removes both copies, then the name.
	held := append(heldOf(both), memberRef)
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/both", held)
	if err != nil || !res.Found || len(res.Removed) != 2 {
		t.Fatalf("DeleteSecret = %+v, %v; want found, two removed", res, err)
	}
	for _, p := range []sobject.SharedObjectProvider{soProvider, member} {
		if slices.Contains(sharedObjectIDs(ctx, t, p), both.GetNestedSharedObjectId()) {
			t.Fatal("a held copy of the deleted Secret remains")
		}
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/both") {
		t.Fatal("the deleted Secret is still named at its key")
	}

	// A member device without the creator's account removes its own copy and
	// the name, leaves the creator's copy unopened, and reports it.
	res, err = s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/remote", []*provider.ProviderResourceRef{memberRef})
	if err != nil || !res.Found || len(res.Removed) != 1 || !res.OthersMayExist {
		t.Fatalf("member DeleteSecret = %+v, %v; want found, one removed, others may exist", res, err)
	}
	if res.Removed[0].GetProviderResourceRef().GetProviderAccountId() != memberRef.GetProviderAccountId() {
		t.Fatal("the member delete removed a copy outside its own account")
	}
	if !slices.Contains(sharedObjectIDs(ctx, t, soProvider), remote.GetNestedSharedObjectId()) {
		t.Fatal("the member delete reached the creator's account")
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/remote") {
		t.Fatal("the member delete left the Secret named")
	}

	// The third Secret keeps both copies and its value.
	expectReadable(ctx, t, tb, soProvider, "secrets/test/kept", kept, "fake-value-kept")
	if !slices.Contains(sharedObjectIDs(ctx, t, member), kept.GetNestedSharedObjectId()) {
		t.Fatal("another Secret lost its second copy")
	}
}

// TestDeleteSecretCrashBetweenCopiesCompletesOnRerun stops a delete after the
// first of two copies is gone and checks that the Secret stays named with
// its second copy, and that a rerun removes that copy and the name.
func TestDeleteSecretCrashBetweenCopiesCompletesOnRerun(t *testing.T) {
	// Store the Secret and a second copy in another account.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	member, memberRef, releaseMember := openSecondAccount(ctx, t, tb)
	defer releaseMember()
	cut := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/cut", "fake-value-cut")
	copyToAccount(ctx, t, tb, member, cut)
	held := append(heldOf(cut), memberRef)

	// Interrupt the delete after the first account.
	interrupted := errors.New("interrupted")
	_, err := s4wave_secret.DeleteSecretWithHook(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/cut", held, func(visited int) error {
		if visited == 1 {
			return interrupted
		}
		return nil
	})
	if !errors.Is(err, interrupted) {
		t.Fatalf("interrupted delete error = %v", err)
	}
	if !hasObject(ctx, t, tb.Engine, "secrets/test/cut") {
		t.Fatal("an interrupted delete left a copy unnamed")
	}
	if slices.Contains(sharedObjectIDs(ctx, t, soProvider), cut.GetNestedSharedObjectId()) {
		t.Fatal("the first copy remains after the first account")
	}
	if !slices.Contains(sharedObjectIDs(ctx, t, member), cut.GetNestedSharedObjectId()) {
		t.Fatal("the second copy went before its account was visited")
	}

	// Running the delete again removes the second copy and the name.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/cut", held)
	if err != nil || !res.Found || len(res.Removed) != 1 {
		t.Fatalf("rerun DeleteSecret = %+v, %v; want found, one removed", res, err)
	}
	if slices.Contains(sharedObjectIDs(ctx, t, member), cut.GetNestedSharedObjectId()) {
		t.Fatal("the rerun left the second copy")
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/cut") {
		t.Fatal("the rerun left the Secret named")
	}
}

// TestDeleteSecretPayloadOffKeepsOldBehavior selects the old behavior and
// checks that the delete removes only the name, keeps the payload copy, and
// reports that copies remain.
func TestDeleteSecretPayloadOffKeepsOldBehavior(t *testing.T) {
	// Store a Secret and select the old behavior.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	s4wave_secret.SetDeletePayloadOff(t, true)
	old := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/oldway", "fake-value-oldway")

	// The delete removes the name and reports the copy it left.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/oldway", heldOf(old))
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("DeleteSecret = %+v, %v; want found, none removed, others may exist", res, err)
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/oldway") {
		t.Fatal("the old behavior left the Secret named")
	}
	if !slices.Contains(sharedObjectIDs(ctx, t, soProvider), old.GetNestedSharedObjectId()) {
		t.Fatal("the old behavior removed the payload copy")
	}
}

// TestDeleteSecretRefusesNonSecretPayload writes a Secret, as any World writer
// can, whose ref names a SharedObject of another body type held in the
// creator's account, and checks that the delete refuses, removes nothing, and
// leaves that SharedObject listed and the Secret named.
func TestDeleteSecretRefusesNonSecretPayload(t *testing.T) {
	// Hold a SharedObject that is not a Secret payload.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	victimID := "victim-" + sobject.NewSOOperationLocalID()
	victim, err := soProvider.CreateSharedObject(ctx, victimID, &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Name it from a forged Secret and try to delete that Secret.
	writeSecret(ctx, t, tb.Engine, "secrets/test/forged", &s4wave_secret.Secret{
		NestedSharedObjectId: victimID,
		Ref:                  victim.CloneVT(),
	})
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/forged", heldOf(&s4wave_secret.Secret{Ref: victim}))
	if !errors.Is(err, s4wave_secret.ErrNotSecretPayload) || res != nil {
		t.Fatalf("delete of a forged Secret = %+v, %v; want nil, ErrNotSecretPayload", res, err)
	}

	// The other SharedObject stays listed and mounts, and the name stays.
	if !slices.Contains(sharedObjectIDs(ctx, t, soProvider), victimID) {
		t.Fatal("the delete removed a SharedObject that is not a Secret payload")
	}
	_, rel, err := soProvider.MountSharedObject(ctx, victim, nil)
	if err != nil {
		t.Fatalf("mount the kept SharedObject: %v", err)
	}
	rel()
	if !hasObject(ctx, t, tb.Engine, "secrets/test/forged") {
		t.Fatal("a refused delete removed the Secret's name")
	}
}

// TestDeleteSecretRefusesRefMismatch writes a Secret whose
// nested_shared_object_id names one real Secret payload and whose ref names
// another, as a migration that remapped only one of them left it, and checks
// that the delete refuses and both payloads stay readable.
func TestDeleteSecretRefusesRefMismatch(t *testing.T) {
	// Store two Secrets and a third name mixing their ids.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	first := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/first", "fake-value-first")
	second := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/second", "fake-value-second")
	writeSecret(ctx, t, tb.Engine, "secrets/test/mixed", &s4wave_secret.Secret{
		NestedSharedObjectId: first.GetNestedSharedObjectId(),
		Ref:                  second.GetRef().CloneVT(),
	})

	// The delete refuses and removes nothing.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/mixed", heldOf(first))
	if !errors.Is(err, s4wave_secret.ErrSecretRefMismatch) || res != nil {
		t.Fatalf("delete of a mixed Secret = %+v, %v; want nil, ErrSecretRefMismatch", res, err)
	}
	if !hasObject(ctx, t, tb.Engine, "secrets/test/mixed") {
		t.Fatal("a refused delete removed the Secret's name")
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/first", first, "fake-value-first")
	expectReadable(ctx, t, tb, soProvider, "secrets/test/second", second, "fake-value-second")
}

// TestDeleteSecretKeepsPayloadOfCopiedSecret writes a second Secret whose
// body is copied from another, as any World writer can since Secret metadata
// is readable by every member, and checks that deleting the copy removes its
// name but keeps the original's payload, which still reads.
func TestDeleteSecretKeepsPayloadOfCopiedSecret(t *testing.T) {
	// Store a Secret and a second name copied from it.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	y := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/y", "fake-value-y")
	writeSecret(ctx, t, tb.Engine, "secrets/test/x", y.CloneVT())

	// Deleting the copied name keeps the payload and says a copy remains.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/x", heldOf(y))
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("delete of a copied Secret = %+v, %v; want found, none removed, others may exist", res, err)
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/x") {
		t.Fatal("the copied Secret is still named")
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/y", y, "fake-value-y")
}

// TestDeleteSecretKeepsPayloadAcrossSpaces writes, in another Space's World and
// at the same key, a Secret copied from one in this Space, and checks that
// deleting it there keeps this Space's payload, which still reads.
func TestDeleteSecretKeepsPayloadAcrossSpaces(t *testing.T) {
	// Store a Secret here and a copy at the same key in another World.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	y := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/y", "fake-value-y")
	other, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()
	writeSecret(ctx, t, other.Engine, "secrets/test/y", y.CloneVT())

	// Deleting it in the other Space keeps the payload.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, other.Engine, "space-other", "secrets/test/y", heldOf(y))
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("delete in another Space = %+v, %v; want found, none removed, others may exist", res, err)
	}
	if hasObject(ctx, t, other.Engine, "secrets/test/y") {
		t.Fatal("the other Space's Secret is still named")
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/y", y, "fake-value-y")
}

// TestDeleteSecretKeepsUnownedPayload stores a Secret whose payload names no
// owner, as payloads written before owners were recorded do, has a writer of
// the payload try to bind it to the Secret's Space, and checks that
// the fold rejects the bind and the delete removes the name but keeps the
// payload, which still reads with no owner.
func TestDeleteSecretKeepsUnownedPayload(t *testing.T) {
	// Store a Secret whose payload names no owner.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	old, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.Engine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/test/unowned",
		DisplayName: "secrets/test/unowned",
		Kind:        s4wave_secret.SecretKindProviderCredential,
		ContentType: s4wave_secret.ProviderCredentialContentType,
		Value:       []byte("fake-value-unowned"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// A writer binding it to this Space is rejected.
	bind := &s4wave_secret.SecretPayload{
		Value:        []byte("fake-value-bound"),
		Version:      2,
		OwnerSpaceId: testSpaceID,
	}
	if err := s4wave_secret.StoreSecretPayload(ctx, tb.Bus, old.GetRef(), bind); !errors.Is(err, sobject.ErrRejectedOp) {
		t.Fatalf("binding an unowned payload = %v; want ErrRejectedOp", err)
	}

	// The delete removes the name and keeps the payload.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/unowned", heldOf(old))
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("delete of an unowned payload = %+v, %v; want found, none removed, others may exist", res, err)
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/unowned") {
		t.Fatal("the delete left the Secret named")
	}
	expectPayload(ctx, t, tb, old, "fake-value-unowned", "")
}

// TestDeleteSecretRebindKeepsOwnedPayload has a writer of a payload rebind it
// to another Space, writes a Secret there whose ref names it, and
// deletes that Secret. The fold rejects the rebind, so the delete keeps the
// payload, which still reads with its first owner. The writer here is the
// creator: every member folds with the same rule, whatever its role.
func TestDeleteSecretRebindKeepsOwnedPayload(t *testing.T) {
	// Store a Secret and try to rebind its payload to another Space.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	y := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/y", "fake-value-y")
	rebind := &s4wave_secret.SecretPayload{
		Value:        []byte("fake-value-y"),
		Version:      2,
		OwnerSpaceId: "space-other",
	}
	if err := s4wave_secret.StoreSecretPayload(ctx, tb.Bus, y.GetRef(), rebind); !errors.Is(err, sobject.ErrRejectedOp) {
		t.Fatalf("rebinding an owned payload = %v; want ErrRejectedOp", err)
	}

	// Name it at the forged key in the other Space and delete that name.
	other, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()
	writeSecret(ctx, t, other.Engine, "secrets/test/forged", y.CloneVT())
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, other.Engine, "space-other", "secrets/test/forged", heldOf(y))
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("delete of the forged Secret = %+v, %v; want found, none removed, others may exist", res, err)
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/y", y, "fake-value-y")
	expectPayload(ctx, t, tb, y, "fake-value-y", testSpaceID)
}

// TestSecretPayloadRewriteKeepsOwner rewrites a Secret's value as a granted
// writer and checks that the new value reads with the first owner, so the
// owner rule leaves ordinary writes alone. A write that names no owner, as a
// writer on a build before owners sends, is accepted and keeps the owner.
func TestSecretPayloadRewriteKeepsOwner(t *testing.T) {
	// Store a Secret and rewrite its value as its writer.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	y := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/y", "fake-value-y")
	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, y.GetRef(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	writer := so.GetPeerID().String()
	soRef.Release()
	err = s4wave_secret.WriteSecretPayloadForPeer(ctx, tb.Bus, y, y.GetKind(), writer, s4wave_secret.ProviderCredentialContentType, []byte("fake-value-y2"))
	if err != nil {
		t.Fatal(err)
	}

	// The new value reads with the first owner.
	expectPayload(ctx, t, tb, y, "fake-value-y2", testSpaceID)

	// A write without an owner is accepted and keeps the owner.
	unnamed := &s4wave_secret.SecretPayload{Value: []byte("fake-value-y3"), Version: 3}
	if err := s4wave_secret.StoreSecretPayload(ctx, tb.Bus, y.GetRef(), unnamed); err != nil {
		t.Fatalf("a write without an owner = %v; want accepted", err)
	}
	expectPayload(ctx, t, tb, y, "fake-value-y3", testSpaceID)
}

// expectPayload checks that secret's payload reads back as value and names
// spaceID as its owner.
func expectPayload(ctx context.Context, t *testing.T, tb *testbed.Testbed, secret *s4wave_secret.Secret, value, spaceID string) {
	t.Helper()
	payload, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, secret)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if string(payload.GetValue()) != value {
		t.Fatal("the payload reads back a different value")
	}
	if payload.GetOwnerSpaceId() != spaceID {
		t.Fatalf("payload owner = %q; want %q", payload.GetOwnerSpaceId(), spaceID)
	}
}

// TestDeleteSecretKeepsPayloadOfRenamedSecret renames a Secret, plants a
// copy of its body at the old key, as any Space writer can, and deletes the
// planted name. The renamed Secret still names the payload, so every copy is
// kept and the renamed Secret still reads. Deleting the renamed Secret once
// the plant is gone then removes the payload.
func TestDeleteSecretKeepsPayloadOfRenamedSecret(t *testing.T) {
	// Rename a Secret and plant a copy of its body at the old key.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	y := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/k1", "fake-value-y")
	renameObject(ctx, t, tb.Engine, "secrets/test/k1", "secrets/test/k2")
	writeSecret(ctx, t, tb.Engine, "secrets/test/k1", y.CloneVT())

	// Deleting the plant keeps the payload the renamed Secret names.
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/k1", heldOf(y))
	if err != nil || !res.Found || len(res.Removed) != 0 || !res.OthersMayExist {
		t.Fatalf("delete of the planted name = %+v, %v; want found, none removed, others may exist", res, err)
	}
	if hasObject(ctx, t, tb.Engine, "secrets/test/k1") {
		t.Fatal("the planted name is still at the old key")
	}
	expectReadable(ctx, t, tb, soProvider, "secrets/test/k2", y, "fake-value-y")

	// With the plant gone, deleting the renamed Secret removes the payload.
	res, err = s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/k2", heldOf(y))
	if err != nil || !res.Found || len(res.Removed) != 1 {
		t.Fatalf("delete of the renamed Secret = %+v, %v; want found, one removed", res, err)
	}
	if slices.Contains(sharedObjectIDs(ctx, t, soProvider), y.GetNestedSharedObjectId()) {
		t.Fatal("the renamed Secret's payload remains")
	}
}

// TestDeleteSecretRemovesPayloadOfRenamedSecret renames a Secret and deletes
// it at its new key: the payload's owner is the Space, not the key, so the
// payload goes with it.
func TestDeleteSecretRemovesPayloadOfRenamedSecret(t *testing.T) {
	// Rename a Secret and delete it at its new key.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()
	y := createFakeSecret(ctx, t, tb, soProvider, "secrets/test/k1", "fake-value-y")
	renameObject(ctx, t, tb.Engine, "secrets/test/k1", "secrets/test/k2")
	res, err := s4wave_secret.DeleteSecret(ctx, tb.Bus, tb.Engine, testSpaceID, "secrets/test/k2", heldOf(y))
	if err != nil || !res.Found || len(res.Removed) != 1 {
		t.Fatalf("delete of the renamed Secret = %+v, %v; want found, one removed", res, err)
	}

	// Neither the name nor the payload remains.
	if hasObject(ctx, t, tb.Engine, "secrets/test/k2") {
		t.Fatal("the renamed Secret is still named")
	}
	if slices.Contains(sharedObjectIDs(ctx, t, soProvider), y.GetNestedSharedObjectId()) {
		t.Fatal("the renamed Secret's payload remains")
	}
}

// renameObject renames the World object at oldKey to newKey, as any World
// writer can.
func renameObject(ctx context.Context, t *testing.T, engine world.Engine, oldKey, newKey string) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, err := tx.RenameObject(ctx, oldKey, newKey, false)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// writeSecret writes secret as the Secret World object at objectKey without
// creating a payload, as any World writer can.
func writeSecret(ctx context.Context, t *testing.T, engine world.Engine, objectKey string, secret *s4wave_secret.Secret) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, _, err := world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tx, objectKey, s4wave_secret.SecretTypeID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// heldOf returns the held account list naming the account that created secret.
func heldOf(secret *s4wave_secret.Secret) []*provider.ProviderResourceRef {
	return []*provider.ProviderResourceRef{secret.GetRef().GetProviderResourceRef()}
}

// openSecondAccount opens another account on the test's local provider.
func openSecondAccount(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
) (sobject.SharedObjectProvider, *provider.ProviderResourceRef, func()) {
	t.Helper()
	ref := &provider.ProviderResourceRef{ProviderId: "local", ProviderAccountId: "test-member-" + sobject.NewSOOperationLocalID()}
	acc, accRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, ref.GetProviderId(), ref.GetProviderAccountId(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
	if err != nil {
		accRef.Release()
		t.Fatal(err)
	}
	return soProvider, ref, accRef.Release
}

// copyToAccount creates a SharedObject at the Secret's nested id in another
// account holding the same payload, owner included, standing in for the copy
// enrollment installs there. It is an independent object with one
// participant, so it says nothing about how many participants a real
// enrolled copy lists.
func copyToAccount(ctx context.Context, t *testing.T, tb *testbed.Testbed, soProvider sobject.SharedObjectProvider, secret *s4wave_secret.Secret) {
	t.Helper()
	payload, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, secret)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := soProvider.CreateSharedObject(ctx, secret.GetNestedSharedObjectId(), s4wave_secret.NewSharedObjectMeta(), "", "")
	if err != nil {
		t.Fatalf("copy %s: %v", secret.GetNestedSharedObjectId(), err)
	}
	if err := s4wave_secret.StoreSecretPayload(ctx, tb.Bus, ref, payload); err != nil {
		t.Fatal(err)
	}
}

// createFakeSecret stores a provider credential Secret holding a fake value.
func createFakeSecret(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	soProvider sobject.SharedObjectProvider,
	objectKey, value string,
) *s4wave_secret.Secret {
	t.Helper()
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.Engine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   objectKey,
		SpaceID:     testSpaceID,
		DisplayName: objectKey,
		Kind:        s4wave_secret.SecretKindProviderCredential,
		ContentType: s4wave_secret.ProviderCredentialContentType,
		Value:       []byte(value),
	})
	if err != nil {
		t.Fatalf("CreateSecret %s: %v", objectKey, err)
	}
	return secret
}

// expectReadable checks that objectKey names secret, its payload SharedObject
// is listed, and its payload reads back as value.
func expectReadable(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	soProvider sobject.SharedObjectProvider,
	objectKey string,
	secret *s4wave_secret.Secret,
	value string,
) {
	t.Helper()
	named := readParentSecret(ctx, t, tb.WorldState, objectKey)
	if named.GetNestedSharedObjectId() != secret.GetNestedSharedObjectId() {
		t.Fatalf("%s names another payload", objectKey)
	}
	if !slices.Contains(sharedObjectIDs(ctx, t, soProvider), secret.GetNestedSharedObjectId()) {
		t.Fatalf("%s lost its payload SharedObject", objectKey)
	}
	payload, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, named)
	if err != nil {
		t.Fatalf("read %s: %v", objectKey, err)
	}
	if string(payload.GetValue()) != value {
		t.Fatalf("%s reads back a different value", objectKey)
	}
}

// hasObject reports whether a World object is at objectKey.
func hasObject(ctx context.Context, t *testing.T, engine world.Engine, objectKey string) bool {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	found, err := tx.HasObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// sharedObjectIDs lists the ids of the account's SharedObjects.
func sharedObjectIDs(ctx context.Context, t *testing.T, soProvider sobject.SharedObjectProvider) []string {
	t.Helper()
	list, rel, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var ids []string
	for _, entry := range list.GetValue().GetSharedObjects() {
		ids = append(ids, entry.GetRef().GetProviderResourceRef().GetId())
	}
	return ids
}
