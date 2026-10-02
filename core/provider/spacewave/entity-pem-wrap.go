package provider_spacewave

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
	"io"
	"strings"

	"filippo.io/age"
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// PasskeyPrfOutputSize is the WebAuthn PRF output length, which keys AES-256-GCM.
const PasskeyPrfOutputSize = 32

// pinScryptWorkFactor is the age scrypt work factor for PIN-wrapped entity keys.
const pinScryptWorkFactor = 18

// WrapPemWithPin encrypts an entity PEM to an age scrypt recipient keyed by pin.
func WrapPemWithPin(pem []byte, pin string) ([]byte, error) {
	// Derive the recipient from the PIN.
	r, err := age.NewScryptRecipient(pin)
	if err != nil {
		return nil, errors.Wrap(err, "create age scrypt recipient")
	}
	r.SetWorkFactor(pinScryptWorkFactor)

	// Encrypt the PEM to the recipient.
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, r)
	if err != nil {
		return nil, errors.Wrap(err, "create age encryptor")
	}
	if _, err := w.Write(pem); err != nil {
		return nil, errors.Wrap(err, "write age payload")
	}
	if err := w.Close(); err != nil {
		return nil, errors.Wrap(err, "close age encryptor")
	}
	return buf.Bytes(), nil
}

// UnwrapPemWithPin decrypts an entity PEM wrapped by WrapPemWithPin.
func UnwrapPemWithPin(wrapped []byte, pin string) ([]byte, error) {
	// Derive the identity from the PIN.
	id, err := age.NewScryptIdentity(pin)
	if err != nil {
		return nil, errors.Wrap(err, "create age scrypt identity")
	}

	// Decrypt and read the PEM.
	r, err := age.Decrypt(bytes.NewReader(wrapped), id)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt wrapped PEM")
	}
	pem, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "read decrypted PEM")
	}
	return pem, nil
}

// WrapWithPasskeyPrf seals plaintext with AES-256-GCM keyed by the WebAuthn
// PRF output. The returned parameters carry the nonce and the PIN flag.
func WrapWithPasskeyPrf(plaintext, prfOutput []byte, pinWrapped bool) ([]byte, *s4wave_provider_spacewave.PasskeyPrfAuthParams, error) {
	// Key the cipher with the PRF output.
	gcm, err := newPasskeyPrfGCM(prfOutput)
	if err != nil {
		return nil, nil, err
	}

	// Seal under a fresh nonce.
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, nil, errors.Wrap(err, "generate passkey PRF nonce")
	}
	params := &s4wave_provider_spacewave.PasskeyPrfAuthParams{
		Algorithm:  s4wave_provider_spacewave.PasskeyPrfWrapAlgorithm_PASSKEY_PRF_WRAP_ALGORITHM_AES_256_GCM_V1,
		Nonce:      nonce,
		PinWrapped: pinWrapped,
	}
	return gcm.Seal(nil, nonce, plaintext, nil), params, nil
}

// UnwrapWithPasskeyPrf opens a blob sealed by WrapWithPasskeyPrf.
func UnwrapWithPasskeyPrf(ciphertext []byte, params *s4wave_provider_spacewave.PasskeyPrfAuthParams, prfOutput []byte) ([]byte, error) {
	// Accept only the algorithm WrapWithPasskeyPrf writes.
	if params.GetAlgorithm() != s4wave_provider_spacewave.PasskeyPrfWrapAlgorithm_PASSKEY_PRF_WRAP_ALGORITHM_AES_256_GCM_V1 {
		return nil, errors.New("unsupported passkey PRF wrap algorithm")
	}
	gcm, err := newPasskeyPrfGCM(prfOutput)
	if err != nil {
		return nil, err
	}
	if len(params.GetNonce()) != gcm.NonceSize() {
		return nil, errors.Errorf("passkey PRF nonce must be %d bytes", gcm.NonceSize())
	}

	// Open the blob.
	plaintext, err := gcm.Open(nil, params.GetNonce(), ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt passkey PRF blob")
	}
	return plaintext, nil
}

// RecoverPasskeyEntityPem recovers the entity PEM from a passkey reauth
// result. readPin is called only when the PEM is also PIN-wrapped.
func RecoverPasskeyEntityPem(
	reauth *s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse,
	readPin func() (string, error),
) ([]byte, error) {
	// Decode the stored blob.
	if reauth.GetEncryptedBlob() == "" {
		return nil, errors.New("passkey reauth returned no key blob")
	}
	blob, err := decodeBase64(reauth.GetEncryptedBlob())
	if err != nil {
		return nil, errors.Wrap(err, "decode passkey key blob")
	}

	// Remove the PRF layer when the passkey supports PRF.
	pinWrapped := reauth.GetPinWrapped()
	if reauth.GetPrfCapable() {
		paramsData, err := decodeBase64(reauth.GetAuthParams())
		if err != nil {
			return nil, errors.Wrap(err, "decode passkey PRF auth params")
		}
		params := &s4wave_provider_spacewave.PasskeyPrfAuthParams{}
		if err := params.UnmarshalVT(paramsData); err != nil {
			return nil, errors.Wrap(err, "unmarshal passkey PRF auth params")
		}
		prfOutput, err := decodeBase64(reauth.GetPrfOutput())
		if err != nil {
			return nil, errors.Wrap(err, "decode passkey PRF output")
		}
		blob, err = UnwrapWithPasskeyPrf(blob, params, prfOutput)
		if err != nil {
			return nil, err
		}
		if !params.GetPinWrapped() {
			return blob, nil
		}

		// A PIN-wrapped PRF payload is the base64 age ciphertext.
		blob, err = decodeBase64(string(blob))
		if err != nil {
			return nil, errors.Wrap(err, "decode PIN-wrapped key")
		}
		pinWrapped = true
	}
	if !pinWrapped {
		return blob, nil
	}

	// Remove the PIN layer.
	pin, err := readPin()
	if err != nil {
		return nil, err
	}
	return UnwrapPemWithPin(blob, pin)
}

// decodeBase64 decodes standard base64 with or without padding, as the
// browser's atob accepts.
func decodeBase64(s string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}

// newPasskeyPrfGCM keys AES-256-GCM with a WebAuthn PRF output.
func newPasskeyPrfGCM(prfOutput []byte) (cipher.AEAD, error) {
	// Require a 256-bit key and build the AEAD.
	if len(prfOutput) != PasskeyPrfOutputSize {
		return nil, errors.Errorf("prf_output must be %d bytes", PasskeyPrfOutputSize)
	}
	block, err := aes.NewCipher(prfOutput)
	if err != nil {
		return nil, errors.Wrap(err, "create passkey PRF cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create passkey PRF gcm")
	}
	return gcm, nil
}
