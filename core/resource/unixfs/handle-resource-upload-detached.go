package resource_unixfs

import (
	"context"
	"io"
	"io/fs"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

// uploadDetachedTree publishes each streamed entry as it is completed.
func (r *FSHandleResource) uploadDetachedTree(strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeStream) (*s4wave_unixfs.HandleUploadTreeResponse, error) {
	// Retain a filesystem handle for the stream without holding the World barriers.
	ctx := strm.Context()
	root, err := r.handle.Clone(ctx)
	if err != nil {
		return nil, err
	}
	defer root.Release()

	// Read one entry at a time and notify watchers after each publication.
	var state uploadTreeState
	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			return &s4wave_unixfs.HandleUploadTreeResponse{
				Result: &s4wave_unixfs.HandleUploadTreeResponse_Written{Written: &state.resp},
			}, nil
		}
		if err != nil {
			return nil, err
		}

		entry, err := uploadDetachedEntry(ctx, root, strm, &state, msg)
		if errors.Is(err, unixfs_errors.ErrExist) {
			return &s4wave_unixfs.HandleUploadTreeResponse{
				Result: &s4wave_unixfs.HandleUploadTreeResponse_Conflict{
					Conflict: &s4wave_unixfs.HandleUploadTreeConflict{Path: entry},
				},
			}, nil
		}
		if err != nil {
			return nil, err
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	}
}

// uploadDetachedEntry creates a directory or streams one file at its relative path.
func uploadDetachedEntry(ctx context.Context, root *unixfs.FSHandle, strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeStream, state *uploadTreeState, msg *s4wave_unixfs.HandleUploadTreeRequest) (string, error) {
	// Validate the next entry and create any parent directories it requires.
	dir, file := msg.GetDirectory(), msg.GetFileStart()
	entry := dir.GetPath()
	if file != nil {
		entry = file.GetPath()
	}
	parts, err := parseUploadTreePath(entry)
	if err != nil {
		return entry, err
	}

	// Validate file expectations before the request can create parent directories.
	if file != nil {
		if file.GetTotalSize() < 0 {
			return entry, errors.New("file total_size must be non-negative")
		}
		switch expect := file.GetExpect().(type) {
		case *s4wave_unixfs.HandleUploadTreeFileStart_Version:
			return entry, unixfs_errors.ErrExist
		case *s4wave_unixfs.HandleUploadTreeFileStart_Overwrite:
			if !expect.Overwrite {
				return entry, errors.New("file overwrite must be true when set")
			}
		default:
			ctx = unixfs.WithExclusiveFileCreate(ctx)
		}
	}

	// Resolve the target parent with the same path primitive used for directory edits.
	ts := time.Now()
	parent, err := root.MkdirAllLookup(ctx, parts[:len(parts)-1], 0o755, ts)
	if err != nil {
		return entry, err
	}
	defer parent.Release()
	name := parts[len(parts)-1]

	// A folder either reserves its name or merges into a directory already there.
	if dir != nil {
		mode := fs.FileMode(dir.GetMode()).Perm()
		if mode == 0 {
			mode = 0o755
		}
		var err error
		if dir.GetExpectMissing() {
			err = parent.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_Dir(), mode, ts)
		} else {
			err = parent.MkdirAll(ctx, []string{name}, mode, ts)
		}
		if err == nil {
			state.resp.DirectoriesWritten++
		}
		return entry, err
	}

	// The stream reader holds a single data frame and stops at this file's boundary.
	size := file.GetTotalSize()
	rdr := &uploadTreeFile{strm: strm, state: state, name: name, remaining: size}
	if err := parent.MknodWithContent(ctx, name, unixfs.NewFSCursorNodeType_File(), size, rdr, fs.FileMode(file.GetMode()), ts); err != nil {
		return entry, err
	}
	if rdr.remaining != 0 || len(rdr.buf) != 0 {
		return entry, io.ErrUnexpectedEOF
	}
	state.resp.FilesWritten++
	return entry, nil
}
