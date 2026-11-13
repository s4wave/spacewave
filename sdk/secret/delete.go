package s4wave_secret

import (
	"context"
	"os"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// DeletePayloadEnv names the environment variable that, set to "off", turns off
// the removal of a Secret's payload copies. DeleteSecret then deletes only the
// name, as the generic World delete did before it, leaves every copy in place,
// lists none in Removed and sets OthersMayExist. The reread that leaves a newer
// Secret at the key untouched stays on, since it loses no data.
const DeletePayloadEnv = "SPACEWAVE_SECRET_DELETE_PAYLOAD"

// deletePayloadOff selects DeletePayloadEnv's old behavior. It is read once,
// here, at startup.
var deletePayloadOff = os.Getenv(DeletePayloadEnv) == "off"

// ErrSecretReplaced is returned when a Secret created at the key after the
// delete read it now holds the name. The delete removed the copies it read
// and leaves the newer Secret in place.
var ErrSecretReplaced = errors.New("secret: the key now names a newer Secret; it was left in place")

// ErrNotSecretPayload is returned when a held account lists the Secret's
// nested SharedObject id under another body type. The delete removes nothing,
// since the id names some other object and not this Secret's payload.
var ErrNotSecretPayload = errors.New("secret: the nested SharedObject is not a Secret payload; nothing was deleted")

// ErrSecretRefMismatch is returned when a Secret's nested_shared_object_id and
// ref name different SharedObjects. Ref is the identity readers mount, but a
// Secret that disagrees with itself cannot say which object is its payload, so
// the delete removes nothing.
var ErrSecretRefMismatch = errors.New("secret: nested_shared_object_id and ref name different SharedObjects; nothing was deleted")

// DeleteResult reports what a Secret delete removed.
type DeleteResult struct {
	// Found is true when a Secret was at the key and is now deleted.
	Found bool
	// Removed lists the payload copies this call deleted, one per account.
	Removed []*sobject.SharedObjectRef
	// OthersMayExist is true when a copy this call could not reach may
	// remain: in the creator's account on another device, or with a peer
	// the Secret was shared with or a member that enrolled in it.
	OthersMayExist bool
}

// DeleteSecret deletes the Secret at objectKey in the World of Space spaceID:
// the payload copy in every account in held that lists the Secret's nested
// SharedObject and whose payload names this Space as its owner, then the
// Secret's World metadata object. The copies go only when no other Secret in
// this World names the same payload. held names the provider accounts this
// device holds; an account outside it is never opened, since opening an
// account id creates it.
//
// A payload is deleted only when the Space that owns it has no other Secret
// naming it. The owner is the Space id the payload's first state records,
// which no later write can change. A key is not an owner: a Secret can be
// renamed, and any writer of the Space can copy a Secret's body to another
// key, in this Space or another, so neither the key nor the ref proves which
// name a payload belongs to.
//
// The states it handles:
//   - No Secret at the key: Found is false and nothing is touched.
//   - Every copy held here: each is deleted, then the name.
//   - Another Secret in this World names the same payload, as a copy or as
//     a name planted at a renamed Secret's old key: every copy is kept, the
//     name is deleted, and OthersMayExist is set. A Secret of that type
//     whose body cannot be read counts as naming it.
//   - Interrupted between copies or before the name: the Secret stays named
//     and a rerun deletes the copies left and the name. A copy already gone
//     counts as deleted and is not listed in Removed.
//   - Named but no copy held here, either after an interrupted delete or on
//     a member's device that never enrolled: the name is deleted and
//     OthersMayExist is set. Any writer may delete the name with the generic
//     World delete, so refusing would protect nothing and only hide that
//     copies may remain.
//   - Replaced between the copies and the name: ErrSecretReplaced, and the
//     newer Secret is untouched.
//   - Its ref names a SharedObject of another body type: ErrNotSecretPayload,
//     or, when its two ids disagree, ErrSecretRefMismatch. Nothing is deleted.
//   - A copy whose payload names another Space, names none (created before
//     owners were recorded, or outside a Space), or cannot be read: the copy
//     is kept, the name is deleted, and OthersMayExist is set.
//
// A non-nil error returns a nil result.
//
// OthersMayExist is set when the creator's account is not held here, when the
// first copy read lists more than one participant, when a copy was kept, or
// when no copy was read.
// A Secret never shared lists only its creator, so a false value means no
// other device held a copy as far as this device knows; it can be stale when
// a member's enrollment has not replicated here yet.
//
// Invariants:
//   - A payload copy is deleted only when its account lists it with the
//     Secret body type, its payload names spaceID as its owner, and no other
//     Secret in this World named it when the delete read the Secret. When
//     unsure, the copy is kept.
//   - Every held copy goes before the name, so an interrupted delete leaves
//     the Secret named and a rerun completes it.
//   - The name is deleted only while it still names the nested SharedObject
//     this delete read, checked and deleted in one write transaction, so a
//     newer Secret at the key is never touched.
//   - Only that one World object is deleted, never another Secret's name.
//   - A Secret whose nested_shared_object_id and ref disagree deletes
//     nothing while copies are removed; with DeletePayloadEnv off it deletes
//     only the name, as the generic World delete does.
//   - Only the held accounts are opened, since opening an account id creates
//     it.
//   - Errors name keys and ids only, never payload bytes.
//
// Inputs the delete reads before it removes a copy, who can write each, and
// why that writer cannot steer the delete to a copy it could not already
// destroy:
//   - spaceID: the caller, from the id of the Space's own SharedObject, which
//     no World writer can change.
//   - objectKey: the person deleting, who chooses what to delete.
//   - The World object at objectKey, its type and its Secret body (ref and
//     nested_shared_object_id): any writer of the Space. They only choose
//     which id to look up. A payload of another Space is kept by its owner;
//     a payload of this Space goes only when no other Secret here names it,
//     so a name planted at a renamed Secret's old key keeps the payload the
//     renamed Secret still names. The reread before the name is deleted only
//     refuses.
//   - The other Secret objects in this World, found through the type index:
//     any writer of the Space can add one, which can only make the delete
//     keep the copies.
//   - held: the caller, the accounts this device holds.
//   - The provider id in the Secret's ref: any writer of the Space. It picks
//     which provider's held accounts are visited; a copy in them still goes
//     only by the rules above.
//   - The account's list entry for the id and its body type: the account's
//     own sessions, or its provider. The id is taken from the ref, so the
//     entry cannot point the delete elsewhere; a wrong body type only
//     refuses.
//   - The payload's owner: fixed by the payload's first state, which its
//     creator writes. The fold every member runs rejects any operation that
//     changes it, and a checkpoint carries state only under an owner's
//     signature. An owner can already destroy the value by signing a
//     checkpoint, though not delete the copies in other accounts. A write
//     that names no owner keeps the current one.
//   - The participant list: the payload's owners. It only sets
//     OthersMayExist and never chooses what is deleted.
//
// The one window: the other Secrets are read once, with the Secret, before
// any copy goes, and the copies cannot be removed in the World transaction
// that deletes the name. A Secret that names the payload from after that
// read until the copies are gone, such as a rename of objectKey to another
// key or a copy of its body, is left naming a deleted payload: its reads
// fail and deleting it removes only the name. Such a name refers to the
// Secret this delete was asked to remove, so no value outlives a delete
// that was not asked for. Checking again in the name's transaction would
// scan every Secret while holding the World's writer and could not bring
// back the copies, so it is not done.
//
// DeletePayloadEnv set to off keeps every copy and deletes only the name.
func DeleteSecret(
	ctx context.Context,
	b bus.Bus,
	engine world.Engine,
	spaceID, objectKey string,
	held []*provider.ProviderResourceRef,
) (*DeleteResult, error) {
	return deleteSecret(ctx, b, engine, spaceID, objectKey, held, nil)
}

// deleteSecret is DeleteSecret with a hook that runs after each held account
// is visited, given the number visited, so tests can interrupt or race the
// window between two copies or between the copies and the name.
func deleteSecret(
	ctx context.Context,
	b bus.Bus,
	engine world.Engine,
	spaceID, objectKey string,
	held []*provider.ProviderResourceRef,
	afterAccount func(visited int) error,
) (*DeleteResult, error) {
	// Read the Secret's nested SharedObject ref from a snapshot.
	if objectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}
	secret, namedElsewhere, err := lookupSecret(ctx, engine, objectKey)
	if err != nil || secret == nil {
		if err != nil {
			return nil, err
		}
		return &DeleteResult{}, nil
	}
	nested := secret.GetRef().GetProviderResourceRef()
	nestedID := nested.GetId()

	// Delete the copy in each held account of the Secret's provider, unless
	// another Secret here still names it.
	res := &DeleteResult{Found: true}
	accounts := heldAccountIDs(held, nested.GetProviderId())
	if deletePayloadOff || namedElsewhere {
		accounts = nil
	}
	res.OthersMayExist = !slices.Contains(accounts, nested.GetProviderAccountId())
	var read bool
	for i, accountID := range accounts {
		if nestedID != "" {
			removed, kept, shared, err := deleteCopy(ctx, b, nested.GetProviderId(), accountID, nestedID, spaceID, !read)
			if err != nil {
				return nil, err
			}
			res.OthersMayExist = res.OthersMayExist || kept
			if removed != nil {
				res.Removed = append(res.Removed, removed)
				res.OthersMayExist = res.OthersMayExist || shared
				read = true
			}
		}
		if afterAccount != nil {
			if err := afterAccount(i + 1); err != nil {
				return nil, err
			}
		}
	}
	res.OthersMayExist = res.OthersMayExist || !read

	// Reread the name in the write transaction that will delete it.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	current, err := lookupSecretIn(ctx, tx, objectKey)
	if err != nil {
		return nil, err
	}

	// Delete the name only while it still names the payload just deleted.
	if current == nil {
		return res, nil
	}
	if current.GetRef().GetProviderResourceRef().GetId() != nestedID {
		return nil, ErrSecretReplaced
	}
	if _, err := tx.DeleteObject(ctx, objectKey); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

// heldAccountIDs returns the ids of the held accounts of providerID, without
// duplicates, in the order given.
func heldAccountIDs(held []*provider.ProviderResourceRef, providerID string) []string {
	var ids []string
	seen := make(map[string]struct{}, len(held))
	for _, ref := range held {
		id := ref.GetProviderAccountId()
		if _, dup := seen[id]; dup || ref.GetProviderId() != providerID || id == "" {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// lookupSecret reads the Secret at objectKey in one read transaction, and
// whether another Secret in that view names its payload. It returns nil when
// no object is at the key, and ErrSecretRefMismatch when the Secret's two ids
// disagree while copies are removed.
func lookupSecret(ctx context.Context, engine world.Engine, objectKey string) (*Secret, bool, error) {
	// Read the Secret.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer tx.Discard()
	secret, err := lookupSecretIn(ctx, tx, objectKey)
	if err != nil || secret == nil || deletePayloadOff {
		return secret, false, err
	}
	nestedID := secret.GetRef().GetProviderResourceRef().GetId()
	if secret.GetNestedSharedObjectId() != nestedID {
		return nil, false, ErrSecretRefMismatch
	}
	if nestedID == "" {
		return secret, false, nil
	}

	// Look for another name of its payload in the same view.
	named, err := otherSecretNames(ctx, tx, objectKey, nestedID)
	if err != nil {
		return nil, false, err
	}
	return secret, named, nil
}

// otherSecretNames reports whether a Secret other than the one at objectKey
// names the SharedObject id by its ref or its nested_shared_object_id. It
// walks the World's type index a page at a time and reads one Secret body at
// a time, stopping at the first match. A Secret whose body cannot be read
// counts as a match, so the delete keeps the copies when unsure.
func otherSecretNames(ctx context.Context, ws world.WorldState, objectKey, id string) (bool, error) {
	var named bool
	err := world_types.IterateObjectsWithType(ctx, ws, SecretTypeID, func(key string) (bool, error) {
		// Skip the Secret being deleted.
		if key == objectKey {
			return true, nil
		}

		// Read the other Secret and compare both of its ids.
		other, err := world.LookupObjectBody[Secret](ctx, ws, key)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, ctxErr
			}
			named = true
			return false, nil
		}
		named = other.GetNestedSharedObjectId() == id || other.GetRef().GetProviderResourceRef().GetId() == id
		return !named, nil
	})
	return named, err
}

// lookupSecretIn reads the Secret at objectKey. It returns nil when no object
// is at the key and refuses an object of another type.
func lookupSecretIn(ctx context.Context, ws world.WorldState, objectKey string) (*Secret, error) {
	found, err := ws.HasObject(ctx, objectKey)
	if err != nil || !found {
		return nil, err
	}
	if err := world_types.CheckObjectType(ctx, ws, objectKey, SecretTypeID); err != nil {
		return nil, err
	}
	return world.LookupObjectBody[Secret](ctx, ws, objectKey)
}

// deleteCopy deletes the SharedObject id from one held account when its
// payload names spaceID as its owner. It returns the deleted
// copy's ref, or nil when the account does not list it; kept is set when the
// account lists it but the copy stays, because its payload names another
// owner, names none or cannot be read. When readShared is set it also reports
// whether the copy lists more than one participant.
func deleteCopy(
	ctx context.Context,
	b bus.Bus,
	providerID, accountID, id, spaceID string,
	readShared bool,
) (removed *sobject.SharedObjectRef, kept, shared bool, err error) {
	// Open the held account.
	acc, rel, err := provider.ExAccessProviderAccount(ctx, b, providerID, accountID, false, nil)
	if err != nil {
		return nil, false, false, err
	}
	if acc == nil {
		return nil, false, false, errors.Errorf("provider account %s/%s is not available", providerID, accountID)
	}
	defer rel.Release()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, acc)
	if err != nil {
		return nil, false, false, err
	}

	// Find its copy, if it lists one, and keep it unless it proves its owner.
	ref, err := listedRef(ctx, soProvider, id)
	if err != nil || ref == nil {
		return nil, false, false, err
	}
	owned, shared := inspectCopy(ctx, soProvider, ref, spaceID)
	if !owned {
		return nil, true, false, nil
	}

	// Delete it; a copy another delete removed first counts as gone.
	if err := soProvider.DeleteSharedObject(ctx, id); err != nil {
		if errors.Is(err, sobject.ErrSharedObjectNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	return ref, false, readShared && shared, nil
}

// listedRef returns the account's ref to the SharedObject id, or nil when the
// account does not list it. It returns ErrNotSecretPayload when the account
// lists the id under another body type.
func listedRef(ctx context.Context, soProvider sobject.SharedObjectProvider, id string) (*sobject.SharedObjectRef, error) {
	// Scan the account's list for the id.
	list, rel, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rel()
	for _, entry := range list.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() != id {
			continue
		}
		if entry.GetMeta().GetBodyType() != SecretBodyType {
			return nil, errors.Wrapf(ErrNotSecretPayload, "shared object %s", id)
		}
		return entry.GetRef().CloneVT(), nil
	}
	return nil, nil
}

// inspectCopy mounts the copy and reports whether its payload names spaceID
// and objectKey as its owner, and whether its config lists more than its
// creator. A copy it cannot read is not owned, so the delete keeps it.
func inspectCopy(
	ctx context.Context,
	soProvider sobject.SharedObjectProvider,
	ref *sobject.SharedObjectRef,
	spaceID string,
) (owned, shared bool) {
	// Mount the copy and read its payload's owner.
	so, rel, err := soProvider.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return false, true
	}
	defer rel()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return false, true
	}
	payload, err := ReadSecretPayloadFromSnapshot(ctx, snap)
	if err != nil {
		return false, true
	}
	owned = spaceID != "" && payload.GetOwnerSpaceId() == spaceID

	// Count its participants.
	ih, ok := so.(sobject.InviteHost)
	if !ok {
		return owned, true
	}
	state, err := ih.GetSOHost().GetHostState(ctx)
	if err != nil {
		return owned, true
	}
	return owned, len(state.GetConfig().GetParticipants()) != 1
}
