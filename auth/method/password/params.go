// Package auth_method_password implements password-based entity key derivation
// using scrypt with a blake3-derived deterministic salt from the username.
package auth_method_password

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	auth_method "github.com/s4wave/spacewave/auth/method"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/zeebo/blake3"
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

// Supported scrypt records use 2^14 through 2^20 work factors, r=8, and p=1.
// The upper bound retains repository-produced 2^20 records while limiting a
// single derivation to approximately 1 GiB of scrypt working memory.
const (
	minScryptN = 14
	maxScryptN = DefaultScryptN
)

func newParameters(username string, n, r, parallel uint32) *Parameters {
	var salt [saltLen]byte
	blake3.DeriveKey(saltContext, []byte(username), salt[:])
	return &Parameters{Salt: salt[:], ScryptN: n, ScryptR: r, ScryptP: parallel}
}

func (p *PasswordMethod) deriveKey(ctx context.Context, params *Parameters, password []byte) (crypto.PrivKey, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	n := params.GetScryptN()
	if n == 0 {
		n = DefaultScryptN
	}
	r := int(params.GetScryptR())
	if r == 0 {
		r = DefaultScryptR
	}
	parallel := int(params.GetScryptP())
	if parallel == 0 {
		parallel = DefaultScryptP
	}

	release, err := p.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	var passKey [32]byte
	blake3.DeriveKey("aperture/auth 2026-03-16 password-kdf passphrase v2", password, passKey[:])
	seed, err := p.derive(passKey[:], params.GetSalt(), 1<<n, r, parallel, 32)
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
	n := p.GetScryptN()
	if n == 0 {
		n = DefaultScryptN
	}
	if n < minScryptN || n > maxScryptN {
		return errors.Errorf("scrypt n exponent %d outside supported range %d..%d", n, minScryptN, maxScryptN)
	}
	r := p.GetScryptR()
	if r == 0 {
		r = DefaultScryptR
	}
	if r != DefaultScryptR {
		return errors.Errorf("unsupported scrypt r parameter %d", r)
	}
	parallel := p.GetScryptP()
	if parallel == 0 {
		parallel = DefaultScryptP
	}
	if parallel != DefaultScryptP {
		return errors.Errorf("unsupported scrypt p parameter %d", parallel)
	}
	return nil
}

// MarshalBlock marshals the parameters to binary.
func (p *Parameters) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// _ is a type assertion.
var _ auth_method.Parameters = (*Parameters)(nil)
