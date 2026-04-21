package peer

import (
	"crypto/ecdh"
	"crypto/ed25519"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/util/extra25519"
	"github.com/zeebo/blake3"
)

// DeriveKey derives a secret using a private key.
//
// Not all private key types are supported.
// Data is written to out.
//
// context should be globally unique, and application-specific.
// salt is any additional data to mix with the private key.
//
// A good format for ctx strings is: [application] [commit timestamp] [purpose]
// e.g., "example.com 2019-12-25 16:18:03 session tokens v1"
//
// the purpose of these requirements is to ensure that an attacker cannot trick
// two different applications into using the same context string.
func DeriveKey(context string, salt []byte, privKey crypto.PrivKey, out []byte) error {
	spKey, err := crypto.PrivKeyToStdKey(privKey)
	if err != nil {
		return err
	}

	var material []byte
	switch t := spKey.(type) {
	case *ed25519.PrivateKey:
		tPrivKeyCurve25519 := extra25519.PrivateKeyToCurve25519(*t)
		tPrivKeyEcdh, err := ecdh.X25519().NewPrivateKey(tPrivKeyCurve25519[:32])
		if err != nil {
			scrub.Scrub(tPrivKeyCurve25519[:])
			return err
		}

		// hash the context + the curve25519 key
		secret := append(tPrivKeyCurve25519[:], []byte(context)...)
		seed := blake3.Sum256(secret)
		scrub.Scrub(secret)
		scrub.Scrub(tPrivKeyCurve25519[:])

		// generate a new ephemeral private / public key
		ephPrivKey := ed25519.NewKeyFromSeed(seed[:])
		defer scrub.Scrub(ephPrivKey)
		ephPubKey := ephPrivKey.Public().(ed25519.PublicKey)
		defer scrub.Scrub(ephPubKey)
		ephPubKeyCurve25519, valid := extra25519.PublicKeyToCurve25519(ephPubKey)
		if !valid {
			return ErrInvalidEd25519PubKeyForCurve25519
		}
		echPubKey, err := ecdh.X25519().NewPublicKey(ephPubKeyCurve25519[:])
		defer scrub.Scrub(ephPubKeyCurve25519[:])
		if err != nil {
			return err
		}

		// derive a shared secret w/ ephemeral & private key
		material, err = tPrivKeyEcdh.ECDH(echPubKey)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unhandled private key type: %s", privKey.Type().String())
	}

	// xor the material with context
	contextb := []byte(context)
	for i := range material {
		material[i] = material[i] ^ contextb[i%len(contextb)]
	}

	// derive key with blake3
	dkh := blake3.NewDeriveKey(context)
	_, err = dkh.Write([]byte("bifrost/peer/derive-key"))
	if err != nil {
		return err
	}
	if len(salt) != 0 {
		_, err = dkh.Write(salt) // never returns an error
		if err != nil {
			return err
		}
	}
	_, err = dkh.Write(material) // never returns an error
	if err != nil {
		return err
	}
	_, err = dkh.Digest().Read(out)
	if err != nil {
		return err
	}
	return nil
}

// DeriveEd25519Key derives a ed25519 private key from an existing private key.
//
// context should be globally unique, and application-specific.
// salt is any additional data to mix with the private key.
//
// A good format for ctx strings is: [application] [commit timestamp] [purpose]
// e.g., "example.com 2019-12-25 16:18:03 session tokens v1"
//
// the purpose of these requirements is to ensure that an attacker cannot trick
// two different applications into using the same context string.
func DeriveEd25519Key(context string, salt []byte, privKey crypto.PrivKey) (crypto.PrivKey, crypto.PubKey, error) {
	seed := make([]byte, ed25519.SeedSize)
	if err := DeriveKey(context, salt, privKey, seed); err != nil {
		return nil, nil, err
	}

	key := ed25519.NewKeyFromSeed(seed)
	return crypto.KeyPairFromStdKey(&key)
}
