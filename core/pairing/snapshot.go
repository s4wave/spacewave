package pairing

import "github.com/s4wave/spacewave/net/peer"

// Status matches the Session resource's pairing status enum.
type Status int32

const (
	// StatusIdle means no pairing operation is active.
	StatusIdle Status = iota
	// StatusCodeGenerated means this client registered a pairing code.
	StatusCodeGenerated
	// StatusWaitingForPeer means this client waits for the other client to connect.
	StatusWaitingForPeer
	// StatusPeerConnected means the pairing link to the other client is up.
	StatusPeerConnected
	// StatusVerifyingEmoji means both people compare the emoji before approving.
	StatusVerifyingEmoji
	// StatusVerified means the emoji were approved on this client.
	StatusVerified
	// StatusFailed means the operation failed; ErrMsg describes why.
	StatusFailed
	// StatusSignalingFailed means the pairing relay could not connect the clients.
	StatusSignalingFailed
	// StatusConnectionTimeout means the other client did not connect in time.
	StatusConnectionTimeout
	// StatusWaitingForRemote means this client approved and waits for the other client.
	StatusWaitingForRemote
	// StatusBothConfirmed means both clients approved the link.
	StatusBothConfirmed
	// StatusPairingRejected means the other client rejected the link.
	StatusPairingRejected
	// StatusConfirmationTimeout means the other client did not approve in time.
	StatusConfirmationTimeout
	// StatusEnrolling means the approved account enrollment is in progress.
	StatusEnrolling
	// StatusSelectingAccount means this client chooses which account to offer.
	StatusSelectingAccount
)

// Snapshot describes one Session's current pairing operation.
type Snapshot struct {
	// Status is the operation's current phase.
	Status Status
	// Code is the pairing code this client registered, when it created one.
	Code string
	// RemotePeerID is the other client's pairing link peer once it is known.
	RemotePeerID peer.ID
	// Emoji is the SAS sequence both people compare before approving.
	Emoji []string
	// ErrMsg describes the failure when Status is a failure phase.
	ErrMsg string
	// AccountID is the account selected for enrollment.
	AccountID string
	// AccountName is the selected account's display name.
	AccountName string
	// ProviderID is the provider that holds the selected account.
	ProviderID string
	// RemoteLabel is the name the other client gave itself, shown on the
	// approval screen and recorded for the Session it enrolls.
	RemoteLabel string
	// SessionPeerID is the receiving Session the approval enrolls. It differs
	// from RemotePeerID, which names the pairing link.
	SessionPeerID string
	// Receiving indicates this client is adding the selected account.
	Receiving bool
	// Choice is the proposed account relationship once both offers are known.
	Choice *AccountChoice
}
