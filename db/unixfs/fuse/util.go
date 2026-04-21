//go:build linux
// +build linux

package fuse

import (
	"context"
	"os"
	"time"

	"bazil.org/fuse"
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
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())

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
