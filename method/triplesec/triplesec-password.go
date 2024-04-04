package auth_method_triplesec

import (
	"context"
	"errors"

	crypto_triplesec "github.com/aperturerobotics/auth/crypto/triplesec"
	auth_method "github.com/aperturerobotics/auth/method"
	"github.com/blang/semver"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/sirupsen/logrus"
)

// MethodID is the auth method ID.
const MethodID = "triplesec"

// ControllerID is the auth method controller ID.
const ControllerID = "auth/method/" + MethodID

// Version is the version of the triplesec-password implementation.
var Version = semver.MustParse("0.0.1")

// TriplesecPassword implements triplesec auth method with password.
type TriplesecPassword struct{}

// NewTriplesecPassword constructs the TriplesecPassword method.
func NewTriplesecPassword() *TriplesecPassword {
	return &TriplesecPassword{}
}

// NewMethod constructs the triplesec password as an auth method.
// Implements constructor
func NewMethod(
	ctx context.Context,
	le *logrus.Entry,
	handler auth_method.Handler,
) (auth_method.Method, error) {
	return NewTriplesecPassword(), nil
}

// _ is a type assertion
var _ auth_method.Constructor = NewMethod

// GetMethodID returns the auth method id.
// This is a unique identifier for this code / method.
func (p *TriplesecPassword) GetMethodID() string {
	return MethodID
}

// UnmarshalParameters unmarshals+validates parameters from binary.
func (p *TriplesecPassword) UnmarshalParameters(data []byte) (auth_method.Parameters, error) {
	params := &Parameters{}
	if err := params.UnmarshalVT(data); err != nil {
		return nil, err
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return params, nil
}

// Authenticate authenticates with existing auth parameters.
// Parameters are generated with either UnmarshalParameters or Register.
// Generates the private key.
func (p *TriplesecPassword) Authenticate(paramsi auth_method.Parameters, authSecretData []byte) (crypto.PrivKey, error) {
	params, ok := paramsi.(*Parameters)
	if !ok {
		return nil, errors.New("params object not recognized")
	}
	if len(authSecretData) == 0 {
		return nil, errors.New("auth secret data must be set")
	}

	salt := params.GetSalt()
	version := params.GetVersion()
	cipher, err := crypto_triplesec.BuildCipher(version, salt, authSecretData)
	if err != nil {
		return nil, err
	}
	return crypto_triplesec.DeriveED25519Key(cipher)
}

// Execute executes the auth method, yielding private keys to the handler.
// If returns nil, will not be retried.
func (p *TriplesecPassword) Execute(ctx context.Context) error {
	// noop
	return nil
}

// Close closes all resources related to the auth method.
func (p *TriplesecPassword) Close() {
	// noop
}

// _ is a type assertion
var _ auth_method.Method = ((*TriplesecPassword)(nil))
