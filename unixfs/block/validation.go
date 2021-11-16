package unixfs_block

import (
	"unicode/utf8"

	"github.com/pkg/errors"
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
	switch nodeType {
	case NodeType_NodeType_DIRECTORY:
	case NodeType_NodeType_FILE:
	default:
		return errors.Errorf("invalid node type for mknod: %s", nodeType.String())
	}
	for _, p := range paths {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateSymlink validates a symbolic link operation.
func ValidateSymlink(path *FSPath, tgt *FSSymlink) error {
	if err := tgt.Validate(); err != nil {
		return err
	}
	if err := path.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateSetModTimestamp validates a set mod timestamp operation parameter set.
func ValidateSetModTimestamp(paths []*FSPath) error {
	if len(paths) == 0 {
		return errors.New("expected at least one path for set modification timestamp")
	}
	for _, p := range paths {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateSetPermissions validates a set mod timestamp operation parameter set.
func ValidateSetPermissions(paths []*FSPath, perms uint32) error {
	if len(paths) == 0 {
		return errors.New("expected at least one path for set permissions")
	}
	for _, p := range paths {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	// xxx: check permissions bits here?
	return nil
}

// ValidateRemove validates a remove operation parameter set.
func ValidateRemove(paths []*FSPath) error {
	if len(paths) == 0 {
		return errors.New("expected at least one path for remove")
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

// ValidateTruncate validates a truncate operation parameter set.
func ValidateTruncate(path *FSPath, size int64) error {
	if size < 0 {
		return errors.New("expected positive file size")
	}
	if err := path.Validate(); err != nil {
		return err
	}
	return nil
}
