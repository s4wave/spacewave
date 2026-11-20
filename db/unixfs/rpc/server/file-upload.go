package unixfs_rpc_server

import (
	"context"
	"io"
	"io/fs"
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
		return fileUploadFailure(unixfs_errors.ErrReadOnly), nil
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
	switch expect := start.GetExpect().(type) {
	case *unixfs_rpc.OpsMknodWithContentStart_Exclusive:
		if !expect.Exclusive {
			return nil, errors.New("file exclusive expectation must be true")
		}
	case *unixfs_rpc.OpsMknodWithContentStart_ExpectedVersion:
		if len(expect.ExpectedVersion) == 0 {
			return nil, errors.New("expected file version must be nonempty")
		}
	}
	ops, dir, err := f.uploadDirectory(start.GetCursorHandleId(), start.GetOpsHandleId())
	if err != nil {
		return fileUploadFailure(err), nil
	}

	// The backend owns publication. Its reader verifies every frame and the stream end.
	ctx := strm.Context()
	r := &fileUploadReader{stream: strm, remaining: start.GetTotalSize()}
	switch {
	case f.upload != nil:
		err = f.upload(ctx, dir, start, r)
	case start.GetExpect() != nil:
		err = errors.New("filesystem does not support conditional file publication")
	default:
		nodeType := unixfs.NewFSCursorNodeType_File()
		err = ops.MknodWithContent(ctx, name, nodeType, start.GetTotalSize(), r, fs.FileMode(start.GetPermissions()), start.GetTimestamp().AsTime())
	}

	// Report failures before invalidating the prior cursor of a published file.
	if err != nil {
		return fileUploadFailure(err), nil
	}

	// A replaced entry's prior cursor must stop returning cached file metadata.
	f.mtx.Lock()
	parent := f.handleIDToCursor[start.GetCursorHandleId()]
	if parent != nil {
		if childID := parent.children[name]; childID != 0 {
			f.releaseFSCursorLocked(childID, nil, nil)
		}
	}
	f.mtx.Unlock()
	return &unixfs_rpc.OpsMknodWithContentResponse{}, nil
}

// fileUploadFailure preserves a stale upload's current version across the RPC.
func fileUploadFailure(err error) *unixfs_rpc.OpsMknodWithContentResponse {
	var conflict *unixfs_errors.FileVersionConflict
	if errors.As(err, &conflict) {
		return &unixfs_rpc.OpsMknodWithContentResponse{
			Failure: &unixfs_rpc.OpsMknodWithContentResponse_Conflict{
				Conflict: &unixfs_rpc.UploadVersionConflict{Version: conflict.Version},
			},
		}
	}
	return &unixfs_rpc.OpsMknodWithContentResponse{
		Failure: &unixfs_rpc.OpsMknodWithContentResponse_UnixfsError{UnixfsError: unixfs_errors.NewUnixFSError(err)},
	}
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

	// Resolve the path within the serving root.
	dir, err := f.directoryPathLocked(cursor)
	return ops, dir, err
}
