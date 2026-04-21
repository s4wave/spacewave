// Package auth_method_password implements password-based entity key derivation
// using scrypt with a blake3-derived deterministic salt from the username.
package auth_method_password

import (
	"bytes"

	"github.com/pkg/errors"
	auth_method "github.com/s4wave/spacewave/auth/method"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/scrypt"
)

// saltContext is the blake3 context for deterministic salt derivation.
var saltContext = "aperture/auth 2026-03-16 password-kdf salt v2"

// DefaultScryptN is the default scrypt N parameter (2^20).
const DefaultScryptN = 20

// DefaultScryptR is the default scrypt r parameter.
const DefaultScryptR = 8

// DefaultScryptP is the default scrypt p parameter.
const DefaultScryptP = 1

// saltLen is the required salt length.
const saltLen = 16

// BuildParametersWithUsernamePassword builds Parameters and derives an
// Ed25519 private key from a username and password.
//
// The salt is derived deterministically: blake3.DeriveKey(context, username).
// No server-stored salt is needed.
func BuildParametersWithUsernamePassword(username string, password []byte) (*Parameters, crypto.PrivKey, error) {
	var salt [saltLen]byte
	blake3.DeriveKey(saltContext, []byte(username), salt[:])

	params := &Parameters{
		Salt:    salt[:],
		ScryptN: DefaultScryptN,
		ScryptR: DefaultScryptR,
		ScryptP: DefaultScryptP,
	}

	privKey, err := deriveKey(params, password)
	if err != nil {
		return nil, nil, err
	}
	return params, privKey, nil
}

// deriveKey derives an Ed25519 private key from parameters and password.
func deriveKey(params *Parameters, password []byte) (crypto.PrivKey, error) {
	n := params.GetScryptN()
	if n == 0 {
		n = DefaultScryptN
	}
	r := int(params.GetScryptR())
	if r == 0 {
		r = DefaultScryptR
	}
	p := int(params.GetScryptP())
	if p == 0 {
		p = DefaultScryptP
	}

	// Derive password key via blake3 before passing to scrypt.
	var passKey [32]byte
	blake3.DeriveKey("aperture/auth 2026-03-16 password-kdf passphrase v2", password, passKey[:])

	seed, err := scrypt.Key(passKey[:], params.GetSalt(), 1<<n, r, p, 32)
	if err != nil {
		return nil, errors.Wrap(err, "scrypt key derivation")
	}

	privKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
	if err != nil {
		return nil, errors.Wrap(err, "generate ed25519 key from seed")
	}
	return privKey, nil
}

// Validate validates the parameters.
func (p *Parameters) Validate() error {
	if len(p.GetSalt()) != saltLen {
		return errors.Errorf("expected salt len %d but got %d", saltLen, len(p.GetSalt()))
	}
	return nil
}

// MarshalBlock marshals the parameters to binary.
func (p *Parameters) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// _ is a type assertion.
var _ auth_method.Parameters = ((*Parameters)(nil))
