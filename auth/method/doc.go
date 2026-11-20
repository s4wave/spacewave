// Package auth_method defines how a private key is derived from something a
// person knows or holds.
//
// A Method has a stable method ID that a Keypair records. Authenticate turns
// stored Parameters and a secret, such as a password, into the private key,
// and must yield the same key from the same inputs, so the key need never be
// stored. UnmarshalParameters decodes the parameters a Keypair carries.
// Execute may run in the background and yield keys it discovers to its
// Handler; returning nil ends it without a retry. The AuthLookupMethod
// directive finds a Method by ID on the bus.
//
// Package password derives an Ed25519 key with scrypt from the password and a
// salt derived from the username with BLAKE3, so the same username and
// password give the same key on any device. Package pem reads a key directly
// from PEM.
package auth_method
