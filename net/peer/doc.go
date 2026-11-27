// Package peer identifies a network identity by its public key.
//
// An ID is the IDENTITY multihash of a marshaled public key, so the key itself
// is recoverable from the ID (ExtractPublicKey) and no lookup is needed to check
// a signature. MatchesPublicKey and MatchesPrivateKey compare a key against an
// ID. The string form is base58 (IDB58Encode, IDB58Decode). A Peer is an ID
// with its public key and, when held, its private key. GetPrivKey returns
// ErrNoPrivKey for a peer known only by its public key.
//
// The GetPeer directive asks the bus for a local Peer, optionally by ID. The
// directive lets a transport or a session find the private key to sign with.
// Signature and SignedMsg sign data under a peer's key, and DeriveKey derives
// a secret from a private key under a caller-chosen context string, which must
// be globally unique to the application and purpose.
package peer
