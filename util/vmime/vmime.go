package vmime

import (
	"errors"
	"regexp"
)

// MimeTypeRe is used to check mime types.
var MimeTypeRe = regexp.MustCompile(`^[-\w.]+/[-\w.]+$`)

// IsValidMimeType checks if a string is a valid mime type.
func IsValidMimeType(str string) bool {
	return MimeTypeRe.MatchString(str)
}

// ErrInvalidMimeType is returned if the mime type is invalid.
var ErrInvalidMimeType = errors.New("invalid mime type")
