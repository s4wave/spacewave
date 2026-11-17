package session

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
)

// CheckMountable returns ErrAccountDeleting when the session list on b holds
// an entry of ref's provider account in a state other than ACTIVE. Every
// MountSession resolves through it before the account opens.
//
// Invariant: no mount opens an account with a DELETING or UNLINKING entry.
// Opening one would recreate a volume the delete removed, or mint a key for a
// session whose key it removed; past the volume delete no mark remains on
// disk, so the list is the only record left to refuse on.
//
// The check is by account, not by session, so a session of the account that
// the list does not name is refused too. A bus with no session controller,
// such as a provider's own tests, passes.
func CheckMountable(ctx context.Context, b bus.Bus, ref *SessionRef) error {
	// Read the session list, if a session controller runs.
	ctrl, ctrlRef, err := ExLookupSessionController(ctx, b, "", true, nil)
	if err != nil || ctrlRef == nil {
		return err
	}
	defer ctrlRef.Release()
	entries, err := ctrl.ListSessions(ctx)
	if err != nil {
		return err
	}

	// Refuse when any entry of the account is past ACTIVE.
	provRef := ref.GetProviderResourceRef()
	for _, entry := range entries {
		entryRef := entry.GetSessionRef().GetProviderResourceRef()
		if entry.GetState() != SessionListState_SESSION_LIST_STATE_ACTIVE &&
			entryRef.GetProviderId() == provRef.GetProviderId() &&
			entryRef.GetProviderAccountId() == provRef.GetProviderAccountId() {
			return ErrAccountDeleting
		}
	}
	return nil
}
