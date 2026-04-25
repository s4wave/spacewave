//go:build e2e

package onboarding_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"math/big"
)

// virtualAuthenticator simulates a WebAuthn authenticator for testing.
// Generates P-256 credentials and constructs valid CBOR attestation/
// assertion objects that pass @simplewebauthn/server verification.
type virtualAuthenticator struct {
	privKey   *ecdsa.PrivateKey
	credID    []byte
	signCount uint32
	rpID      string
	origin    string
	rpIDHash  [32]byte
}

// newVirtualAuthenticatorWithOrigin creates a new P-256 virtual authenticator.
func newVirtualAuthenticatorWithOrigin(
	origin string,
	rpID string,
) (*virtualAuthenticator, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	credID := make([]byte, 32)
	if _, err := rand.Read(credID); err != nil {
		return nil, err
	}

	va := &virtualAuthenticator{
		privKey: key,
		credID:  credID,
		rpID:    rpID,
		origin:  origin,
	}
	va.rpIDHash = sha256.Sum256([]byte(va.rpID))

	return va, nil
}

// newVirtualAuthenticator creates a new P-256 virtual authenticator.
func newVirtualAuthenticator() (*virtualAuthenticator, error) {
	return newVirtualAuthenticatorWithOrigin(
		"https://app.spacewave.app",
		"spacewave.app",
	)
}

// createRegistrationResponse builds a WebAuthn registration response JSON
// for the given challenge (base64url-encoded from the server options).
func (va *virtualAuthenticator) createRegistrationResponse(challenge string) string {
	clientData := `{"type":"webauthn.create","challenge":"` + challenge +
		`","origin":"` + va.origin + `","crossOrigin":false}`
	clientDataB64 := base64URLEncode([]byte(clientData))

	authData := va.buildAuthDataWithKey()
	attObj := va.buildAttestationObject(authData)
	attObjB64 := base64URLEncode(attObj)

	credIDB64 := base64URLEncode(va.credID)

	return `{"id":"` + credIDB64 +
		`","rawId":"` + credIDB64 +
		`","type":"public-key"` +
		`,"response":{"clientDataJSON":"` + clientDataB64 +
		`","attestationObject":"` + attObjB64 +
		`"},"clientExtensionResults":{},"authenticatorAttachment":"platform"}`
}

// createAuthenticationResponse builds a WebAuthn authentication response JSON.
func (va *virtualAuthenticator) createAuthenticationResponse(challenge string) string {
	va.signCount++

	clientData := `{"type":"webauthn.get","challenge":"` + challenge +
		`","origin":"` + va.origin + `","crossOrigin":false}`
	clientDataJSON := []byte(clientData)
	clientDataB64 := base64URLEncode(clientDataJSON)

	authData := va.buildAuthDataSimple()
	authDataB64 := base64URLEncode(authData)

	// Signature is ECDSA-SHA256 over authData || SHA-256(clientDataJSON).
	clientDataHash := sha256.Sum256(clientDataJSON)
	sigInput := make([]byte, len(authData)+32)
	copy(sigInput, authData)
	copy(sigInput[len(authData):], clientDataHash[:])

	sigHash := sha256.Sum256(sigInput)
	r, s, _ := ecdsa.Sign(rand.Reader, va.privKey, sigHash[:])
	sig := marshalECDSASignatureDER(r, s)
	sigB64 := base64URLEncode(sig)

	credIDB64 := base64URLEncode(va.credID)

	return `{"id":"` + credIDB64 +
		`","rawId":"` + credIDB64 +
		`","type":"public-key"` +
		`,"response":{"clientDataJSON":"` + clientDataB64 +
		`","authenticatorData":"` + authDataB64 +
		`","signature":"` + sigB64 +
		`","userHandle":""}` +
		`,"clientExtensionResults":{},"authenticatorAttachment":"platform"}`
}

// buildAuthDataWithKey constructs authData with the attested credential
// data flag set (for registration).
func (va *virtualAuthenticator) buildAuthDataWithKey() []byte {
	coseKey := va.marshalCOSEKey()

	// 32 rpIdHash + 1 flags + 4 signCount + 16 aaguid + 2 credIdLen + credId + coseKey
	credIDLen := len(va.credID)
	buf := make([]byte, 32+1+4+16+2+credIDLen+len(coseKey))
	off := 0

	copy(buf[off:], va.rpIDHash[:])
	off += 32

	// Flags: UP=1, UV=1, AT=1 = 0x01 | 0x04 | 0x40 = 0x45
	buf[off] = 0x45
	off++

	binary.BigEndian.PutUint32(buf[off:], va.signCount)
	off += 4

	// AAGUID: 16 zero bytes
	off += 16

	binary.BigEndian.PutUint16(buf[off:], uint16(credIDLen))
	off += 2

	copy(buf[off:], va.credID)
	off += credIDLen

	copy(buf[off:], coseKey)

	return buf
}

