package peer

import (
	"crypto/aes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/subtle"

	"github.com/aperturerobotics/util/scrub"
	"github.com/klauspost/compress/s2"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/extra25519"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/chacha20poly1305"
)

// EncryptToEd25519 encrypts to a ed25519 key using curve25519.
//
// t is the target ed25519 public key.
//
// mix pub key into seed: blake3(context + msgSrc + tPubKey)
// generate the one-time use message priv key (ed25519) from seed
// convert the target public key to a curve25519 point
// convert the message private key to a curve25519 scalar
// generate the nonce with blake3(context + msgPubKeyEd25519 + msgPubKeyCurve25519)[:24]
// xor the nonce with blake3(msgPubKeyEd25519 + msgPubKeyCurve25519)[24:] (8 bytes long)
// generate msgPubKey aes256 key: blake3(context + tPubKey + msgNonce[:4])
// generate key for chacha20poly1305 with ecdh(msgPrivKeyCurve25519, tPubKeyCurve25519)
//
// ciphertext: msgNonce[:4] + aes256(msgPubKey) + chacha20poly1305(s2(message))
//
// context and destination public key must be the same when decrypting
// context should be globally unique, and application-specific.
// A good format for ctx strings is: [application] [commit timestamp] [purpose]
// e.g., "example.com 2019-12-25 16:18:03 session tokens v1"
// the purpose of these requirements is to ensure that an attacker cannot trick two different applications into using the same context string.
func EncryptToEd25519(
	tPubKey ed25519.PublicKey,
	context string,
	msgSrc []byte,
) ([]byte, error) {
	if len(tPubKey) != 32 {
		return nil, errors.Errorf("unexpected ed25519 public key len: %d", len(tPubKey))
	}

	// mix pub key and msg src into 32-byte seed: blake3(context + msgSrc + tPubKey)
	seedHasher := blake3.NewDeriveKey("bifrost/peer encrypt curve25519 " + context)
	_, err := seedHasher.Write(msgSrc)
	if err != nil {
		return nil, err
	}
	_, err = seedHasher.Write(tPubKey)
	if err != nil {
		return nil, err
	}
	msgSeed := seedHasher.Sum(nil)
	seedHasher.Reset()

	// generate the message priv key (ed25519) from seed
	msgPrivKey := ed25519.NewKeyFromSeed(msgSeed[:])
	scrub.Scrub(msgSeed[:])
	msgPubKey := msgPrivKey.Public().(ed25519.PublicKey)
	defer scrub.Scrub(msgPubKey)

	// See: https://words.filippo.io/using-ed25519-keys-for-encryption/
	msgPrivKeyCurve25519 := extra25519.PrivateKeyToCurve25519(msgPrivKey)
	scrub.Scrub(msgPrivKey)
	msgPrivKeyEcdh, err := ecdh.X25519().NewPrivateKey(msgPrivKeyCurve25519[:32])
	scrub.Scrub(msgPrivKeyCurve25519)
	if err != nil {
		return nil, err
	}

	// generate the nonce
	msgNonceHasher := blake3.NewDeriveKey("bifrost/peer encrypt curve25519 nonce " + context)
	_, err = msgNonceHasher.Write(msgPubKey)
	if err != nil {
		return nil, err
	}
	msgPubKeyHash := msgNonceHasher.Sum(nil)

	// use the first 24 bytes of msg pubkey hash as 24-byte nonce
	msgNonce := msgPubKeyHash[:chacha20poly1305.NonceSizeX]

	// xor the remaining 8 bytes of the hash with the nonce
	xorHash := msgPubKeyHash[chacha20poly1305.NonceSizeX:]
	for i := range msgNonce {
		msgNonce[i] ^= xorHash[(i+2)%len(xorHash)]
	}

	// convert public key to curve25519 montgomery point
	tPubKeyCurve25519, valid := extra25519.PublicKeyToCurve25519(tPubKey)
	if !valid {
		return nil, ErrInvalidEd25519PubKeyForCurve25519
	}
	tPubKeyEcdh, err := ecdh.X25519().NewPublicKey(tPubKeyCurve25519[:])
	scrub.Scrub(tPubKeyCurve25519[:])
	if err != nil {
		return nil, err
	}

	// compress the message
	msg := s2.EncodeBetter(nil, msgSrc)

	// build the seed for the message prefix: blake3(context, tPubKey+msgNonce[:4])
	seedHasher = blake3.NewDeriveKey("bifrost/peer encrypt curve25519 prefix " + context)
	_, err = seedHasher.Write(tPubKey[:])
	if err != nil {
		return nil, err
	}
	_, err = seedHasher.Write(msgNonce[:4])
	if err != nil {
		return nil, err
	}
	aes256Seed := seedHasher.Sum(nil)

	// encrypt the message prefix with aes256
	// allocate enough space for the ciphertext as well
	prefix := make([]byte, 4+32, len(msgPubKey)+chacha20poly1305.Overhead+len(msg)+8)
	copy(prefix[:4], msgNonce[:4])
	copy(prefix[4:], msgPubKey[:])
	prefixCipher, err := aes.NewCipher(aes256Seed[:32])
	if err != nil {
		scrub.Scrub(aes256Seed)
		return nil, err
	}
	prefixCipher.Encrypt(prefix[4:], prefix[4:])
	scrub.Scrub(aes256Seed)

	// sharedSecret is 32 bytes with ecdh(msgPrivKey, tPubKey)
	sharedSecret, err := msgPrivKeyEcdh.ECDH(tPubKeyEcdh)
	if err != nil {
		return nil, err
	}

	// encrypt with chacha20poly1305 cipher
	// prepend the 32-byte curve25519 one-time message key
	// re-use the prefix buf
	cipher, err := chacha20poly1305.NewX(sharedSecret)
	scrub.Scrub(sharedSecret)
	if err != nil {
		return nil, err
	}
	return cipher.Seal(
		prefix,
		msgNonce,
		msg,
		msgPubKey[:],
	), nil
}

