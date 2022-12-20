package auth_method_triplesec

import (
	"bytes"
	"strconv"

	crypto_triplesec "github.com/aperturerobotics/auth/crypto/triplesec"
	auth_method "github.com/aperturerobotics/auth/method"
	"github.com/keybase/go-triplesec"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2s"
)

// NewParameters constructs the parameters.
func NewParameters(salt []byte, version uint32) *Parameters {
	return &Parameters{Salt: salt, Version: version}
}

// BuildParametersWithUsernamePassword builds Parameters with username and pass.
func BuildParametersWithUsernamePassword(version uint32, username string, password []byte) (*Parameters, crypto.PrivKey, error) {
	if version == 0 {
		version = uint32(triplesec.LatestVersion)
	}
	saltSrc := bytes.Join([][]byte{
		[]byte(username),
		[]byte(strconv.Itoa(int(version))),
	}, []byte("-"))
	saltData := blake2s.Sum256(saltSrc)
	salt := saltData[:triplesec.SaltLen]
	cipher, err := crypto_triplesec.BuildCipher(version, salt, password)
	if err != nil {
		return nil, nil, err
	}
	privKey, err := crypto_triplesec.DeriveED25519Key(cipher)
	if err != nil {
		return nil, nil, err
	}
	return &Parameters{
		Salt:    salt,
		Version: version,
	}, privKey, nil
}

// Validate validates the parameters (cursory).
func (p *Parameters) Validate() error {
	if saltLen := len(p.GetSalt()); saltLen != triplesec.SaltLen {
		return errors.Errorf("expected salt len %v but got %v", triplesec.SaltLen, saltLen)
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (p *Parameters) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// _ is a type assertion
var _ auth_method.Parameters = ((*Parameters)(nil))
