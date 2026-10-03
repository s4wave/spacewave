package unixfs_sync

import (
	"context"
	"io/fs"
	"os"
	"path"
	"slices"

	"github.com/aperturerobotics/util/scrub"
	"github.com/go-git/go-billy/v6"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/util/mbuffer"
)

// copyBufferSize is the size of each file write. A UnixFS file rewrites its
// last blob block, up to blob.DefChunkingMaxSize, on every write, so writes
// smaller than that block rewrite it many times.
const copyBufferSize = 4 << 20

// BillyFS has the needed billy filesystem interfaces.
type BillyFS interface {
	billy.Basic
	billy.Dir
}

// ChangeKind classifies a destination file changed by a sync.
type ChangeKind int

const (
	// ChangeWrite rewrote an existing file.
	ChangeWrite ChangeKind = iota
	// ChangeCreate created a file.
	ChangeCreate
	// ChangeRemove removed a file.
	ChangeRemove
)

// ChangeCb observes a destination file after the sync changes it.
// Removing a directory reports each file it contained.
type ChangeCb func(outPath string, kind ChangeKind)

// SyncToBilly recursively synchronizes the contents of the UnixFS to a BillyFS.
//
// Attempts to skip files by checking size and modification time.
// The output path does not have to be empty when starting.
// TODO: Node types other than directory, regular file, and symlink are not supported.
func SyncToBilly(
	ctx context.Context,
	bfs BillyFS,
	fsHandle *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	return syncToBilly(ctx, bfs, fsHandle, deleteMode, filterCb, false, nil)
}

// SyncToBillyContents compares regular-file bytes even when size and mtime match.
// Unchanged files are left intact so source watchers only observe actual edits.
// changeCb, if set, observes each destination file the sync changes.
func SyncToBillyContents(
	ctx context.Context,
	bfs BillyFS,
	fsHandle *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
	changeCb ChangeCb,
) error {
	return syncToBilly(ctx, bfs, fsHandle, deleteMode, filterCb, true, changeCb)
}

// syncToBilly applies the selected deletion policy and content comparison mode.
func syncToBilly(
	ctx context.Context,
	bfs BillyFS,
	fsHandle *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
	compareContents bool,
	changeCb ChangeCb,
) error {
	once := func(doDelete, doWrite bool) error {
		return syncToBillyOnce(ctx, bfs, fsHandle, doDelete, doWrite, filterCb, compareContents, changeCb)
	}
	switch deleteMode {
	case DeleteMode_DeleteMode_BEFORE:
		if err := once(true, false); err != nil {
			return err
		}
		return once(false, true)
	case DeleteMode_DeleteMode_DURING:
		return once(true, true)
	case DeleteMode_DeleteMode_AFTER:
		if err := once(false, true); err != nil {
			return err
		}
		return once(true, false)
	case DeleteMode_DeleteMode_ONLY:
		return once(true, false)
	case DeleteMode_DeleteMode_NONE:
		return once(false, true)
	default:
		return errors.Errorf("unknown delete mode: %s", deleteMode.String())
	}
}

