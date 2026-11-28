package refwalk

import (
	"reflect"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ErrInexactDecode is returned when bytes decode as a type that does not
// explain all of them: the decoded message kept fields its type does not
// declare, or its encoding is a different length than the bytes.
var ErrInexactDecode = errors.New("block decodes inexactly as its type")

// DecodeExact decodes data into blk and checks that blk's type explains every
// byte. A block written by its own type always decodes exactly. Bytes of
// another type, or of a newer schema, can decode without error while their
// refs sit in fields the type does not declare, so a walk that trusts such a
// decode misses those refs. DecodeExact returns ErrInexactDecode for them.
//
// The check has two parts. No message in the decoded tree may keep unknown
// fields. The canonical encoding must have the same length as data, which
// rules out repeated singular fields, explicit default values and padded
// varints. Lengths are compared rather than bytes because map entries
// marshal in no fixed order.
func DecodeExact(blk block.Block, data []byte) error {
	// Decode, then check the decoded tree and its encoded length.
	if err := blk.UnmarshalVT(data); err != nil {
		return err
	}
	if hasUnknownFields(reflect.ValueOf(blk)) {
		return errors.Wrap(ErrInexactDecode, "unknown fields")
	}
	n, err := encodedSize(blk)
	if err != nil {
		return err
	}
	if n != len(data) {
		return errors.Wrapf(ErrInexactDecode, "encodes to %d of %d bytes", n, len(data))
	}
	return nil
}

// sizer is a block that reports its encoded size, as protobuf-go-lite
// messages do.
type sizer interface {
	// SizeVT returns the length of the message's encoding.
	SizeVT() int
}

// encodedSize returns the length of blk's canonical encoding.
func encodedSize(blk block.Block) (int, error) {
	if s, ok := blk.(sizer); ok {
		return s.SizeVT(), nil
	}
	out, err := blk.MarshalVT()
	return len(out), err
}

// unknownFieldsName is the field in which protobuf-go-lite messages keep the
// bytes of fields their type does not declare.
const unknownFieldsName = "unknownFields"

// hasUnknownFields reports whether any message reachable from v keeps unknown
// fields. Decoded messages form a tree, so the walk needs no visited set. It
// reads unexported fields only through Len, which reflect allows.
func hasUnknownFields(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return !v.IsNil() && hasUnknownFields(v.Elem())
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			f := v.Field(i)
			if t.Field(i).Name == unknownFieldsName && f.Kind() == reflect.Slice {
				if f.Len() != 0 {
					return true
				}
				continue
			}
			if hasUnknownFields(f) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		if !holdsMessages(v.Type().Elem()) {
			return false
		}
		for i := range v.Len() {
			if hasUnknownFields(v.Index(i)) {
				return true
			}
		}
	case reflect.Map:
		if !holdsMessages(v.Type().Elem()) {
			return false
		}
		for it := v.MapRange(); it.Next(); {
			if hasUnknownFields(it.Value()) {
				return true
			}
		}
	}
	return false
}

// holdsMessages reports whether values of type t can contain a message, so
// that a slice of bytes or scalars is skipped without visiting each element.
func holdsMessages(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
		return true
	}
	return false
}
