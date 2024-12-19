package ulid

import (
	"errors"
	"strings"

	"github.com/oklog/ulid/v2"
)

// ULID is the ulid type.
type ULID = ulid.ULID

// EncodedSize is the encoded length of a ulid.
const EncodedSize = ulid.EncodedSize

// ErrInvalidULID is returned if the ULID is in an invalid format.
var ErrInvalidULID = errors.New("invalid ulid")

// NewULID constructs a new randomized ulid in lowercase.
func NewULID() string {
	return strings.ToLower(ulid.Make().String())
}

// ParseULID parses and validates the lowercase ulid is the correct format.
func ParseULID(id string) (ULID, error) {
	var result ULID
	if len(id) != ulid.EncodedSize {
		return result, ErrInvalidULID
	}
	if strings.ToLower(id) != id {
		return result, ErrInvalidULID
	}
	upper := strings.ToUpper(id)
	res, err := ulid.ParseStrict(upper)
	if err != nil {
		return res, err
	}
	if res.Time() < 1257893000000 {
		return res, ErrInvalidULID
	}
	return res, nil
}
