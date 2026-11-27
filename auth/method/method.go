// Package auth_method defines how a private key is derived from something a
// person knows or holds.
//
// A Method has a stable method ID that a Keypair records. Authenticate turns
// stored Parameters and a secret, such as a password, into the private key,
// and must yield the same key from the same inputs, so the key need never be
// stored. UnmarshalParameters decodes the parameters a Keypair carries.
// Execute may run in the background and yield keys it discovers to its
// Handler. Returning nil ends Execute without a retry. The AuthLookupMethod
// directive finds a Method by ID on the bus.
//
// Package password derives an Ed25519 key with scrypt from the password and a
// salt derived from the username with BLAKE3, so the same username and
// password give the same key on any device. Package pem reads a key directly
// from PEM.
package auth_method

import (
	"context"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// Parameters are authentication method params.
//
// Parameters are stored in a user record.
type Parameters interface {
	// MarshalVT marshals the block to binary.
	MarshalVT() ([]byte, error)
	// Validate validates the parameters (cursory).
	Validate() error
}

// Handler is the method handler.
// Manages "ambient-ly discovered" authentication keys.
type Handler any

// Method is an authentication method.
//
// The method likely produces Parameters to register.
type Method interface {
	// GetMethodID returns the auth method id.
	// This ID is a unique identifier for this code / method.
	GetMethodID() string
	// Execute executes the auth method, yielding private keys to the handler.
	// If Execute returns nil, the method will not be retried.
	Execute(ctx context.Context) error
	// UnmarshalParameters unmarshals+validates parameters from binary.
	UnmarshalParameters(data []byte) (Parameters, error)
	// Authenticate authenticates with existing auth parameters.
	// Parameters are generated with either UnmarshalParameters or Register.
	// Generates the private key.
	Authenticate(params Parameters, authSecretData []byte) (crypto.PrivKey, error)
	// Close closes all resources related to the auth method.
	Close()
}

// Constructor constructs a method with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	handler Handler,
) (Method, error)
