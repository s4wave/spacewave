// Package identity names a person or service across devices and binds keys to
// that name.
//
// An Entity is an entity ID, akin to a username and a lowercase DNS label, in
// a domain, with a domain-unique UUID and an epoch that rises with each change.
// Its EntityKeypairSet lists the Keypairs that act for it; each entry is
// signed by its own private key over the keypair and the entity's IDs, so a
// key proves its own membership and a set cannot list a key whose holder did
// not sign. Validate and UnmarshalVerifyKeypairs check every signature and
// that each entry names this entity. A Keypair records its peer ID, its PEM
// public key, and the auth method ID and parameters that derive its private
// key again, such as a password's scrypt parameters; the private key itself is
// never stored.
//
// The directives connect an entity to its keys at runtime. IdentityLookupEntity
// finds an Entity by domain and entity ID, SelectIdentityEntity and
// SelectEntityId choose the active one, DeriveEntityKeypair derives a private
// key for one of its Keypairs (the auth/derive controller resolves it), and
// PromptPassword asks the person for a secret when a method needs one. Package
// world stores entities, keypairs and domain info as World objects.
package identity
