// Package pairing adds a device to an account by having the person compare
// emoji on both screens.
//
// An Engine runs one pairing operation at a time for one mounted Session and
// publishes each step as a snapshot that Watch and Snapshot return. The
// offering client calls GenerateCode, which draws an eight-character code from
// A-Z and 0-9 and registers it, signed by the Session peer, at the Relay; the
// relay forgets a code after ten minutes. The other client calls ResolveCode,
// which consumes the code so it resolves once, and then CompletePeer, which
// links to the registering peer over the Session's ordinary transport, using
// the relay only for signaling. StartOnStream and StartDirect run the same
// steps on a connection that is already authenticated.
//
// Over that link the clients exchange account descriptions, the code-entering
// client picks the relationship with SelectAccount, and both show six emoji
// derived from their own private key and the other's public key, which match
// only when no one sits between them. ConfirmSAS records each side's decision;
// the account choice must finish within two minutes and the comparison within
// ten. Only after both approve does the AccountAdapter of the responsible
// provider enroll the receiving client, or merge its account when the choice
// says so; preparation may reserve keys but cannot grant access.
// BindEnrolledLink then proves the receiving key on the pairing connection,
// and Result returns the durable receiving Session.
package pairing
