package auth_triplesec

import (
	"bytes"
	"crypto/sha256"

	"github.com/keybase/go-triplesec"
	"github.com/pkg/errors"
)

// DeriveSalt derives the salt from a seed of any length.
func DeriveSalt(seed []byte) ([]byte, error) {
	sum := sha256.Sum256(seed)
	salt := sum[:triplesec.SaltLen] // 16
	return salt, nil
}

// BuildCipher builds the cipher from the parameters.
func (p *Params) BuildCipher(passphrase []byte) (*triplesec.Cipher, error) {
	salt := p.GetSalt()
	if len(salt) != triplesec.SaltLen {
		return nil, errors.Errorf("salt length must be %d", triplesec.SaltLen)
	}
	ver := triplesec.Version(p.GetVersion())
	switch {
	case ver == 0:
		ver = triplesec.LatestVersion
	case ver > triplesec.LatestVersion:
		return nil, errors.Errorf("unknown triplesec version: %v", ver)
	}
	return triplesec.NewCipher(passphrase, salt, ver)
}

// VerifyCipher checks if the cipher matches the params.
func (p *Params) VerifyCipher(cipher *triplesec.Cipher) error {
	// check salt matches if set
	salt := p.GetSalt()
	if len(salt) != 0 {
		csalt, _ := cipher.GetSalt()
		if len(csalt) != 0 {
			if bytes.Compare(csalt, salt) != 0 {
				return errors.New("salt mismatch")
			}
		}
	}

	// TODO: derive key and check against public key?
	return nil
}
