package auth_method_password

import (
	"context"

	auth_method "github.com/aperturerobotics/auth/method"
	"github.com/aperturerobotics/bifrost/crypto"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MethodID is the auth method ID.
const MethodID = "password"

// ControllerID is the auth method controller ID.
const ControllerID = "auth/method/" + MethodID

// Version is the version of the password method implementation.
var Version = semver.MustParse("0.1.0")

// PasswordMethod implements password-based auth via scrypt+blake3 KDF.
type PasswordMethod struct{}

// NewPasswordMethod constructs the PasswordMethod.
func NewPasswordMethod() *PasswordMethod {
	return &PasswordMethod{}
}

// NewMethod constructs the password method as an auth method.
func NewMethod(
	ctx context.Context,
	le *logrus.Entry,
	handler auth_method.Handler,
) (auth_method.Method, error) {
	return NewPasswordMethod(), nil
}

// _ is a type assertion.
var _ auth_method.Constructor = NewMethod

// GetMethodID returns the auth method ID.
func (p *PasswordMethod) GetMethodID() string {
	return MethodID
}

// UnmarshalParameters unmarshals+validates parameters from binary.
func (p *PasswordMethod) UnmarshalParameters(data []byte) (auth_method.Parameters, error) {
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
// authSecretData is the password bytes.
func (p *PasswordMethod) Authenticate(paramsi auth_method.Parameters, authSecretData []byte) (crypto.PrivKey, error) {
	params, ok := paramsi.(*Parameters)
	if !ok {
		return nil, errors.New("params object not recognized")
	}
	if len(authSecretData) == 0 {
		return nil, errors.New("auth secret data must be set")
	}
	return deriveKey(params, authSecretData)
}

// Execute executes the auth method.
func (p *PasswordMethod) Execute(ctx context.Context) error {
	return nil
}

// Close closes all resources related to the auth method.
func (p *PasswordMethod) Close() {}

// _ is a type assertion.
var _ auth_method.Method = ((*PasswordMethod)(nil))
