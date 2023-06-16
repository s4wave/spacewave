package auth_triplesec

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/keybase/go-triplesec"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
)

// encContext is the encryption context for blake3.
var encContext = "aperture/auth 2023-06-15 06:44:53PM PDT auth/crypto/triplesec cipher v1"

// DeriveSalt derives the salt from a seed of any length.
func DeriveSalt(seed []byte) ([]byte, error) {
	sum := sha256.Sum256(seed)
	salt := sum[:triplesec.SaltLen] // 16
	return salt, nil
}

// BuildCipher builds the cipher from the parameters.
func BuildCipher(version uint32, salt, passphrase []byte) (*triplesec.Cipher, error) {
	if len(salt) != triplesec.SaltLen {
		return nil, errors.Errorf("salt length must be %d", triplesec.SaltLen)
	}
	ver := triplesec.Version(version)
	switch {
	case ver == 0:
		ver = triplesec.LatestVersion
	case ver > triplesec.LatestVersion:
		return nil, errors.Errorf("unknown triplesec version: %v", ver)
	}
	var passKey [32]byte
	blake3.DeriveKey(encContext, passphrase, passKey[:])
	return triplesec.NewCipher(passKey[:], salt, ver)
}

// VerifyCipher checks if the cipher matches the params.
func VerifyCipher(cipher *triplesec.Cipher, salt []byte) error {
	// check salt matches if set
	if len(salt) != 0 {
		csalt, _ := cipher.GetSalt()
		if len(csalt) != 0 {
			if !bytes.Equal(csalt, salt) {
				return errors.New("salt mismatch")
			}
		}
	}

	return nil
}

// DeriveED25519Key derives the ed25519 crypto key.
func DeriveED25519Key(cipher *triplesec.Cipher) (crypto.PrivKey, error) {
	// derive key data. we use the triplesec settings for the key length.
	// the result is used as as a seed to generate the crypto key.
	_, keyData, err := cipher.DeriveKey(ed25519.SeedSize)
	if err != nil {
		return nil, err
	}
	// ed25519 uses a 32 byte seed
	privKey, pubKey, err := crypto.GenerateEd25519Key(bytes.NewReader(keyData))
	if err != nil {
		return nil, err
	}
	_ = pubKey
	return privKey, nil
}
