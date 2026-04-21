package protocol

import "unicode/utf8"

// ID is a protocol identifier.
type ID string

func (i ID) Validate() error {
	if i == "" {
		return ErrEmptyProtocolID
	}

	if !utf8.ValidString(string(i)) {
		return ErrInvalidProtocolID
	}

	// TODO: additional constraints on the protocol id
	return nil
}

// String returns the ID as a string.
func (i ID) String() string {
	return string(i)
}

// IDsToString converts a slice of IDs to strings.
func IDsToString(ids []ID) []string {
	out := make([]string, len(ids))
	for i := range ids {
		out[i] = ids[i].String()
	}
	return out
}
