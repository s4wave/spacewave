package unixfs_block

import (
	"strings"
	"unicode/utf8"

	"github.com/s4wave/spacewave/db/unixfs"
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

// ValidateDirentName checks if the directory entry name is valid.
// Names must be UTF-8 characters.
// Not allowed: /, \, /, :, *, ", <, >, |
// Reserved: "..", "."
//
// POSIX: a-z, A-Z, 0-9, ., _, -, or space.
func ValidateDirentName(name string) error {
	if name == "" {
		return ErrDirectoryNameEmpty
	}
	if !utf8.ValidString(name) {
		return ErrDirectoryNameInvalidUTF8
	}
	if name == ".." || name == "." {
		return ErrDirectoryNameInvalidReserved
	}
	if strings.ContainsRune(name, unixfs.PathSeparator) {
		return errors.Errorf("name cannot contain path separator %s: %s", string([]rune{unixfs.PathSeparator}), name)
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
		if err := p.Validate(false, false); err != nil {
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
	if err := path.Validate(false, false); err != nil {
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
		if err := p.Validate(false, false); err != nil {
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
		if err := p.Validate(false, false); err != nil {
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
		if err := p.Validate(false, false); err != nil {
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
	if err := path.Validate(false, false); err != nil {
		return err
	}
	return nil
}

// ValidateTruncate validates a truncate operation parameter set.
func ValidateTruncate(path *FSPath, size int64) error {
	if size < 0 {
		return errors.New("expected positive file size")
	}
	if err := path.Validate(false, false); err != nil {
		return err
	}
	return nil
}

// ValidateCopy validates a copy operation.
func ValidateCopy(srcPath, destPath *FSPath) error {
	if err := srcPath.Validate(false, false); err != nil {
		return err
	}
	if err := destPath.Validate(false, false); err != nil {
		return err
	}
	return nil
}

// ValidateRename validates a move operation.
func ValidateRename(srcPath, destPath *FSPath) error {
	if err := ValidateCopy(srcPath, destPath); err != nil {
		return err
	}
	return nil
}
