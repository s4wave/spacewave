package mysql

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// TableRowKeyEndian is the endian-ness to use for table keys.
// Note: this is selected to make keys sort in the correct order.
var TableRowKeyEndian = binary.BigEndian

// MarshalTableRowKey marshals the key for a table row.
func MarshalTableRowKey(nonce uint64) []byte {
	// the key must be sortable by []byte
	dat := make([]byte, 8)
	TableRowKeyEndian.PutUint64(dat, nonce)
	return dat
}

// UnmarshalTableRowKey unmarshals the nonce for a table row.
func UnmarshalTableRowKey(dat []byte) (uint64, error) {
	if len(dat) != 8 {
		return 0, errors.Errorf("expected 8 bytes, got %d bytes for uint64", len(dat))
	}
	return TableRowKeyEndian.Uint64(dat), nil
}