// DecryptWithEd25519 decrypts with a ed25519 key using curve25519.
//
// tPrivKey is the target (destination) private key.
//
// derive aes256 key: blake3(context + tPubKey + ciphertext[:4])
// decrypt msgPubKey with aes256 from ciphertext[4:][:32]
// convert the message public key to a curve25519 point
// convert the target private key to a curve25519 scalar
// derive key for chacha20poly1305 with ecdh(privKeyCurve25519, msgPubKeyCurve25519)
// derive nonce with blake3(context, msgPubKey)[:24]
// xor the nonce with blake3(context, msgPubKey)[24:] (8 bytes long)
//
// ciphertext: msgNonce[:4] + aes256(msgPubKey) + chacha20poly1305(s2(message))
//
// context and destination key must be the same as when encrypting
func DecryptWithEd25519(
	tPrivKey ed25519.PrivateKey,
	context string,
	ciphertext []byte,
) ([]byte, error) {
	if len(tPrivKey) != 64 {
		return nil, errors.Errorf("unexpected ed25519 private key len: %d", len(tPrivKey))
	}
	if len(ciphertext) < 34 {
		return nil, ErrShortMessage
	}

	// initialize tPubKey with ed25519 public key
	tPubKey := tPrivKey.Public().(ed25519.PublicKey)
	defer scrub.Scrub(tPubKey[:])

	// build the seed for the aes256 key
	// ciphertext[:4] is msgNonce[:4]
	seedHasher := blake3.NewDeriveKey("bifrost/peer encrypt curve25519 prefix " + context)
	_, err := seedHasher.Write(tPubKey)
	if err != nil {
		return nil, err
	}
	_, err = seedHasher.Write(ciphertext[:4])
	if err != nil {
		return nil, err
	}
	aes256Seed := seedHasher.Sum(nil)

	// msgPubKey is the 32-byte per-message ed25519 pub key
	var msgPubKey [32]byte
	copy(msgPubKey[:], ciphertext[4:])
	defer scrub.Scrub(msgPubKey[:])

	// decrypt the message prefix (msg pub key) with aes256
	prefixCipher, err := aes.NewCipher(aes256Seed[:32])
	if err == nil {
		prefixCipher.Decrypt(msgPubKey[:], msgPubKey[:])
	}
	scrub.Scrub(aes256Seed)
	if err != nil {
		return nil, err
	}

	// build msgPubKeyEcdh
	msgPubKeyCurve25519, valid := extra25519.PublicKeyToCurve25519(msgPubKey[:])
	if !valid {
		return nil, ErrInvalidEd25519PubKeyForCurve25519
	}
	msgPubKeyEcdh, err := ecdh.X25519().NewPublicKey(msgPubKeyCurve25519[:])
	defer scrub.Scrub(msgPubKeyCurve25519[:])
	if err != nil {
		return nil, err
	}

	// build tPrivKeyEcdh
	tPrivKeyCurve25519 := extra25519.PrivateKeyToCurve25519(tPrivKey)
	tPrivKeyEcdh, err := ecdh.X25519().NewPrivateKey(tPrivKeyCurve25519[:32])
	scrub.Scrub(tPrivKeyCurve25519[:])
	if err != nil {
		return nil, err
	}

	// sharedSecret is 32 bytes with (tPrivKeyCurve25519, msgPubKeyCurve25519)
	sharedSecret, err := tPrivKeyEcdh.ECDH(msgPubKeyEcdh)
	if err != nil {
		scrub.Scrub(sharedSecret)
		return nil, err
	}

	// generate the nonce
	msgNonceHasher := blake3.NewDeriveKey("bifrost/peer encrypt curve25519 nonce " + context)
	_, err = msgNonceHasher.Write(msgPubKey[:])
	if err != nil {
		return nil, err
	}
	msgPubKeyHash := msgNonceHasher.Sum(nil)

	// use the first 24 bytes of msg pubkey hash as 24-byte nonce
	msgNonce := msgPubKeyHash[:chacha20poly1305.NonceSizeX]

	// xor the remaining 8 bytes of the hash with the nonce
	xorHash := msgPubKeyHash[chacha20poly1305.NonceSizeX:]
	for i := range msgNonce {
		msgNonce[i] ^= xorHash[(i+2)%len(xorHash)]
	}

	// decrypt message with shared secret
	cipher, err := chacha20poly1305.NewX(sharedSecret)
	scrub.Scrub(sharedSecret)
	if err != nil {
		return nil, err
	}
	msgEnc := ciphertext[32+4:]
	msgDec, err := cipher.Open(nil, msgNonce, msgEnc, msgPubKey[:])
	if err != nil {
		return nil, err
	}

	// decompress message, re-use scrubbed shared secret buffer
	msgSrc, err := s2.Decode(sharedSecret, msgDec)
	if err != nil {
		return nil, err
	}

	// verify message: re-generate ed25519 private key
	// mix pub key into 32-byte seed: blake3(context + msgSrc + tPubKey)
	seedHasher = blake3.NewDeriveKey("bifrost/peer encrypt curve25519 " + context)
	_, err = seedHasher.Write(msgSrc)
	if err != nil {
		return nil, err
	}
	_, err = seedHasher.Write(tPubKey[:])
	if err != nil {
		return nil, err
	}
	msgSeed := seedHasher.Sum(nil)

	// generate the message priv key (ed25519) from seed
	msgEdPrivKey := ed25519.NewKeyFromSeed(msgSeed[:])
	scrub.Scrub(msgSeed)
	msgEdPubKey := msgEdPrivKey.Public().(ed25519.PublicKey)
	scrub.Scrub(msgEdPrivKey)
	defer scrub.Scrub(msgEdPubKey)

	// convert message pub key to curve25519 msg pub key
	expectedMsgPubKeyCurve25519, valid := extra25519.PublicKeyToCurve25519(msgEdPubKey)
	scrub.Scrub(msgEdPubKey)
	if !valid {
		return nil, ErrInvalidEd25519PubKeyForCurve25519
	}
	defer scrub.Scrub(expectedMsgPubKeyCurve25519[:])

	// check that they match
	if subtle.ConstantTimeCompare(expectedMsgPubKeyCurve25519, msgPubKeyCurve25519) == 0 {
		return nil, errors.Errorf(
			"message pubkey %s does not match expected pubkey %s",
			b58.Encode(expectedMsgPubKeyCurve25519[:]),
			b58.Encode(msgPubKeyCurve25519[:]),
		)
	}

	// done
	return msgSrc, nil
}
