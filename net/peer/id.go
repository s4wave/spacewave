package peer

import (
	"encoding/binary"
	"strings"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// mhIdentity is the multihash IDENTITY function code.
const mhIdentity uint64 = 0x00

// ID is a peer identifier derived from a public key's multihash.
//
// The raw bytes are a multihash: varint(hash_code) + varint(digest_len) + digest.
// The IDENTITY multihash is always used, embedding the full marshaled public key.
//
// The string representation is base58-encoded.
type ID string

// String returns the base58-encoded peer ID.
func (id ID) String() string {
	return b58.Encode([]byte(id))
}

// ShortString returns a short representation of the peer ID.
func (id ID) ShortString() string {
	pid := id.String()
	if len(pid) <= 10 {
		return pid
	}
	return pid[:2] + "*" + pid[len(pid)-6:]
}

// Validate checks if the ID is empty.
func (id ID) Validate() error {
	if id == "" {
		return ErrEmptyPeerID
	}
	return nil
}

// MatchesPublicKey tests whether this ID was derived from the public key pk.
func (id ID) MatchesPublicKey(pk crypto.PubKey) bool {
	oid, err := IDFromPublicKey(pk)
	if err != nil {
		return false
	}
	return oid == id
}

// MatchesPrivateKey tests whether this ID was derived from the secret key sk.
func (id ID) MatchesPrivateKey(sk crypto.PrivKey) bool {
	return id.MatchesPublicKey(sk.GetPublic())
}

// ExtractPublicKey extracts the public key from an ID.
//
// All peer IDs embed the full public key using the IDENTITY multihash.
func (id ID) ExtractPublicKey() (crypto.PubKey, error) {
	code, digest, err := decodeMultihash([]byte(id))
	if err != nil {
		return nil, err
	}
	if code != mhIdentity {
		return nil, ErrNoPublicKey
	}
	return crypto.UnmarshalPublicKey(digest)
}

// IDFromBytes casts a byte slice to the ID type and validates that
// the value is a well-formed multihash.
func IDFromBytes(b []byte) (ID, error) {
	if _, _, err := decodeMultihash(b); err != nil {
		return "", err
	}
	return ID(b), nil
}

// IDB58Decode returns a base58-decoded peer ID.
func IDB58Decode(s string) (ID, error) {
	m, err := b58.Decode(s)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse peer ID")
	}
	return IDFromBytes(m)
}

// IDB58Encode returns the base58-encoded string of a peer ID.
func IDB58Encode(id ID) string {
	return b58.Encode([]byte(id))
}

// IDFromPublicKey returns the peer ID corresponding to pk.
//
// The IDENTITY multihash is always used, embedding the full marshaled public key.
func IDFromPublicKey(pk crypto.PubKey) (ID, error) {
	b, err := crypto.MarshalPublicKey(pk)
	if err != nil {
		return "", err
	}
	return ID(encodeMultihash(mhIdentity, b)), nil
}

// IDFromPrivateKey returns the peer ID corresponding to sk.
func IDFromPrivateKey(sk crypto.PrivKey) (ID, error) {
	return IDFromPublicKey(sk.GetPublic())
}

// IDsToString converts a slice of IDs to strings.
func IDsToString(ids []ID) []string {
	out := make([]string, len(ids))
	for i := range ids {
		out[i] = ids[i].String()
	}
	return out
}

// IDSlice is a sortable slice of peer IDs.
type IDSlice []ID

func (es IDSlice) Len() int           { return len(es) }
func (es IDSlice) Swap(i, j int)      { es[i], es[j] = es[j], es[i] }
func (es IDSlice) Less(i, j int) bool { return string(es[i]) < string(es[j]) }

func (es IDSlice) String() string {
	strs := make([]string, len(es))
	for i, id := range es {
		strs[i] = id.String()
	}
	return strings.Join(strs, ", ")
}

// encodeMultihash encodes a multihash: varint(code) + varint(len(digest)) + digest.
func encodeMultihash(code uint64, digest []byte) []byte {
	buf := make([]byte, binary.MaxVarintLen64*2+len(digest))
	n := binary.PutUvarint(buf, code)
	n += binary.PutUvarint(buf[n:], uint64(len(digest)))
	n += copy(buf[n:], digest)
	return buf[:n]
}

// decodeMultihash decodes a multihash into its function code and digest.
func decodeMultihash(b []byte) (code uint64, digest []byte, err error) {
	if len(b) == 0 {
		return 0, nil, errors.New("multihash too short")
	}
	code, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, nil, errors.New("invalid multihash varint")
	}
	b = b[n:]
	dlen, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, nil, errors.New("invalid multihash digest length varint")
	}
	b = b[n:]
	if uint64(len(b)) != dlen {
		return 0, nil, errors.Errorf("multihash digest length mismatch: expected %d, got %d", dlen, len(b))
	}
	return code, b, nil
}
