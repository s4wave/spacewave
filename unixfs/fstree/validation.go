package fstree

import (
	"errors"
	"unicode/utf8"
)

var (
	// ErrDirectoryNameInvalidUnicode indicates a directory name must be  utf8
	ErrDirectoryNameInvalidUTF8 = errors.New("directory name must be valid utf8")
	// ErrDirectoryNameInvalidReserved
	ErrDirectoryNameInvalidReserved = errors.New("directory name cannot be reserved")
	// ErrDirectoryNameEmpty indicates a directory name cannot be empty.
	ErrDirectoryNameEmpty = errors.New("directory name cannot be empty")
)

// ValidateDirectoryName checks if the directory name is valid.
// Names must be UTF-8 characters.
// Not allowed: /, \, /, :, *, ", <, >, |
// Reserved: "..", "."
//
// POSIX: a-z, A-Z, 0-9, ., _, -, or space.
func ValidateDirectoryName(name string) error {
	if name == "" {
		return ErrDirectoryNameEmpty
	}
	if !utf8.ValidString(name) {
		return ErrDirectoryNameInvalidUTF8
	}
	if name == ".." || name == "." {
		return ErrDirectoryNameInvalidReserved
	}
	return nil
}
