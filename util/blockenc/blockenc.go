package blockenc

import "github.com/pkg/errors"

// BlockEnc_BlockEnc_MAX is the maximum value for BlockCrypt.
const BlockEnc_BlockEnc_MAX = BlockEnc_BlockEnc_SECRET_BOX

// BlockEnc_KeySize is the set of known key sizes.
var BlockEnc_KeySize = map[BlockEnc]int{
	BlockEnc_BlockEnc_SECRET_BOX:         32,
	BlockEnc_BlockEnc_XCHACHA20_POLY1305: 32,
}

// ValidateKeySize checks the given key size is the expected.
func ValidateKeySize(e BlockEnc, keySize int) error {
	expected, expectedOk := BlockEnc_KeySize[e]
	if !expectedOk {
		return errors.Errorf("unknown blockenc key size: %s", e.String())
	}
	if keySize != expected {
		return errors.Errorf("unexpected key size: %s requires %v but got %v", e.String(), expected, keySize)
	}
	return nil
}

// BuildBlockEnc builds block enc methods from known types.
func BuildBlockEnc(enc BlockEnc, key []byte) (Method, error) {
	switch enc {
	case BlockEnc_BlockEnc_UNKNOWN:
		fallthrough
	case BlockEnc_BlockEnc_NONE:
		return NewNoop(), nil
	case BlockEnc_BlockEnc_XCHACHA20_POLY1305:
		return NewXChaCha20Poly1305(key)
	case BlockEnc_BlockEnc_SECRET_BOX:
		return NewSecretBox(key)
	default:
		return nil, errors.Errorf("unknown blockenc type: %s", enc.String())
	}
}

// Validate checks if the blockenc is in the known set.
func (e BlockEnc) Validate() error {
	switch e {
	case BlockEnc_BlockEnc_UNKNOWN:
		fallthrough
	case BlockEnc_BlockEnc_NONE:
		return nil
	case BlockEnc_BlockEnc_XCHACHA20_POLY1305:
		return nil
	case BlockEnc_BlockEnc_SECRET_BOX:
		return nil
	default:
		return errors.Errorf("unknown blockenc type: %s", e.String())
	}
}
