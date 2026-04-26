//go:build linux
// +build linux

package fuse

import (
	"context"
	"os"
	"time"

	"bazil.org/fuse"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

const (
	// nodeValidTime is the amount of time the kernel will keep a inode in cache.
	// we notify the kernel of changes, so this is a larger value.
	// the kernel may still forget some parts of the tree under memory pressure
	nodeValidTime = time.Minute * 5
)

// FsOpsToAttr computes inode attributes for a FSCursorOps.
func FsOpsToAttr(ctx context.Context, node *unixfs.FSHandle, out *fuse.Attr) error {
	// TODO: many values are defaulted for now.
	nt, err := node.GetNodeType(ctx)
	if err != nil {
		return err
	}
	fileInfo, err := node.GetFileInfo(ctx)
	if err != nil {
		return err
	}

	out.Mode = fileInfo.Mode()
	out.Valid = nodeValidTime

	if nt.GetIsFile() {
		size := fileInfo.Size()
		if size < 0 {
			return errors.New("negative file size")
		}
		out.Size = uint64(size)

		// The blocks size must be calculated correctly:
		// (out.Size + blockSize - 1) / blockSize
		if size != 0 {
			out.Blocks = (out.Size + refBlockSize - 1) / refBlockSize
		}

		// note: this field is most likely ignored by the kernel
		// "preferred block size for i/o"
		// use the standard 512 bytes size
		out.BlockSize = refBlockSize
	}

	// Nlink - not very well documented - sshfs uses constant 1
	// it's supposedly used to indicate the # of children in a directory
	// "find" might care about it
	out.Nlink = 1

	// NOTE: all values use the modification time
	modTime := fileInfo.ModTime()
	out.Atime = modTime // time of last access, use mod time
	out.Mtime = modTime // time of last modification
	out.Ctime = modTime // time of last inode change

	/* TODO: ownership, device nodes
	Uid       uint32      // owner uid
	Gid       uint32      // group gid
	Rdev      uint32      // device numbers
	*/
	uid := os.Getuid()
	if uid < 0 {
		return errors.New("negative uid")
	}
	if uint64(uid) > uint64(^uint32(0)) {
		return errors.New("uid exceeds uint32")
	}
	out.Uid = uint32(uid) //nolint:gosec // guarded above
	gid := os.Getgid()
	if gid < 0 {
		return errors.New("negative gid")
	}
	if uint64(gid) > uint64(^uint32(0)) {
		return errors.New("gid exceeds uint32")
	}
	out.Gid = uint32(gid) //nolint:gosec // guarded above

	return nil
}

// DirentToFuseDirent converts a dirent to a fuse dirent.
func DirentToFuseDirent(dirent unixfs.FSCursorDirent, out *fuse.Dirent) error {
	out.Name = dirent.GetName()
	out.Type = NodeTypeToDirentType(dirent)
	return nil
}

// NodeTypeToDirentType converts a fstree node type into a fuse node type.
func NodeTypeToDirentType(nodeType unixfs.FSCursorNodeType) fuse.DirentType {
	if nodeType.GetIsDirectory() {
		return fuse.DT_Dir
	}
	// TODO default to file for now
	// nodeType.GetIsFile()
	return fuse.DT_File
}
