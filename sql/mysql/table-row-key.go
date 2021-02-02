package mysql

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// MarshalTableRowKey marshals the key for a table row.
func MarshalTableRowKey(nonce uint64) []byte {
	// the key must be sortable by []byte
	dat := make([]byte, 8)
	binary.BigEndian.PutUint64(dat, nonce)
	return dat
}

// UnmarshalTableRowKey unmarshals the nonce for a table row.
func UnmarshalTableRowKey(dat []byte) (uint64, error) {
	if len(dat) != 8 {
		return 0, errors.Errorf("expected 8 bytes, got %d bytes for uint64", len(dat))
	}
	return binary.BigEndian.Uint64(dat), nil
}
