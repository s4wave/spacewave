package unixfs_checkout

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/aperturerobotics/bifrost/util/scrub"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-billy/v5"
	"github.com/pkg/errors"
)

// errInvalidWrite means that a write returned an impossible count.
var errInvalidWrite = errors.New("invalid write result")

// BillyFS has the needed billy filesystem interfaces.
type BillyFS interface {
	billy.Basic
	billy.Dir
}

// CheckoutToBilly recursively copies the contents of the UnixFS to a BillyFS.
//
// Assumes that the output path is empty when starting.
// NOTE: Does not (yet) support symlinks or other non-file and non-dir node types.
func CheckoutToBilly(ctx context.Context, bfs BillyFS, fsHandle *unixfs.FSHandle) error {
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
	defer func() {
		for i := len(stack) - 1; i >= 0; i-- {
			stack[i].fsHandle.Release()
		}
	}()

	// add initial stack element
	pushStack(fsHandle, "", "")

	// copy buffer
	var cpyBuffer []byte
	allocCopyBuffer := func() []byte {
		if cap(cpyBuffer) != 0 {
			return cpyBuffer
		}
		cpyBuffer = make([]byte, 32*1024)
		return cpyBuffer
	}

	// recursively traverse filesystem
	for len(stack) != 0 {
		nelem := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		handle := nelem.fsHandle
		srcPath := nelem.srcPath
		outPath := nelem.outPath
		fi, err := handle.GetFileInfo(ctx)
		if err != nil {
			handle.Release()
			return &fs.PathError{Op: "stat", Path: srcPath, Err: err}
		}

		// if directory: call mkdir on destination
		if fi.IsDir() {
			if err := bfs.MkdirAll(outPath, fi.Mode().Perm()); err != nil {
				handle.Release()
				return &fs.PathError{Op: "mkdir", Path: outPath, Err: err}
			}

			// iterate over source directory contents & enqueue
			var childNames []string
			err = handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
				name := ent.GetName()
				childNames = append(childNames, name)
				return nil
			})
			if err != nil {
				handle.Release()
				return &fs.PathError{Op: "readdir", Path: outPath, Err: err}
			}
			for _, childName := range childNames {
				childHandle, err := handle.Lookup(ctx, childName)
				if err != nil {
					if err == unixfs_errors.ErrNotExist {
						// skip not-existing files
						continue
					}
					handle.Release()
					return &fs.PathError{Op: "lookup", Path: path.Join(srcPath, childName), Err: err}
				}
				pushStack(
					childHandle,
					path.Join(srcPath, childName),
					path.Join(outPath, childName),
				)
			}

			// continue to next file
			handle.Release()
			continue
		}

		// destination is not a directory.
		if !fi.Mode().IsRegular() {
			// skip any non-regular files.
			handle.Release()
			continue
		}

		// destination is a regular file
		of, err := bfs.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode().Perm())
		if err != nil {
			handle.Release()
			return &fs.PathError{Op: "openfile", Path: outPath, Err: err}
		}

		// copy data: based on io.Copy in stdlib
		var offset int64
		var written int64
		copyBuffer := allocCopyBuffer()
		for {
			nr, er := handle.Read(ctx, offset, copyBuffer)
			offset += nr
			if nr > 0 {
				nw, ew := of.Write(copyBuffer[0:nr])
				if nw < 0 || int(nr) < nw {
					nw = 0
					if ew == nil {
						ew = errInvalidWrite
					}
				}
				written += int64(nw)
				if ew != nil {
					err = ew
					break
				}
				if int(nr) != nw {
					err = io.ErrShortWrite
					break
				}
			}
			if er != nil {
				if er != io.EOF {
					err = er
				}
				break
			}
		}
		// release file handle
		handle.Release()
		// scrub the copy buffer
		scrub.Scrub(copyBuffer)
		if err != nil {
			return &fs.PathError{Op: "write", Path: outPath, Err: err}
		}
		// close / flush the file
		if cerr := of.Close(); err == nil && cerr != nil {
			err = cerr
		}
		if err != nil {
			return &fs.PathError{Op: "write", Path: outPath, Err: err}
		}
	}

	return nil
}
