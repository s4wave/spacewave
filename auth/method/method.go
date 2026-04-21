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
	// MarshalBlock marshals the block to binary.
	MarshalBlock() ([]byte, error)
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
	// This is a unique identifier for this code / method.
	GetMethodID() string
	// Execute executes the auth method, yielding private keys to the handler.
	// If returns nil, will not be retried.
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
