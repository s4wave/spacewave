// Package crypto implements cryptographic key utilities for bifrost.
//
// It provides Key, PrivKey, and PubKey interfaces with an Ed25519
// implementation. Wire-compatible with go-libp2p's crypto protobuf format.
//
// Loosely based on the go-libp2p crypto implementation, covered under the MIT
// license: https://github.com/libp2p/go-libp2p/tree/master/core/crypto
// Original reference commit: github.com/aperturerobotics/go-libp2p@5cfbb50b74e0
package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"io"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
)

var (
	// ErrBadKeyType is returned when a key is not supported.
	ErrBadKeyType = errors.New("invalid or unsupported key type")
	// ErrNilPrivateKey is returned when a nil private key is provided.
	ErrNilPrivateKey = errors.New("private key is nil")
	// ErrNilPublicKey is returned when a nil public key is provided.
	ErrNilPublicKey = errors.New("public key is nil")
)

// Ed25519 is the Ed25519 key type constant for use with GenerateKeyPair.
const Ed25519 = int(KeyType_Ed25519)

// PubKeyUnmarshaller is a func that creates a PubKey from a given slice of bytes.
type PubKeyUnmarshaller func(data []byte) (PubKey, error)

// PrivKeyUnmarshaller is a func that creates a PrivKey from a given slice of bytes.
type PrivKeyUnmarshaller func(data []byte) (PrivKey, error)

// PubKeyUnmarshallers is a map of unmarshallers by key type.
var PubKeyUnmarshallers = map[KeyType]PubKeyUnmarshaller{
	KeyType_Ed25519: UnmarshalEd25519PublicKey,
}

// PrivKeyUnmarshallers is a map of unmarshallers by key type.
var PrivKeyUnmarshallers = map[KeyType]PrivKeyUnmarshaller{
	KeyType_Ed25519: UnmarshalEd25519PrivateKey,
}

// Key represents a crypto key that can be compared to another key.
type Key interface {
	// Equals checks whether two PubKeys are the same.
	Equals(Key) bool

	// Raw returns the raw bytes of the key (not wrapped in the protobuf).
	//
	// This function is the inverse of {Priv,Pub}KeyUnmarshaler.
	Raw() ([]byte, error)

	// Type returns the protobuf key type.
	Type() KeyType
}

// PrivKey represents a private key that can be used to generate a public key and sign data.
type PrivKey interface {
	Key

	// Sign cryptographically signs the given bytes.
	Sign([]byte) ([]byte, error)

	// GetPublic returns a public key paired with this private key.
	GetPublic() PubKey
}

// PubKey is a public key that can be used to verify data signed with the corresponding private key.
type PubKey interface {
	Key

	// Verify checks that 'sig' is the signed hash of 'data'.
	Verify(data []byte, sig []byte) (bool, error)
}

// GenerateKeyPair generates a private and public key.
func GenerateKeyPair(typ KeyType, bits int) (PrivKey, PubKey, error) {
	return GenerateKeyPairWithReader(typ, bits, rand.Reader)
}

// GenerateKeyPairWithReader returns a keypair of the given type and bit-size.
func GenerateKeyPairWithReader(typ KeyType, bits int, src io.Reader) (PrivKey, PubKey, error) {
	switch typ {
	case KeyType_Ed25519:
		return GenerateEd25519Key(src)
	default:
		return nil, nil, ErrBadKeyType
	}
}

// UnmarshalPublicKey converts a protobuf serialized public key into its representative object.
func UnmarshalPublicKey(data []byte) (PubKey, error) {
	pmes := new(PublicKey)
	if err := pmes.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return PublicKeyFromProto(pmes)
}

// PublicKeyFromProto converts an unserialized protobuf PublicKey message into its representative object.
func PublicKeyFromProto(pmes *PublicKey) (PubKey, error) {
	um, ok := PubKeyUnmarshallers[pmes.GetKeyType()]
	if !ok {
		return nil, ErrBadKeyType
	}
	return um(pmes.GetData())
}

// MarshalPublicKey converts a public key object into a protobuf serialized public key.
func MarshalPublicKey(k PubKey) ([]byte, error) {
	pbmes, err := PublicKeyToProto(k)
	if err != nil {
		return nil, err
	}
	return pbmes.MarshalVT()
}

// PublicKeyToProto converts a public key object into an unserialized protobuf PublicKey message.
func PublicKeyToProto(k PubKey) (*PublicKey, error) {
	data, err := k.Raw()
	if err != nil {
		return nil, err
	}
	return &PublicKey{
		KeyType: k.Type(),
		Data:    data,
	}, nil
}

// UnmarshalPrivateKey converts a protobuf serialized private key into its representative object.
func UnmarshalPrivateKey(data []byte) (PrivKey, error) {
	pmes := new(PrivateKey)
	if err := pmes.UnmarshalVT(data); err != nil {
		return nil, err
	}
	um, ok := PrivKeyUnmarshallers[pmes.GetKeyType()]
	if !ok {
		return nil, ErrBadKeyType
	}
	return um(pmes.GetData())
}

// MarshalPrivateKey converts a key object into its protobuf serialized form.
func MarshalPrivateKey(k PrivKey) ([]byte, error) {
	data, err := k.Raw()
	if err != nil {
		return nil, err
	}
	return (&PrivateKey{
		KeyType: k.Type(),
		Data:    data,
	}).MarshalVT()
}

// ConfigDecodeKey decodes from b64 (for config file) to a byte array that can be unmarshalled.
func ConfigDecodeKey(b string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b)
}

// ConfigEncodeKey encodes a marshalled key to b64 (for config file).
func ConfigEncodeKey(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// basicEquals compares two keys by raw bytes.
func basicEquals(k1, k2 Key) bool {
	if k1.Type() != k2.Type() {
		return false
	}
	a, err := k1.Raw()
	if err != nil {
		return false
	}
	b, err := k2.Raw()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ensure proto types implement the interface
var (
	_ protobuf_go_lite.Message = (*PublicKey)(nil)
	_ protobuf_go_lite.Message = (*PrivateKey)(nil)
)
