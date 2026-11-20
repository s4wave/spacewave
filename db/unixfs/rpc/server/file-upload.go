package unixfs_rpc_server

import (
	"context"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// FileUpload publishes one regular file within a directory of the serving root.
// The directory path comes from the registered cursor, relative to the root.
type FileUpload func(context.Context, string, *unixfs_rpc.OpsMknodWithContentStart, io.Reader) error

// OpsMknodWithContent streams one complete file through the configured publisher.
func (f *FSCursorService) OpsMknodWithContent(strm unixfs_rpc.SRPCFSCursorService_OpsMknodWithContentStream) (*unixfs_rpc.OpsMknodWithContentResponse, error) {
	// Reject writes to a read-only root before receiving the file.
	if f.readOnly {
		return &unixfs_rpc.OpsMknodWithContentResponse{UnixfsError: unixfs_errors.NewUnixFSError(unixfs_errors.ErrReadOnly)}, nil
	}

	// Validate the start and select the directory while its handles are live.
	first, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	start := first.GetStart()
	name := start.GetName()
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || start.GetTotalSize() < 0 {
		return nil, errors.New("file upload requires one entry name and a non-negative size")
	}
	ops, dir, err := f.uploadDirectory(start.GetCursorHandleId(), start.GetOpsHandleId())
	if err != nil {
		return &unixfs_rpc.OpsMknodWithContentResponse{UnixfsError: unixfs_errors.NewUnixFSError(err)}, nil
	}

	// The backend owns publication. Its reader verifies every frame and the stream end.
	ctx := strm.Context()
	r := &fileUploadReader{stream: strm, remaining: start.GetTotalSize()}
	switch {
	case f.upload != nil:
		err = f.upload(ctx, dir, start, r)
	case start.GetExclusive():
		err = errors.New("filesystem does not support exclusive file publication")
	default:
		nodeType := unixfs.NewFSCursorNodeType_File()
		err = ops.MknodWithContent(ctx, name, nodeType, start.GetTotalSize(), r, fs.FileMode(start.GetPermissions()), start.GetTimestamp().AsTime())
	}

	// A replaced entry's prior cursor must stop returning cached file metadata.
	if err == nil {
		f.mtx.Lock()
		parent := f.handleIDToCursor[start.GetCursorHandleId()]
		if parent != nil {
			if childID := parent.children[name]; childID != 0 {
				f.releaseFSCursorLocked(childID, nil, nil)
			}
		}
		f.mtx.Unlock()
	}
	return &unixfs_rpc.OpsMknodWithContentResponse{UnixfsError: unixfs_errors.NewUnixFSError(err)}, nil
}

// uploadDirectory validates the operations handle and reconstructs the registered directory's path.
func (f *FSCursorService) uploadDirectory(cursorID, opsID uint64) (unixfs.FSCursorOps, string, error) {
	// Read only the registered cursor and its ancestors while the service lock is held.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	cursor := f.lookupFSCursorLocked(cursorID)
	if cursor == nil || cursor.opsHandleID == nil || *cursor.opsHandleID != opsID {
		return nil, "", unixfs_errors.ErrReleased
	}
	ops, err := f.lookupCursorOpsLocked(opsID)
	if err != nil {
		return nil, "", err
	}
	if !ops.GetIsDirectory() {
		return nil, "", unixfs_errors.ErrNotDirectory
	}

	// Resolve the host path from the root outwards without re-reading filesystem nodes.
	var parts []string
	for cursor.parent != 0 {
		if cursor.name != "" {
			parts = append(parts, cursor.name)
		}
		cursor = f.handleIDToCursor[cursor.parent]
		if cursor == nil {
			return nil, "", unixfs_errors.ErrReleased
		}
	}
	slices.Reverse(parts)
	return ops, path.Join(".", path.Join(parts...)), nil
}
