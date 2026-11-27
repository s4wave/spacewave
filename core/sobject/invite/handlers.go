package sobject_invite

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// inviteLookupFn resolves an invite by token hash.
// Returns nil result if no matching invite is found.
type inviteLookupFn func(ctx context.Context, tokenHash []byte) (*InviteLookupResult, error)

// enrollFn enrolls a participant after invite verification, raising a lower
// role to the invite's and never lowering one. Called with the resolved invite
// context and the invitee's identity. Returns the SOGrant it made, nil when the
// invitee already held one, the outcome and the role the invitee holds after
// enrollment.
type enrollFn func(
	ctx context.Context,
	result *InviteLookupResult,
	inviteePeerID peer.ID,
	inviteePubKey crypto.PubKey,
) (*sobject.SOGrant, sobject.SOParticipantAddOutcome, sobject.SOParticipantRole, error)

// leaveFn commits voluntary departure while retaining the selected native host
// through acknowledgment.
type leaveFn func(context.Context, *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error)

// queueFn holds a redemption as a join request for an owner to grant or refuse.
// The request keeps both join responses, so a grant admits the session peer
// and its storage peer as an immediate redemption would.
type queueFn func(ctx context.Context, result *InviteLookupResult, joinResp, storageJoinResp *sobject.SOJoinResponse) error

// withdrawFn removes the pending join request of peerID to join the shared
// object, succeeding when none remains.
type withdrawFn func(ctx context.Context, sharedObjectID string, peerID peer.ID) error

// heldConfigFn returns the host's own configuration of a Space, or nil when
// the host does not hold it.
type heldConfigFn func(ctx context.Context, sharedObjectID string) (*sobject.SharedObjectConfig, error)

// Handlers are the owner operations behind the invite service.
type Handlers struct {
	// Lookup resolves the owner's invitation authority.
	Lookup inviteLookupFn
	// Enroll issues participant grants under the resolved owner.
	Enroll enrollFn
	// Leave commits authenticated voluntary departure.
	Leave leaveFn
	// Queue holds join requests. A nil Queue refuses redemptions that need
	// approval.
	Queue queueFn
	// Withdraw removes join requests. A nil Withdraw holds none to remove.
	Withdraw withdrawFn
	// HeldConfig reads the Spaces a participant_of invite selects. A nil
	// HeldConfig holds none.
	HeldConfig heldConfigFn
}
