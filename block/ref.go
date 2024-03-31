package block

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/aperturerobotics/bifrost/hash"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

// DefaultHashType is the default hash type for refs.
const DefaultHashType = hash.HashType_HashType_BLAKE3

// NewBlockRef constructs a new block reference.
func NewBlockRef(hash *hash.Hash) *BlockRef {
	return &BlockRef{Hash: hash}
}

// NewBlockRefBlock constructs a new block reference block.
func NewBlockRefBlock() Block {
	return &BlockRef{}
}

// UnmarshalBlockRefBlock unmarshals a BlockRef from a block cursor.
func UnmarshalBlockRefBlock(ctx context.Context, bcs *Cursor) (*BlockRef, error) {
	return UnmarshalBlock[*BlockRef](ctx, bcs, NewBlockRefBlock)
}

// BuildBlockRef builds a block ref from put opts by hashing the data.
// If putOpts are nil, uses default hash type.
func BuildBlockRef(data []byte, putOpts *PutOpts) (*BlockRef, error) {
	hashType := putOpts.SelectHashType(0)
	h, err := hashType.Sum(data)
	if err != nil {
		return nil, err
	}
	return &BlockRef{Hash: hash.NewHash(hashType, h)}, nil
}

// UnmarshalBlockRefB58 unmarshals a b58 string block ref.
func UnmarshalBlockRefB58(ref string) (*BlockRef, error) {
	if ref == "" {
		return nil, nil
	}

	dat, err := b58.Decode(ref)
	if err != nil {
		return nil, err
	}
	r := &BlockRef{}
	if err := r.UnmarshalVT(dat); err != nil {
		return nil, err
	}
	// if a block ref string has non-zero length, it must not be empty.
	if err := r.Validate(false); err != nil {
		return nil, err
	}
	return r, nil
}

// UnmarshalBlockRefJSON attempts to unmarshal an BlockRef from JSON.
func UnmarshalBlockRefJSON(data []byte) (*BlockRef, error) {
	ref := &BlockRef{}
	if err := ref.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return ref, nil
}

// MarshalBlockRefJSON marshals an BlockRef to JSON.
//
// Returns "null" if the ref is nil.
func MarshalBlockRefJSON(ref *BlockRef) ([]byte, error) {
	return ref.MarshalJSON()
}

// Clone clones the block ref.
func (b *BlockRef) Clone() *BlockRef {
	if b == nil {
		return nil
	}
	return &BlockRef{
		Hash: b.GetHash().Clone(),
	}
}

// Validate validates the block ref.
func (b *BlockRef) Validate(allowEmpty bool) error {
	if !allowEmpty && b.GetEmpty() {
		return ErrEmptyBlockRef
	}
	if err := b.GetHash().Validate(); err != nil {
		return err
	}
	return nil
}

// VerifyData checks the given data matches the block ref.
// If errDetails is set, wraps the error with the unexpected and expected refs if mismatch.
func (b *BlockRef) VerifyData(data []byte, errDetails bool) error {
	if b == nil || b.Hash == nil {
		return ErrEmptyBlockRef
	}

	actualHash, err := b.GetHash().VerifyData(data)
	if err != nil {
		actualRef := b.Clone()
		if actualRef.Hash == nil {
			actualRef.Hash = &hash.Hash{}
		}
		actualRef.Hash.Hash = actualHash
		if errDetails {
			return errors.Wrapf(err, "expected block %s but got %s", b.MarshalLog(), actualRef.MarshalLog())
		}
		return err
	}

	return nil
}

// GetEmpty returns if the ref is empty.
func (b *BlockRef) GetEmpty() bool {
	return len(b.GetHash().GetHash()) == 0 || b.GetHash().GetHashType() == 0
}

// EqualsRef checks if two refs are equal.
func (b *BlockRef) EqualsRef(oref *BlockRef) bool {
	return oref.EqualVT(b)
}

// MarshalKey marshals the block ref for use as a key.
// The format should be reproducible and identical between versions.
func (b *BlockRef) MarshalKey() ([]byte, error) {
	return b.MarshalVT()
}

// MarshalString marshals the reference to a string form.
func (b *BlockRef) MarshalString() string {
	if b == nil {
		return ""
	}
	dat, err := b.MarshalKey()
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// MarshalLog marshals the reference for logging.
// If nil or empty returns <nil>
func (b *BlockRef) MarshalLog() string {
	ref := b.MarshalString()
	if ref == "" {
		ref = "<nil>"
	}
	return ref
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *BlockRef) MarshalBlock() ([]byte, error) {
	return b.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *BlockRef) UnmarshalBlock(data []byte) error {
	return b.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (b *BlockRef) ApplyBlockRef(id uint32, ptr *BlockRef) error {
	switch id {
	case 1:
		if h := ptr.GetHash(); h != nil {
			b.Hash = h.Clone()
		} else {
			b.Hash = nil
		}
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (b *BlockRef) GetBlockRefs() (map[uint32]*BlockRef, error) {
	return nil, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (b *BlockRef) GetBlockRefCtor(id uint32) Ctor {
	return nil
}

// LessThan checks if the ref is less than another.
// 1. Empty is sorted to the end.
// 2. Hash type is sorted.
// 3. Hash itself is sorted in bytes ordering
func (b *BlockRef) LessThan(other *BlockRef) bool {
	if b.GetEmpty() {
		return false
	}
	if other.GetEmpty() {
		return true
	}
	bh := b.GetHash()
	oh := other.GetHash()
	bht := bh.GetHashType()
	oht := oh.GetHashType()
	if bht != oht {
		return bht < oht
	}
	return bytes.Compare(bh.GetHash(), oh.GetHash()) < 0
}

// MarshalJSON marshals the reference to a JSON string.
// Returns "" if the ref is nil.
func (b *BlockRef) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(b.MarshalString())), nil
}

// UnmarshalJSON unmarshals the reference from a JSON string.
// Also accepts an object (in jsonpb format).
func (b *BlockRef) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || b == nil {
		return nil
	}
	val, err := fastjson.ParseBytes(data)
	if err != nil {
		return err
	}
	return b.UnmarshalFastJSON(val)
}

// ParseFromB58 parses the object ref from a base58 string.
func (b *BlockRef) ParseFromB58(ref string) error {
	dat, err := b58.Decode(ref)
	if err != nil {
		return err
	}
	return b.UnmarshalVT(dat)
}

// UnmarshalFastJSON unmarshals the fast json container.
// If the val or object ref are nil, does nothing.
func (b *BlockRef) UnmarshalFastJSON(val *fastjson.Value) error {
	if val == nil || b == nil {
		return nil
	}
	switch val.Type() {
	case fastjson.TypeString:
		return b.ParseFromB58(string(val.GetStringBytes()))
	case fastjson.TypeObject:

	default:
		return errors.Errorf("unexpected json type for object ref: %v", val.Type().String())
	}

	// hash
	if hashVal := val.Get("hash"); hashVal != nil {
		bh, err := hash.UnmarshalHashFastJSON(hashVal)
		if err != nil {
			return errors.Wrap(err, "hash")
		}
		b.Hash = bh
	}

	return nil
}

// _ is a type assertion
var (
	_ Block         = ((*BlockRef)(nil))
	_ BlockWithRefs = ((*BlockRef)(nil))

	_ json.Marshaler   = ((*BlockRef)(nil))
	_ json.Unmarshaler = ((*BlockRef)(nil))
)
