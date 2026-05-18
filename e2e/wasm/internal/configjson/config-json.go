//go:build !js

package configjson

import (
	"slices"
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// Marshaler is the protobuf-go-lite JSON marshaler shape used by e2e config
// messages.
type Marshaler interface {
	MarshalJSON() ([]byte, error)
}

// MarshalCanonical marshals a protobuf-go-lite message to JSON with object
// keys sorted recursively. Generated protobuf-go-lite map fields currently use
// Go map iteration order, so harness-mutated builder config bytes otherwise
// churn between identical boots and invalidate the startup Manifest cache.
func MarshalCanonical(m Marshaler) ([]byte, error) {
	data, err := m.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	return appendCanonicalJSON(nil, value), nil
}

func appendCanonicalJSON(dst []byte, value *fastjson.Value) []byte {
	switch value.Type() {
	case fastjson.TypeObject:
		return appendCanonicalObject(dst, value.GetObject())
	case fastjson.TypeArray:
		dst = append(dst, '[')
		for i, element := range value.GetArray() {
			if i != 0 {
				dst = append(dst, ',')
			}
			dst = appendCanonicalJSON(dst, element)
		}
		return append(dst, ']')
	case fastjson.TypeString:
		return strconv.AppendQuote(dst, string(value.GetStringBytes()))
	case fastjson.TypeNumber:
		return append(dst, value.GetNumberAsStringBytes()...)
	case fastjson.TypeTrue:
		return append(dst, "true"...)
	case fastjson.TypeFalse:
		return append(dst, "false"...)
	case fastjson.TypeNull:
		return append(dst, "null"...)
	default:
		panic("unexpected fastjson value type")
	}
}

func appendCanonicalObject(dst []byte, obj *fastjson.Object) []byte {
	dst = append(dst, '{')
	keys := make([]string, 0, obj.Len())
	obj.Visit(func(key []byte, _ *fastjson.Value) {
		keys = append(keys, string(key))
	})
	slices.Sort(keys)
	for i, key := range keys {
		if i != 0 {
			dst = append(dst, ',')
		}
		dst = strconv.AppendQuote(dst, key)
		dst = append(dst, ':')
		dst = appendCanonicalJSON(dst, obj.Get(key))
	}
	return append(dst, '}')
}
