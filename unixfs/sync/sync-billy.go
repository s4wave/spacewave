package unixfs_sync

import (
	"context"
	"io/fs"
	"os"
	"path"
	"sort"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/util/mbuffer"
	"github.com/aperturerobotics/util/scrub"
	"github.com/go-git/go-billy/v5"
	"github.com/pkg/errors"
)

// BillyFS has the needed billy filesystem interfaces.
type BillyFS interface {
	billy.Basic
	billy.Dir
}

// SyncToBilly recursively synchronizes the contents of the UnixFS to a BillyFS.
//
// Attempts to skip files by checking size and modification time.
// The output path does not have to be empty when starting.
// TODO: Does not (yet) support symlinks or other non-file and non-dir node types.
func SyncToBilly(
	ctx context.Context,
	bfs BillyFS,
	fsHandle *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	switch deleteMode {
	case DeleteMode_DeleteMode_BEFORE:
		if err := syncToBillyOnce(ctx, bfs, fsHandle, true, false, filterCb); err != nil {
			return err
		}
		return syncToBillyOnce(ctx, bfs, fsHandle, false, true, filterCb)
	case DeleteMode_DeleteMode_DURING:
		return syncToBillyOnce(ctx, bfs, fsHandle, true, true, filterCb)
	case DeleteMode_DeleteMode_AFTER:
		if err := syncToBillyOnce(ctx, bfs, fsHandle, false, true, filterCb); err != nil {
			return err
		}
		return syncToBillyOnce(ctx, bfs, fsHandle, true, false, filterCb)
	case DeleteMode_DeleteMode_ONLY:
		return syncToBillyOnce(ctx, bfs, fsHandle, true, false, filterCb)
	case DeleteMode_DeleteMode_NONE:
		return syncToBillyOnce(ctx, bfs, fsHandle, false, true, filterCb)
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
) error {
	if fsHandle.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if bfs == nil {
		return nil
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
		for i := len(stack) - 1; i >= 0; i-- {
			stack[i].fsHandle.Release()
		}
	}()

	// add initial stack element
	pushStack(fsHandle, "", "")

	// copy buffer
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
			_, err := bfs.Stat(outPath)
			if err == unixfs_errors.ErrNotExist || os.IsNotExist(err) {
				continue
			}
			if err != nil {
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
			// sort childNames
			sort.Strings(childNames)
			// we can check if the child exists via a sorted search
			checkChildExists := func(name string) bool {
				idx := sort.SearchStrings(childNames, name)
				if idx < 0 || idx >= len(childNames) {
					return false
				}
				return childNames[idx] == name
			}

			// delete: remove any entries that shouldn't exist
			if doDelete {
				outEntries, err := bfs.ReadDir(outPath)
				if err != nil {
					releaseElem(nelem)
					return err
				}
				for _, entry := range outEntries {
					_, entryName := path.Split(entry.Name())
					if !checkChildExists(entryName) {
						// delete from destination
						// skip if filterCb mismatch
						if filterCb != nil {
							srcDelPath := path.Join(srcPath, entryName)
							cntu, err := filterCb(ctx, srcDelPath, nil)
							if err != nil {
								return err
							}
							if !cntu {
								continue
							}
						}
						if filterCb != nil {
							filterPath := path.Join(srcPath, entryName)
							nodeType, err := unixfs.FileModeToNodeType(entry.Mode())
							if err != nil {
								return err
							}
							cntu, err := filterCb(ctx, filterPath, nodeType)
							if err != nil || !cntu {
								return err
							}
						}

						err = bfs.Remove(path.Join(outPath, entryName))
						if err != nil {
							releaseElem(nelem)
							return err
						}
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
		if !srcFileInfo.Mode().IsRegular() || !doWrite {
			// skip any non-regular files.
			releaseElem(nelem)
			continue
		}

		// if createTruncateFile is set, we will create the file, truncating
		// existing contents, and do a full copy from src to destination.
		var createTruncateFile bool

		// first: check if we can skip this file with heuristics.
		// if the modification time is set on both source and destination,
		// the modification time matches, and the file size matches,
		// we can conclude the file is /most likely/ the same and skip it.
		outFileInfo, outStatErr := bfs.Stat(outPath)
		if outStatErr == unixfs_errors.ErrNotExist || os.IsNotExist(outStatErr) {
			createTruncateFile = true
		}
		if outStatErr == nil {
			// if dst file exists already, update its permissions if necessary.
			outPerms := outFileInfo.Mode().Perm()
			srcPerms := srcFileInfo.Mode().Perm()
			if outPerms != srcPerms {
				// TODO: Chmod does not exist on billy.Filesystem nor osfs!
				// bfs.Chmod(outPath, srcPerms)
				// try to just fully truncate / overwrite the file instead.
				createTruncateFile = true
			}

			srcModTime := srcFileInfo.ModTime()
			outModTime := outFileInfo.ModTime()
			dstIdenticalToSrc := outFileInfo.Size() == srcFileInfo.Size() && !outModTime.IsZero() && outModTime.Equal(srcModTime)
			if dstIdenticalToSrc && !createTruncateFile {
				// the files look identical by size and mod time.
				// skip this file.
				releaseElem(nelem)
				continue
			}
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

		xferBuf := cpyBuffer.GetOrAllocate(32 * 1024)
		if createTruncateFile {
			err = unixfs.CopyToBillyFSFile(ctx, of, handle, xferBuf, 0)
		} else {
			wbuffer := writeBuffer.GetOrAllocate(32 * 1024)
			err = unixfs.SyncToBillyFSFile(ctx, of, handle, xferBuf, wbuffer)
		}

		if cerr := of.Close(); err == nil && cerr != nil {
			err = cerr
		}

		releaseElem(nelem)
		scrub.Scrub(xferBuf)
		if err != nil {
			return &fs.PathError{Op: "write", Path: outPath, Err: err}
		}
	}

	return nil
}
