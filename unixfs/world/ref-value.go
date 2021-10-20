package unixfs_world

import (
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
)

// UnmarshalFromKey unmarshals the ref value from a key.
func UnmarshalRefValueFromKey(key string) (*RefValue, error) {
	v := &RefValue{}
	if len(key) == 0 {
		return v, nil
	}

	dat, err := b58.Decode(key)
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(dat, v); err != nil {
		return nil, err
	}
	return v, nil
}

// MarshalToKey marshals the ref value to a key.
func (v *RefValue) MarshalToKey() (string, error) {
	if v == nil {
		return "", nil
	}

	dv, err := proto.Marshal(v)
	if err != nil {
		return "", err
	}
	return b58.Encode(dv), nil
}
