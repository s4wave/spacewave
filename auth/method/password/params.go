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

// MaxScryptN is the maximum accepted scrypt N parameter (log2 cost).
//
// Bounds the memory and time of deriving keys from untrusted parameters.
const MaxScryptN = DefaultScryptN

// MaxScryptR is the maximum accepted scrypt r parameter.
const MaxScryptR = DefaultScryptR

// MaxScryptP is the maximum accepted scrypt p parameter.
const MaxScryptP = 4

// saltLen is the required salt length.
const saltLen = 16

// BuildParametersWithUsernamePassword builds Parameters and derives an
// Ed25519 private key from a username and password.
//
// The salt is derived deterministically: blake3.DeriveKey(context, username).
// No server-stored salt is needed.
func BuildParametersWithUsernamePassword(username string, password []byte) (*Parameters, crypto.PrivKey, error) {
	return buildParametersWithUsernamePassword(
		username,
		password,
		DefaultScryptN,
		DefaultScryptR,
		DefaultScryptP,
	)
}

// buildParametersWithUsernamePassword builds Parameters with explicit scrypt
// settings and derives an Ed25519 private key from a username and password.
func buildParametersWithUsernamePassword(username string, password []byte, n, r, p uint32) (*Parameters, crypto.PrivKey, error) {
	// Derive the deterministic salt from the username.
	var salt [saltLen]byte
	blake3.DeriveKey(saltContext, []byte(username), salt[:])

	// Assemble the persisted password KDF parameters.
	params := &Parameters{
		Salt:    salt[:],
		ScryptN: n,
		ScryptR: r,
		ScryptP: p,
	}

	// Derive the private key from the assembled parameters.
	privKey, err := deriveKey(params, password)
	if err != nil {
		return nil, nil, err
	}
	return params, privKey, nil
}

// deriveKey derives an Ed25519 private key from parameters and password.
func deriveKey(params *Parameters, password []byte) (crypto.PrivKey, error) {
	// Reject parameters outside the accepted KDF bounds.
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Resolve default KDF parameters when the record omits them.
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

	// Derive the password key before passing it to scrypt.
	var passKey [32]byte
	blake3.DeriveKey("aperture/auth 2026-03-16 password-kdf passphrase v2", password, passKey[:])

	// Run scrypt with the persisted salt and resolved parameters.
	seed, err := scrypt.Key(passKey[:], params.GetSalt(), 1<<n, r, p, 32)
	if err != nil {
		return nil, errors.Wrap(err, "scrypt key derivation")
	}

	// Turn the derived seed into the Ed25519 private key.
	privKey, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed))
	if err != nil {
		return nil, errors.Wrap(err, "generate ed25519 key from seed")
	}
	return privKey, nil
}

// Validate validates the parameters.
func (p *Parameters) Validate() error {
	// Validate the salt length and each KDF cost parameter against its bound.
	if len(p.GetSalt()) != saltLen {
		return errors.Errorf("expected salt len %d but got %d", saltLen, len(p.GetSalt()))
	}
	if n := p.GetScryptN(); n > MaxScryptN {
		return errors.Errorf("scrypt n %d exceeds maximum %d", n, MaxScryptN)
	}
	if r := p.GetScryptR(); r > MaxScryptR {
		return errors.Errorf("scrypt r %d exceeds maximum %d", r, MaxScryptR)
	}
	if sp := p.GetScryptP(); sp > MaxScryptP {
		return errors.Errorf("scrypt p %d exceeds maximum %d", sp, MaxScryptP)
	}
	return nil
}

// MarshalBlock marshals the parameters to binary.
func (p *Parameters) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// _ is a type assertion.
var _ auth_method.Parameters = (*Parameters)(nil)
