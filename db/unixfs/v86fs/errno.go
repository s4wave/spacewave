package unixfs_v86fs

import (
	"context"
	"io"
	"io/fs"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// ATTR_* valid mask bits (matching Linux kernel).
const (
	attrMode = 1
	attrSize = 8
)

// DT_* types (matching Linux dirent.h).
const (
	dtDir = 4
	dtReg = 8
	dtLnk = 10
)

// S_IF* mode bits (matching Linux stat.h).
const (
	sIFDIR = 0o040000
	sIFREG = 0o100000
	sIFLNK = 0o120000
)

// errno values (matching Linux errno.h).
const (
	enoent  = 2
	eexist  = 17
	enotdir = 20
	einval  = 22
	enosys  = 38
)

// errnoFromError maps a Go error to a v86fs errno value.
func errnoFromError(err error) uint32 {
	if err == nil {
		return 0
	}
	if errors.Is(err, unixfs_errors.ErrNotExist) || errors.Is(err, fs.ErrNotExist) {
		return enoent
	}
	if errors.Is(err, unixfs_errors.ErrExist) || errors.Is(err, fs.ErrExist) {
		return eexist
	}
	if errors.Is(err, unixfs_errors.ErrNotDirectory) {
		return enotdir
	}
	if errors.Is(err, fs.ErrInvalid) {
		return einval
	}
	if errors.Is(err, io.EOF) {
		return 0
	}
	return enosys
}

// nodeTypeToMode converts an FSCursorNodeType to S_IF* mode bits.
func nodeTypeToMode(nt interface {
	GetIsDirectory() bool
	GetIsFile() bool
	GetIsSymlink() bool
},
) uint32 {
	if nt == nil {
		return sIFREG
	}
	if nt.GetIsDirectory() {
		return sIFDIR
	}
	if nt.GetIsSymlink() {
		return sIFLNK
	}
	return sIFREG
}

// nodeTypeToDtType converts an FSCursorDirent to a DT_* type.
func nodeTypeToDtType(ent interface {
	GetIsDirectory() bool
	GetIsSymlink() bool
},
) uint32 {
	if ent.GetIsDirectory() {
		return dtDir
	}
	if ent.GetIsSymlink() {
		return dtLnk
	}
	return dtReg
}

// getNodeMode returns the combined S_IF* mode and permission bits for an FSHandle.
func getNodeMode(ctx context.Context, h *unixfs.FSHandle) (uint32, error) {
	nodeType, err := h.GetNodeType(ctx)
	if err != nil {
		return 0, err
	}
	mode := nodeTypeToMode(nodeType)
	perm, err := h.GetPermissions(ctx)
	if err != nil {
		return 0, err
	}
	return mode | uint32(perm), nil
}