// syncToBillyOnce implements a SyncToBilly step.
func syncToBillyOnce(
	ctx context.Context,
	bfs BillyFS,
	fsHandle *unixfs.FSHandle,
	doDelete bool,
	doWrite bool,
	filterCb FilterCb,
	compareContents bool,
	changeCb ChangeCb,
) error {
	// Skip a released source or a missing destination.
	if fsHandle.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if bfs == nil {
		return nil
	}

	// Stat destination symlinks themselves when the filesystem supports them.
	bfsStat := bfs.Stat
	bfsSymlink, bfsSymlinkOk := bfs.(billy.Symlink)
	if bfsSymlinkOk {
		bfsStat = bfsSymlink.Lstat
	}

	// stackElem is a element in the fs location stack.
	type stackElem struct {
		// fsHandle is the filesystem handle.
		fsHandle *unixfs.FSHandle
		// srcPath is the path in the source fs
		srcPath string
		// outPath is the path in the output fs.
		outPath string
	}

	// Track the directories left to visit, releasing each handle the walk
	// opened.
	stack := make([]stackElem, 0, 10)
	pushStack := func(fsHandle *unixfs.FSHandle, srcPath, outPath string) {
		stack = append(stack, stackElem{
			fsHandle: fsHandle,
			srcPath:  srcPath,
			outPath:  outPath,
		})
	}
	releaseElem := func(elem stackElem) {
		if elem.srcPath != "" {
			elem.fsHandle.Release()
		}
	}
	defer func() {
		for _, v := range slices.Backward(stack) {
			v.fsHandle.Release()
		}
	}()

	// Start the walk at the source root with reusable copy buffers.
	pushStack(fsHandle, "", "")
	var cpyBuffer, writeBuffer mbuffer.MBuffer

	// recursively traverse filesystem
	for len(stack) != 0 {
		nelem := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		handle := nelem.fsHandle
		srcPath := nelem.srcPath
		outPath := nelem.outPath
		srcFileInfo, err := handle.GetFileInfo(ctx)
		if err != nil {
			releaseElem(nelem)
			return &fs.PathError{Op: "stat", Path: srcPath, Err: err}
		}

		// if !doWrite and the destination doesn't exist, skip this node.
		if !doWrite {
			_, err := bfsStat(outPath)
			if err == unixfs_errors.ErrNotExist || os.IsNotExist(err) {
				continue
			}
			if err != nil {
				releaseElem(nelem)
				return &fs.PathError{Op: "stat", Path: outPath, Err: err}
			}
		}

		// if directory
		if srcFileInfo.IsDir() {
			// call MkdirAll to create the destination in case it doesn't exist.
			// if !doWrite, we already checked that outPath exists above.
			if doWrite {
				if err := bfs.MkdirAll(outPath, srcFileInfo.Mode().Perm()); err != nil {
					releaseElem(nelem)
					return &fs.PathError{Op: "mkdir", Path: outPath, Err: err}
				}
			}

			// iterate over source directory contents & enqueue
			var childNames []string
			err = handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
				name := ent.GetName()
				if filterCb != nil {
					filterPath := path.Join(srcPath, name)
					cntu, err := filterCb(ctx, filterPath, ent)
					if err != nil || !cntu {
						return err
					}
				}
				childNames = append(childNames, name)
				return nil
			})
			if err != nil {
				releaseElem(nelem)
				return &fs.PathError{Op: "readdir", Path: outPath, Err: err}
			}
			slices.Sort(childNames)

			// delete: remove any entries that shouldn't exist
			if doDelete {
				outEntries, err := bfs.ReadDir(outPath)
				if err != nil {
					releaseElem(nelem)
					return err
				}
				for _, entry := range outEntries {
					_, entryName := path.Split(entry.Name())
					if _, exists := slices.BinarySearch(childNames, entryName); exists {
						continue
					}

					// Filtered destination entries are outside the sync.
					if filterCb != nil {
						nodeType, err := unixfs.FileModeToNodeType(entry.Type())
						if err != nil {
							releaseElem(nelem)
							return err
						}
						cntu, err := filterCb(ctx, path.Join(srcPath, entryName), nodeType)
						if err != nil {
							releaseElem(nelem)
							return err
						}
						if !cntu {
							continue
						}
					}

					// List the files before removing them so each can be reported.
					delPath := path.Join(outPath, entryName)
					var removed []string
					if changeCb != nil {
						removed, err = listBillyFiles(bfs, delPath, entry)
						if err != nil {
							releaseElem(nelem)
							return err
						}
					}
					if err := billy_util.RemoveAll(bfs, delPath); err != nil {
						releaseElem(nelem)
						return err
					}
					for _, name := range removed {
						changeCb(name, ChangeRemove)
					}
				}
			}

			// visit any child entries that should exist
			for _, childName := range childNames {
				childHandle, err := handle.Lookup(ctx, childName)
				if err != nil {
					releaseElem(nelem)
					if err == unixfs_errors.ErrNotExist {
						// skip files that disappeared from the source
						continue
					}
					return &fs.PathError{Op: "lookup", Path: path.Join(srcPath, childName), Err: err}
				}
				pushStack(
					childHandle,
					path.Join(srcPath, childName),
					path.Join(outPath, childName),
				)
			}

			// continue to next queued entry
			releaseElem(nelem)
			continue
		}

		// destination is not a directory, or we are not writing.
		if !doWrite {
			releaseElem(nelem)
			continue
		}

		// check file type
		srcFileMode := srcFileInfo.Mode()
		srcFileIsSymlink := srcFileMode&fs.ModeSymlink != 0
		srcFileIsRegular := !srcFileIsSymlink && srcFileMode.IsRegular()

		// all non-regular files other than symlinks are ignored
		if (!srcFileIsRegular && !srcFileIsSymlink) ||
			// if the destination doesn't support symlinks, skip it.
			(srcFileIsSymlink && !bfsSymlinkOk) {
			releaseElem(nelem)
			continue
		}

		// if symlink: read it
		var srcFileSymlinkPath string
		if srcFileIsSymlink {
			linkPath, linkPathIsAbs, err := handle.Readlink(ctx, "")
			if err != nil {
				releaseElem(nelem)
				return &fs.PathError{Op: "readlink", Path: srcPath, Err: err}
			}
			srcFileSymlinkPath = unixfs.JoinPath(linkPath, linkPathIsAbs)
		}

		// if createTruncateFile is set, we will create the file, truncating
		// existing contents, and do a full copy from src to destination.
		var createTruncateFile bool

		// first: check if we can skip this file with heuristics.
		// if the modification time is set on both source and destination,
		// the modification time matches, and the file size matches,
		// we can conclude the file is /most likely/ the same and skip it.
		var outFileInfo fs.FileInfo
		var outStatErr error

		outFileInfo, outStatErr = bfsStat(outPath)

		if outStatErr != nil {
			// outStatErr == unixfs_errors.ErrNotExist || os.IsNotExist(outStatErr)
			createTruncateFile = true
		}
		if !createTruncateFile {
			// create/truncate the file if the file type is different.
			createTruncateFile = outFileInfo.Mode().Type() != srcFileMode.Type()
		}

		var dstIdenticalToSrc bool
		if !createTruncateFile && srcFileIsRegular {
			// if dst file exists already, update its permissions if necessary.
			srcPerms, outPerms := srcFileInfo.Mode().Perm(), outFileInfo.Mode().Perm()
			if outPerms != srcPerms {
				// TODO: Chmod does not exist on billy.Filesystem nor osfs!
				// bfs.Chmod(outPath, srcPerms)

				// try to just fully truncate / overwrite the file instead.
				createTruncateFile = true
			}

			// Skip if the size and modification time are identical
			srcModTime, outModTime := srcFileInfo.ModTime(), outFileInfo.ModTime()
			dstIdenticalToSrc = !compareContents && outFileInfo.Size() == srcFileInfo.Size() &&
				!outModTime.IsZero() && outModTime.Equal(srcModTime)
		}

		if !createTruncateFile && srcFileIsSymlink {
			outLink, err := bfsSymlink.Readlink(outPath)
			if err != nil {
				createTruncateFile = true
			} else if outLink == srcFileSymlinkPath {
				dstIdenticalToSrc = true
			}
		}

		if dstIdenticalToSrc && !createTruncateFile {
			// the files look identical by size and mod time.
			// skip this file.
			releaseElem(nelem)
			continue
		}

		// create the symlink
		if srcFileIsSymlink {
			if outStatErr == nil || !os.IsNotExist(outStatErr) {
				// delete the existing file first
				if err := bfs.Remove(outPath); err != nil && !os.IsNotExist(err) {
					releaseElem(nelem)
					return &fs.PathError{Op: "remove", Path: outPath, Err: err}
				}
			}

			// create the symlink
			if err := bfsSymlink.Symlink(srcFileSymlinkPath, outPath); err != nil {
				releaseElem(nelem)
				// err is already a fs.PathError
				return err
			}
			if changeCb != nil {
				changeCb(outPath, writeKind(outStatErr))
			}

			// done with this entry
			releaseElem(nelem)
			continue
		}

		// if !createTruncFile and destination already exists
		fileOpts := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		if !createTruncateFile {
			fileOpts = os.O_RDWR
		}

		of, err := bfs.OpenFile(outPath, fileOpts, srcFileInfo.Mode().Perm())
		if err != nil {
			releaseElem(nelem)
			return &fs.PathError{Op: "openfile", Path: outPath, Err: err}
		}

		xferBuf := cpyBuffer.GetOrAllocate(copyBufferSize)
		changed := createTruncateFile
		if createTruncateFile {
			err = unixfs_billy.CopyToBillyFSFile(ctx, of, handle, xferBuf)
		} else {
			wbuffer := writeBuffer.GetOrAllocate(copyBufferSize)
			changed, err = unixfs_billy.SyncToBillyFSFile(ctx, of, handle, xferBuf, wbuffer)
		}

		if cerr := of.Close(); err == nil && cerr != nil {
			err = cerr
		}

		releaseElem(nelem)
		scrub.Scrub(xferBuf)
		if err != nil {
			return &fs.PathError{Op: "write", Path: outPath, Err: err}
		}
		if changed && changeCb != nil {
			changeCb(outPath, writeKind(outStatErr))
		}
	}

	return nil
}

// writeKind classifies a destination write by the prior destination stat.
func writeKind(outStatErr error) ChangeKind {
	if outStatErr != nil {
		return ChangeCreate
	}
	return ChangeWrite
}

// listBillyFiles lists the non-directory paths at or under a destination entry.
func listBillyFiles(bfs BillyFS, name string, entry fs.DirEntry) ([]string, error) {
	if !entry.IsDir() {
		return []string{name}, nil
	}
	children, err := bfs.ReadDir(name)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, child := range children {
		_, childName := path.Split(child.Name())
		childFiles, err := listBillyFiles(bfs, path.Join(name, childName), child)
		if err != nil {
			return nil, err
		}
		files = append(files, childFiles...)
	}
	return files, nil
}
