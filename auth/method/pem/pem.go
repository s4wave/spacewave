// Package auth_method_pem implements a PEM backup key auth method.
package auth_method_pem

import (
	"context"
	"crypto/rand"

	"github.com/pkg/errors"
	auth_method "github.com/s4wave/spacewave/auth/method"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/sirupsen/logrus"
)

// MethodID is the auth method ID for PEM backup keys.
const MethodID = "pem"

// PemMethod implements the auth method interface for PEM backup keys.
type PemMethod struct{}

// NewPemMethod constructs a PemMethod.
func NewPemMethod() *PemMethod {
	return &PemMethod{}
}

// NewMethod constructs the PEM method as an auth method.
func NewMethod(
	ctx context.Context,
	le *logrus.Entry,
	handler auth_method.Handler,
) (auth_method.Method, error) {
	return NewPemMethod(), nil
}

// _ is a type assertion
var _ auth_method.Constructor = NewMethod

// GetMethodID returns the auth method ID.
func (p *PemMethod) GetMethodID() string {
	return MethodID
}

// Execute executes the auth method.
func (p *PemMethod) Execute(ctx context.Context) error {
	return nil
}

// Close closes all resources related to the auth method.
func (p *PemMethod) Close() {}

// UnmarshalParameters unmarshals+validates parameters from binary.
// The data is the PEM-encoded public key bytes.
func (p *PemMethod) UnmarshalParameters(data []byte) (auth_method.Parameters, error) {
	params := &PemParameters{PubKeyPem: data}
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return params, nil
}

// Authenticate authenticates with existing auth parameters.
// authSecretData is the full PEM private key file bytes.
func (p *PemMethod) Authenticate(paramsi auth_method.Parameters, authSecretData []byte) (crypto.PrivKey, error) {
	if _, ok := paramsi.(*PemParameters); !ok {
		return nil, errors.New("params object not recognized")
	}
	if len(authSecretData) == 0 {
		return nil, errors.New("auth secret data must be set")
	}
	privKey, err := keypem.ParsePrivKeyPem(authSecretData)
	if err != nil {
		return nil, errors.Wrap(err, "parse pem private key")
	}
	if privKey == nil {
		return nil, errors.New("no private key found in pem data")
	}
	return privKey, nil
}

// PemParameters stores the PEM-encoded public key for verification.
type PemParameters struct {
	// PubKeyPem is the PEM-encoded public key bytes.
	PubKeyPem []byte
}

// MarshalBlock marshals the parameters to binary.
func (p *PemParameters) MarshalBlock() ([]byte, error) {
	return p.PubKeyPem, nil
}

// Validate validates the parameters by parsing the PEM public key.
func (p *PemParameters) Validate() error {
	if len(p.PubKeyPem) == 0 {
		return errors.New("pub key pem must be set")
	}
	pub, err := keypem.ParsePubKeyPem(p.PubKeyPem)
	if err != nil {
		return errors.Wrap(err, "parse pub key pem")
	}
	if pub == nil {
		return errors.New("no public key found in pem data")
	}
	return nil
}

// _ is a type assertion
var _ auth_method.Parameters = ((*PemParameters)(nil))

// GenerateBackupKey creates a new Ed25519 keypair for PEM backup.
// Returns the private key PEM and public key PEM bytes.
func GenerateBackupKey() (privPem []byte, pubPem []byte, err error) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate ed25519 key")
	}
	privPem, err = keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal private key pem")
	}
	pubPem, err = keypem.MarshalPubKeyPem(priv.GetPublic())
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal public key pem")
	}
	return privPem, pubPem, nil
}

// _ is a type assertion
var _ auth_method.Method = ((*PemMethod)(nil))
