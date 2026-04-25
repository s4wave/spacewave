package session_lock

import (
	"crypto/rand"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/scrypt"
)

// pinKDFContext is the blake3 context for PIN key derivation.
var pinKDFContext = "aperture/alpha 2026-03-16 session-lock pin-kdf v2"

// LockConfig is stored at {sessionID}/lock-params in ObjectStore.
type LockConfig struct {
	// ScryptN is the scrypt N parameter (cost factor as power of 2).
	ScryptN uint32
	// Salt is the random salt for PIN key derivation (16 bytes).
	Salt []byte
}

// MarshalVT serializes LockConfig to bytes.
// Format: 4 bytes scryptN (big-endian) + salt bytes.
func (c *LockConfig) MarshalVT() ([]byte, error) {
	out := make([]byte, 4+len(c.Salt))
	out[0] = byte(c.ScryptN >> 24)
	out[1] = byte(c.ScryptN >> 16)
	out[2] = byte(c.ScryptN >> 8)
	out[3] = byte(c.ScryptN)
	copy(out[4:], c.Salt)
	return out, nil
}

// UnmarshalVT deserializes LockConfig from bytes.
func (c *LockConfig) UnmarshalVT(data []byte) error {
	if len(data) < 4 {
		return errors.New("lock config too short")
	}
	c.ScryptN = uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	c.Salt = make([]byte, len(data)-4)
	copy(c.Salt, data[4:])
	return nil
}

// derivePinKey derives a 32-byte key from a PIN using scrypt+blake3.
func derivePinKey(config *LockConfig, pin []byte) ([]byte, error) {
	n := config.ScryptN
	if n == 0 {
		n = 18 // 2^18 = ~0.25s, appropriate for PIN unlock
	}

	// Pre-hash PIN with blake3 for context binding.
	var passKey [32]byte
	blake3.DeriveKey(pinKDFContext, pin, passKey[:])

	pinKey, err := scrypt.Key(passKey[:], config.Salt, 1<<n, 8, 1, 32)
	if err != nil {
		return nil, errors.Wrap(err, "scrypt pin key derivation")
	}
	return pinKey, nil
}

// CreatePINLock creates PIN-encrypted lock files for a session private key.
// Returns encrypted privkey, encrypted symmetric key, and lock config.
func CreatePINLock(privPEM, pin []byte) (encPriv, encSymKey []byte, config *LockConfig, err error) {
	// Generate random 32-byte symmetric key.
	var symKey [32]byte
	_, err = rand.Read(symKey[:])
	if err != nil {
		return nil, nil, nil, err
	}
	defer scrub.Scrub(symKey[:])

	// Encrypt privkey with symKey.
	symMethod, err := blockenc.NewXChaCha20Poly1305(symKey[:])
	if err != nil {
		return nil, nil, nil, err
	}
	encPriv, err = symMethod.Encrypt(blockenc.DefaultAllocFn(), privPEM)
	if err != nil {
		return nil, nil, nil, err
	}

	// Derive pinKey from PIN via scrypt (random salt).
	salt := make([]byte, 16)
	_, err = rand.Read(salt)
	if err != nil {
		return nil, nil, nil, err
	}
	config = &LockConfig{ScryptN: 18, Salt: salt}
	pinKey, err := derivePinKey(config, pin)
	if err != nil {
		return nil, nil, nil, err
	}
	defer scrub.Scrub(pinKey)

	// Encrypt symKey with pinKey.
	pinMethod, err := blockenc.NewXChaCha20Poly1305(pinKey)
	if err != nil {
		return nil, nil, nil, err
	}
	encSymKey, err = pinMethod.Encrypt(blockenc.DefaultAllocFn(), symKey[:])
	if err != nil {
		return nil, nil, nil, err
	}

	return encPriv, encSymKey, config, nil
}

// UnlockPIN decrypts a PIN-locked session key.
func UnlockPIN(encPriv, encSymKey []byte, config *LockConfig, pin []byte) ([]byte, error) {
	pinKey, err := derivePinKey(config, pin)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(pinKey)

	// Decrypt symKey.
	pinMethod, err := blockenc.NewXChaCha20Poly1305(pinKey)
	if err != nil {
		return nil, err
	}
	symKeyBytes, err := pinMethod.Decrypt(blockenc.DefaultAllocFn(), encSymKey)
	if err != nil {
		return nil, errors.New("wrong PIN or corrupted lock key")
	}
	defer scrub.Scrub(symKeyBytes)

	// Decrypt privkey.
	symMethod, err := blockenc.NewXChaCha20Poly1305(symKeyBytes)
	if err != nil {
		return nil, err
	}
	return symMethod.Decrypt(blockenc.DefaultAllocFn(), encPriv)
}
