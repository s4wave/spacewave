package unixfs_block

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

// ValidateMknod validates a mknod operation parameter set.
func ValidateMknod(paths []*FSPath, nodeType NodeType) error {
	if len(paths) == 0 {
		return errors.New("expected at least one path for mknod")
	}
	for _, p := range paths {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateWrite validates a write operation parameter set.
func ValidateWrite(path *FSPath, offset int64) error {
	if offset < 0 {
		return errors.New("expected positive offset")
	}
	if err := path.Validate(); err != nil {
		return err
	}
	return nil
}