// buildAuthDataSimple constructs minimal authData (for authentication).
func (va *virtualAuthenticator) buildAuthDataSimple() []byte {
	buf := make([]byte, 37) // 32 rpIdHash + 1 flags + 4 signCount
	copy(buf, va.rpIDHash[:])
	buf[32] = 0x05 // UP=1, UV=1
	binary.BigEndian.PutUint32(buf[33:], va.signCount)
	return buf
}

// marshalCOSEKey encodes the P-256 public key as a COSE Key in CBOR.
// Map: {1:2, 3:-7, -1:1, -2:<x>, -3:<y>}
func (va *virtualAuthenticator) marshalCOSEKey() []byte {
	x := va.privKey.PublicKey.X.Bytes()
	y := va.privKey.PublicKey.Y.Bytes()

	// Pad to 32 bytes.
	xPad := padLeft(x, 32)
	yPad := padLeft(y, 32)

	var buf []byte
	buf = append(buf, 0xA5) // map(5)

	// 1: 2 (kty: EC2)
	buf = append(buf, 0x01, 0x02)
	// 3: -7 (alg: ES256) -- negative: -7 = major1(6) = 0x26
	buf = append(buf, 0x03, 0x26)
	// -1: 1 (crv: P-256) -- key -1 = major1(0) = 0x20
	buf = append(buf, 0x20, 0x01)
	// -2: x (bstr)
	buf = append(buf, 0x21)           // key -2 = major1(1) = 0x21
	buf = append(buf, 0x58, byte(32)) // bstr(32)
	buf = append(buf, xPad...)
	// -3: y (bstr)
	buf = append(buf, 0x22)           // key -3 = major1(2) = 0x22
	buf = append(buf, 0x58, byte(32)) // bstr(32)
	buf = append(buf, yPad...)

	return buf
}

// buildAttestationObject constructs a CBOR attestation object with fmt=none.
// Map: {"fmt":"none", "attStmt":{}, "authData":<bytes>}
func (va *virtualAuthenticator) buildAttestationObject(authData []byte) []byte {
	var buf []byte
	buf = append(buf, 0xA3) // map(3)

	// "fmt": "none"
	buf = appendCBORTextString(buf, "fmt")
	buf = appendCBORTextString(buf, "none")

	// "attStmt": {}
	buf = appendCBORTextString(buf, "attStmt")
	buf = append(buf, 0xA0) // map(0) = empty map

	// "authData": <bstr>
	buf = appendCBORTextString(buf, "authData")
	buf = appendCBORByteString(buf, authData)

	return buf
}

// appendCBORTextString appends a CBOR text string (major type 3).
func appendCBORTextString(buf []byte, s string) []byte {
	b := []byte(s)
	l := len(b)
	if l <= 23 {
		buf = append(buf, byte(0x60|l))
	} else if l <= 255 {
		buf = append(buf, 0x78, byte(l))
	} else {
		buf = append(buf, 0x79)
		buf = append(buf, byte(l>>8), byte(l))
	}
	return append(buf, b...)
}

// appendCBORByteString appends a CBOR byte string (major type 2).
func appendCBORByteString(buf []byte, b []byte) []byte {
	l := len(b)
	if l <= 23 {
		buf = append(buf, byte(0x40|l))
	} else if l <= 255 {
		buf = append(buf, 0x58, byte(l))
	} else {
		buf = append(buf, 0x59)
		buf = append(buf, byte(l>>8), byte(l))
	}
	return append(buf, b...)
}

// padLeft pads b with leading zeros to reach length n.
func padLeft(b []byte, n int) []byte {
	if len(b) >= n {
		return b[len(b)-n:]
	}
	out := make([]byte, n)
	copy(out[n-len(b):], b)
	return out
}

// marshalECDSASignatureDER encodes an ECDSA signature in DER format.
func marshalECDSASignatureDER(r, s *big.Int) []byte {
	rBytes := intToDER(r)
	sBytes := intToDER(s)

	totalLen := len(rBytes) + len(sBytes)
	var buf []byte
	buf = append(buf, 0x30) // SEQUENCE
	if totalLen <= 127 {
		buf = append(buf, byte(totalLen))
	} else {
		buf = append(buf, 0x81, byte(totalLen))
	}
	buf = append(buf, rBytes...)
	buf = append(buf, sBytes...)
	return buf
}

// intToDER encodes a big.Int as a DER INTEGER.
func intToDER(n *big.Int) []byte {
	b := n.Bytes()
	// Prepend 0x00 if high bit is set (positive number with sign bit).
	if len(b) > 0 && b[0]&0x80 != 0 {
		b = append([]byte{0x00}, b...)
	}
	var out []byte
	out = append(out, 0x02, byte(len(b))) // INTEGER tag + length
	return append(out, b...)
}

// base64URLEncode encodes bytes as base64url without padding.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
