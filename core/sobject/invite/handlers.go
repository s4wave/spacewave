package sobject_invite

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// InviteLookupFn resolves an invite by token hash.
// Returns nil result if no matching invite is found.
type InviteLookupFn func(ctx context.Context, tokenHash []byte) (*InviteLookupResult, error)

// EnrollFn enrolls a participant after invite verification, raising a lower
// role to the invite's and never lowering one. Called with the resolved invite
// context and the invitee's identity. Returns the SOGrant it made, nil when the
// invitee already held one, the outcome and the role the invitee holds after.
type EnrollFn func(ctx context.Context, result *InviteLookupResult, inviteePeerID peer.ID, inviteePubKey crypto.PubKey) (*sobject.SOGrant, sobject.SOParticipantAddOutcome, sobject.SOParticipantRole, error)

// LeaveFn commits voluntary departure while retaining the selected native host through acknowledgment.
type LeaveFn func(context.Context, *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error)

// QueueFn holds a redemption as a join request for an owner to grant or refuse.
type QueueFn func(ctx context.Context, result *InviteLookupResult, joinResp *sobject.SOJoinResponse) error

// WithdrawFn removes the pending join request of peerID to join the shared
// object, succeeding when none remains.
type WithdrawFn func(ctx context.Context, sharedObjectID string, peerID peer.ID) error

// HeldConfigFn returns the host's own configuration of a Space, or nil when
// the host does not hold it.
type HeldConfigFn func(ctx context.Context, sharedObjectID string) (*sobject.SharedObjectConfig, error)

// Handlers are the owner operations behind the invite service.
type Handlers struct {
	// Lookup resolves the owner's invitation authority.
	Lookup InviteLookupFn
	// Enroll issues participant grants under the resolved owner.
	Enroll EnrollFn
	// Leave commits authenticated voluntary departure.
	Leave LeaveFn
	// Queue holds join requests; nil refuses redemptions that need approval.
	Queue QueueFn
	// Withdraw removes join requests; nil holds none to remove.
	Withdraw WithdrawFn
	// HeldConfig reads the Spaces a participant_of invite names; nil holds none.
	HeldConfig HeldConfigFn
}
