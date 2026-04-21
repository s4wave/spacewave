//go:build !js

package bldr_project_starlark

import (
	"strconv"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// valueToJSON converts a Starlark value to JSON bytes.
// Supports: string, int, float, bool, None, list, tuple, dict.
func valueToJSON(val starlark.Value) ([]byte, error) {
	var buf []byte
	var err error
	buf, err = appendJSON(buf, val)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// appendJSON appends the JSON representation of a Starlark value to buf.
func appendJSON(buf []byte, val starlark.Value) ([]byte, error) {
	switch v := val.(type) {
	case starlark.NoneType:
		return append(buf, "null"...), nil
	case starlark.Bool:
		if v {
			return append(buf, "true"...), nil
		}
		return append(buf, "false"...), nil
	case starlark.Int:
		i, ok := v.Int64()
		if ok {
			return strconv.AppendInt(buf, i, 10), nil
		}
		u, ok := v.Uint64()
		if ok {
			return strconv.AppendUint(buf, u, 10), nil
		}
		return nil, errors.New("integer value out of range")
	case starlark.Float:
		return strconv.AppendFloat(buf, float64(v), 'g', -1, 64), nil
	case starlark.String:
		return appendJSONString(buf, string(v)), nil
	case *starlark.List:
		buf = append(buf, '[')
		for i := range v.Len() {
			if i > 0 {
				buf = append(buf, ',')
			}
			var err error
			buf, err = appendJSON(buf, v.Index(i))
			if err != nil {
				return nil, err
			}
		}
		return append(buf, ']'), nil
	case starlark.Tuple:
		buf = append(buf, '[')
		for i, item := range v {
			if i > 0 {
				buf = append(buf, ',')
			}
			var err error
			buf, err = appendJSON(buf, item)
			if err != nil {
				return nil, err
			}
		}
		return append(buf, ']'), nil
	case *starlark.Dict:
		buf = append(buf, '{')
		items := v.Items()
		for i, item := range items {
			if i > 0 {
				buf = append(buf, ',')
			}
			key, ok := item[0].(starlark.String)
			if !ok {
				return nil, errors.Errorf("dict key must be a string, got %s", item[0].Type())
			}
			buf = appendJSONString(buf, string(key))
			buf = append(buf, ':')
			var err error
			buf, err = appendJSON(buf, item[1])
			if err != nil {
				return nil, err
			}
		}
		return append(buf, '}'), nil
	default:
		return nil, errors.Errorf("cannot convert %s to JSON", val.Type())
	}
}

// appendJSONString appends a JSON-encoded string to buf.
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			buf = append(buf, '\\', c)
		case c == '\n':
			buf = append(buf, '\\', 'n')
		case c == '\r':
			buf = append(buf, '\\', 'r')
		case c == '\t':
			buf = append(buf, '\\', 't')
		case c < 0x20:
			buf = append(buf, '\\', 'u', '0', '0', hexDigit(c>>4), hexDigit(c&0xf))
		default:
			buf = append(buf, c)
		}
	}
	return append(buf, '"')
}

func hexDigit(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + b - 10
}
