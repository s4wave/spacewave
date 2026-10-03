package resource_unixfs

import (
	"context"
	"io"
	"io/fs"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

const (
	fsHandleMaxReadSize     = 64 * 1024
	uploadDataFrameMaxBytes = 64 * 1024
)

func validateUploadDataFrame(data []byte) error {
	if len(data) > uploadDataFrameMaxBytes {
		return errors.Errorf("unixfs upload data frame exceeds max size %d", uploadDataFrameMaxBytes)
	}
	return nil
}

// FSHandleResource implements FSHandleResourceService for a single FSHandle.
// Each instance wraps exactly one hydra/unixfs.FSHandle with 1:1 mapping.
type FSHandleResource struct {
	// le is the logger entry for object-backed handle construction; may be nil
	// only when no object path is bound.
	le     *logrus.Entry
	handle *unixfs.FSHandle
	mux    srpc.Mux
	bcast  *broadcast.Broadcast
	// writeMtx serializes every root republication for this resource tree.
	// world.AccessObjectState is a non-atomic read-merge-publish, so two
	// concurrent writers on one world object are a lost update: the second
	// publisher overwrites the first with a stale snapshot. Every mutating path
	// (per-op writes and batch tree uploads) and the reloadHandle swap take this
	// one lock across their full read-merge-publish, so whichever runs second
	// merges onto the other's committed root. Shared across child resources so
	// an edit on a child and an upload on its parent serialize together.
	writeMtx *sync.Mutex
	// handleMtx fences read RPCs across batch root publication and the following
	// handle reload. Readers hold RLock for the complete handle operation;
	// UploadTree holds Lock from before Commit until the replacement handle is
	// installed. Shared across child resources because one commit invalidates
	// cursors throughout the resource tree.
	handleMtx *sync.RWMutex
	ws        world.WorldState
	objKey    string
	fsType    unixfs_world.FSType
	path      []string
}

// NewFSHandleResource creates a new FSHandleResource.
//
// The handle-only form never constructs an object-backed cursor, so no logger
// entry is needed.
func NewFSHandleResource(handle *unixfs.FSHandle) *FSHandleResource {
	return newFSHandleResource(nil, handle, nil, nil, nil, nil, "", 0, nil)
}

// NewFSHandleObjectResource creates a new FSHandleResource bound to a world
// object path so batch tree uploads can target the same filesystem subtree.
func NewFSHandleObjectResource(
	le *logrus.Entry,
	handle *unixfs.FSHandle,
	bcast *broadcast.Broadcast,
	ws world.WorldState,
	objKey string,
	fsType unixfs_world.FSType,
	path []string,
) *FSHandleResource {
	return newFSHandleResource(le, handle, bcast, nil, nil, ws, objKey, fsType, path)
}

func newFSHandleResource(
	le *logrus.Entry,
	handle *unixfs.FSHandle,
	bcast *broadcast.Broadcast,
	writeMtx *sync.Mutex,
	handleMtx *sync.RWMutex,
	ws world.WorldState,
	objKey string,
	fsType unixfs_world.FSType,
	path []string,
) *FSHandleResource {
	// Initialize the notifications and publication barriers shared by child resources.
	if bcast == nil {
		bcast = &broadcast.Broadcast{}
	}
	if writeMtx == nil {
		writeMtx = &sync.Mutex{}
	}
	if handleMtx == nil {
		handleMtx = &sync.RWMutex{}
	}

	// Bind the filesystem handle and World path to the resource service.
	r := &FSHandleResource{
		le:        le,
		handle:    handle,
		bcast:     bcast,
		writeMtx:  writeMtx,
		handleMtx: handleMtx,
		ws:        ws,
		objKey:    objKey,
		fsType:    fsType,
		path:      slices.Clone(path),
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_unixfs.SRPCRegisterFSHandleResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *FSHandleResource) GetMux() srpc.Mux {
	return r.mux
}

// GetHandle returns the underlying FSHandle.
func (r *FSHandleResource) GetHandle() *unixfs.FSHandle {
	return r.handle
}

// registerChildResource registers a child FSHandle as a new resource.
func (r *FSHandleResource) registerChildResource(
	ctx context.Context,
	childHandle *unixfs.FSHandle,
	childPath []string,
) (uint32, error) {
	// Resolve the resource client that will retain the child handle.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		childHandle.Release()
		return 0, err
	}

	// Register the child service with the shared filesystem barriers.
	childResource := newFSHandleResource(
		r.le,
		childHandle,
		r.bcast,
		r.writeMtx,
		r.handleMtx,
		r.ws,
		r.objKey,
		r.fsType,
		childPath,
	)
	resourceID, err := client.AddResourceValue(childResource.GetMux(), childResource, func() {
		childHandle.Release()
	})
	if err != nil {
		childHandle.Release()
		return 0, err
	}

	return resourceID, nil
}

// joinHandlePath joins relPath onto the current handle path.
func (r *FSHandleResource) joinHandlePath(relPath string) []string {
	// Preserve the current handle path for a lookup of the current directory.
	if relPath == "" || relPath == "." {
		return slices.Clone(r.path)
	}

	// Resolve relative path components against the current handle path.
	next := slices.Clone(r.path)
	parts, _ := unixfs.SplitPath(relPath)
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(next) != 0 {
				next = next[:len(next)-1]
			}
			continue
		}
		next = append(next, part)
	}
	return next
}

// mutate serializes a filesystem mutation and its change broadcast against
// every other writer (per-op writes and batch tree uploads) and against the
// reloadHandle swap for this world object. It holds writeMtx across the whole
// mutation so the read-merge-publish in world.AccessObjectState cannot interleave
// with another writer and lose an update. fn performs the world mutation.
func (r *FSHandleResource) mutate(fn func() error) error {
	// Hold the resource tree write barrier across the filesystem mutation.
	r.writeMtx.Lock()
	defer r.writeMtx.Unlock()
	if err := fn(); err != nil {
		return err
	}

	// Notify directory watchers after the filesystem mutation succeeds.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return nil
}

// borrowHandle returns an independent clone while retaining the resource-tree
// read barrier. The caller invokes the returned release function after the
// complete handle operation so UploadTree cannot publish a new root and
// invalidate the borrowed cursor before it installs the replacement handle.
func (r *FSHandleResource) borrowHandle(
	ctx context.Context,
) (*unixfs.FSHandle, func(), error) {
	r.handleMtx.RLock()
	handle, err := r.handle.Clone(ctx)
	if err != nil {
		r.handleMtx.RUnlock()
		return nil, nil, err
	}
	return handle, func() {
		handle.Release()
		r.handleMtx.RUnlock()
	}, nil
}

// reloadHandleLocked reloads the current handle from world state at r.path.
// The caller holds writeMtx and handleMtx from before publishing the new root,
// so neither another writer nor a read operation can observe the invalidated
// handle generation before this replacement is installed.
func (r *FSHandleResource) reloadHandleLocked(ctx context.Context) error {
	// Keep detached filesystem handles on their current cursor.
	if r.ws == nil || r.objKey == "" {
		return nil
	}

	// Open a replacement handle on the current World root.
	nextHandle, err := r.newObjectFSHandle(ctx)
	if err != nil {
		return err
	}

	// Replace the resource handle and release its previous cursor.
	prev := r.handle
	r.handle = nextHandle
	prev.Release()
	return nil
}

func (r *FSHandleResource) newObjectFSHandle(ctx context.Context) (*unixfs.FSHandle, error) {
	// Require the World object that backs the filesystem handle.
	if r.ws == nil || r.objKey == "" {
		return nil, errors.New("object-backed filesystem handle unavailable")
	}

	// Open a filesystem cursor with the World state access mode.
	var fsCursor *unixfs_world.FSCursor
	if r.ws.GetReadOnly() {
		fsCursor = unixfs_world.NewFSCursor(r.le, r.ws, r.objKey, r.fsType, nil, true)
	} else {
		// note: NewFSCursorWithWriter returns (cursor, writer) and cannot fail;
		// the cursor retains the writer internally, so the second return value
		// carries nothing new here.
		fsCursor, _ = unixfs_world.NewFSCursorWithWriter(ctx, r.le, r.ws, r.objKey, r.fsType, "")
	}

	// Position the replacement handle at the resource path.
	var nextHandle *unixfs.FSHandle
	var err error
	if len(r.path) == 0 {
		nextHandle, err = unixfs.NewFSHandle(fsCursor)
		if err != nil {
			return nil, err
		}
	} else {
		nextHandle, err = unixfs.NewFSHandleWithPrefix(
			ctx,
			fsCursor,
			r.path,
			false,
			time.Now(),
		)
		if err != nil {
			return nil, err
		}
	}

	return nextHandle, nil
}

// borrowDestParentHandle resolves and borrows a destination parent resource.
func borrowDestParentHandle(
	ctx context.Context,
	destParentResourceID uint32,
) (*unixfs.FSHandle, func(), error) {
	// Resolve the client containing the destination parent resource.
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Retrieve the registered destination parent resource.
	value, err := client.GetResourceValue(destParentResourceID)
	if err != nil {
		return nil, nil, err
	}

	// Require a filesystem handle service for the destination parent.
	destParentResource, ok := value.(*FSHandleResource)
	if !ok {
		return nil, nil, errors.New("destination parent is not a unixfs handle resource")
	}

	return destParentResource.borrowHandle(ctx)
}

// getFileInfo gets FileInfo from a handle.
func getFileInfo(ctx context.Context, handle *unixfs.FSHandle) (*s4wave_unixfs.FileInfo, error) {
	info, err := handle.GetFileInfo(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    uint32(info.Mode()),
		ModTime: info.ModTime().Unix(),
		IsDir:   info.IsDir(),
	}, nil
}

// Lookup looks up a child by name and returns a new handle resource.
func (r *FSHandleResource) Lookup(ctx context.Context, req *s4wave_unixfs.HandleLookupRequest) (*s4wave_unixfs.HandleLookupResponse, error) {
	// Borrow the parent handle for the requested child lookup.
	name := req.GetName()
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Open the requested child handle for resource registration.
	childHandle, err := handle.Lookup(ctx, name)
	if err != nil {
		return nil, err
	}

	// Get file info for the child
	info, err := getFileInfo(ctx, childHandle)
	if err != nil {
		childHandle.Release()
		return nil, err
	}

	// Register the child handle as a resource
	resourceID, err := r.registerChildResource(
		ctx,
		childHandle,
		r.joinHandlePath(name),
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleLookupResponse{
		ResourceId: resourceID,
		Info:       info,
	}, nil
}

// LookupPath looks up a path and returns a new handle resource.
func (r *FSHandleResource) LookupPath(ctx context.Context, req *s4wave_unixfs.HandleLookupPathRequest) (*s4wave_unixfs.HandleLookupPathResponse, error) {
	// Borrow the starting handle for the requested filesystem path lookup.
	path := req.GetPath()
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Traverse the requested path and release any partial handle on failure.
	childHandle, traversedPath, err := handle.LookupPath(ctx, path)
	if err != nil {
		if childHandle != nil {
			childHandle.Release()
		}
		return nil, err
	}

	// Get file info
	info, err := getFileInfo(ctx, childHandle)
	if err != nil {
		childHandle.Release()
		return nil, err
	}

	// Register the child handle as a resource
	resourceID, err := r.registerChildResource(
		ctx,
		childHandle,
		r.joinHandlePath(path),
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleLookupPathResponse{
		ResourceId:    resourceID,
		TraversedPath: traversedPath,
		Info:          info,
	}, nil
}

// ReadAt reads bytes at the given offset.
func (r *FSHandleResource) ReadAt(ctx context.Context, req *s4wave_unixfs.HandleReadAtRequest) (*s4wave_unixfs.HandleReadAtResponse, error) {
	// Borrow the file handle for the requested bounded read range.
	offset := req.GetOffset()
	length := req.GetLength()
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// length<=0 requests the remaining file from offset, but only when that
	// fits in one bounded resource response. Larger reads use ReadStream.
	if length <= 0 {
		size, err := handle.GetSize(ctx)
		if err != nil {
			return nil, err
		}
		if size > math.MaxInt64 {
			return nil, errors.Errorf("handle size exceeds int64 range: %d", size)
		}
		length = int64(size) - offset //nolint:gosec // the preceding MaxInt64 check protects the signed range calculation.
		if length <= 0 {
			return &s4wave_unixfs.HandleReadAtResponse{
				Eof: true,
			}, nil
		}
		if length > fsHandleMaxReadSize {
			return nil, errors.Errorf(
				"unixfs ReadAt length=0 would exceed max response size %d; use ReadStream",
				fsHandleMaxReadSize,
			)
		}
	}

	// Bound resource read responses to the same chunk scale used by UploadTree.
	// TinyGo browser workers can stall or trap when a single SRPC response tries
	// to carry multi-hundred-KiB UnixFS payloads through the wasm bridge.
	if length > fsHandleMaxReadSize {
		length = fsHandleMaxReadSize
	}

	// Read the bounded file range into the response buffer.
	data := make([]byte, length)
	bytesRead, err := handle.ReadAt(ctx, offset, data)

	// Handle io.EOF specially: ReadAt may return both data AND io.EOF
	// when reaching the end of file. This is valid Go io.ReaderAt semantics.
	eof := false
	if err != nil {
		if err == io.EOF {
			eof = true
		} else {
			return nil, err
		}
	}

	return &s4wave_unixfs.HandleReadAtResponse{
		Data:      data[:bytesRead],
		BytesRead: bytesRead,
		Eof:       eof,
	}, nil
}

// ReadStream streams a byte range in file order. The reads run in order on
// one handle, so the file's chunk read-ahead overlaps block fetches with the
// stream instead of each call seeking into the file afresh.
func (r *FSHandleResource) ReadStream(
	req *s4wave_unixfs.HandleReadStreamRequest,
	strm s4wave_unixfs.SRPCFSHandleResourceService_ReadStreamStream,
) error {
	// Require a nonnegative file offset for the streamed range.
	ctx := strm.Context()
	offset, length := req.GetOffset(), req.GetLength()
	if offset < 0 {
		return errors.Errorf("negative read offset: %d", offset)
	}

	// Borrow one file handle for the complete read stream.
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return err
	}
	defer releaseHandle()

	// A length <= 0 reads to the end of the file. Send encodes each frame
	// before returning, so one buffer serves every frame.
	bounded := length > 0
	buf := make([]byte, fsHandleMaxReadSize)
	for !bounded || length > 0 {
		frame := buf
		if bounded && length < int64(len(frame)) {
			frame = frame[:length]
		}
		n, readErr := handle.ReadAt(ctx, offset, frame)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if n > 0 {
			if err := strm.Send(&s4wave_unixfs.HandleReadStreamResponse{Data: frame[:n]}); err != nil {
				return err
			}
			offset += n
			length -= n
		}
		if readErr == io.EOF || n == 0 {
			return nil
		}
	}
	return nil
}

// WriteAt writes bytes at the given offset.
func (r *FSHandleResource) WriteAt(ctx context.Context, req *s4wave_unixfs.HandleWriteAtRequest) (*s4wave_unixfs.HandleWriteAtResponse, error) {
	offset := req.GetOffset()
	data := req.GetData()

	if err := r.mutate(func() error {
		return r.handle.WriteAt(ctx, offset, data, time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleWriteAtResponse{
		BytesWritten: int64(len(data)),
	}, nil
}

// Sync commits buffered writes in the filesystem tree and returns the first
// write that failed to commit.
func (r *FSHandleResource) Sync(ctx context.Context, req *s4wave_unixfs.HandleSyncRequest) (*s4wave_unixfs.HandleSyncResponse, error) {
	if err := r.mutate(func() error {
		return r.handle.Sync(ctx)
	}); err != nil {
		return nil, err
	}
	return &s4wave_unixfs.HandleSyncResponse{}, nil
}

// Truncate truncates the file to the given size.
func (r *FSHandleResource) Truncate(ctx context.Context, req *s4wave_unixfs.HandleTruncateRequest) (*s4wave_unixfs.HandleTruncateResponse, error) {
	size := req.GetSize()

	if err := r.mutate(func() error {
		return r.handle.Truncate(ctx, size, time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleTruncateResponse{}, nil
}

// GetSize returns the current size of the file.
func (r *FSHandleResource) GetSize(ctx context.Context, req *s4wave_unixfs.HandleGetSizeRequest) (*s4wave_unixfs.HandleGetSizeResponse, error) {
	// Borrow the file handle while the resource tree read barrier is held.
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Read the current file size from the borrowed handle.
	size, err := handle.GetSize(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleGetSizeResponse{
		Size: size,
	}, nil
}

// GetFileInfo returns file metadata for the handle's location.
func (r *FSHandleResource) GetFileInfo(ctx context.Context, req *s4wave_unixfs.HandleGetFileInfoRequest) (*s4wave_unixfs.HandleGetFileInfoResponse, error) {
	// Borrow the filesystem handle while the resource tree read barrier is held.
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Read the metadata at the borrowed handle location.
	info, err := getFileInfo(ctx, handle)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleGetFileInfoResponse{
		Info: info,
	}, nil
}

// GetNodeType returns the node type (file, directory, symlink).
func (r *FSHandleResource) GetNodeType(ctx context.Context, req *s4wave_unixfs.HandleGetNodeTypeRequest) (*s4wave_unixfs.HandleGetNodeTypeResponse, error) {
	// Borrow the filesystem handle while the resource tree read barrier is held.
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Read the node type at the borrowed handle location.
	nodeType, err := handle.GetNodeType(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleGetNodeTypeResponse{
		NodeType: &s4wave_unixfs.NodeType{
			IsFile:    nodeType.GetIsFile(),
			IsDir:     nodeType.GetIsDirectory(),
			IsSymlink: nodeType.GetIsSymlink(),
		},
	}, nil
}

// Readdir reads directory entries (streaming for large directories).
func (r *FSHandleResource) Readdir(req *s4wave_unixfs.HandleReaddirRequest, strm s4wave_unixfs.SRPCFSHandleResourceService_ReaddirStream) error {
	// Borrow one directory handle for the requested listing offset.
	ctx := strm.Context()
	skip := req.GetSkip()
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return err
	}
	defer releaseHandle()

	// Stream directory entries with the metadata available from each child.
	err = handle.ReaddirAll(ctx, skip, func(ent unixfs.FSCursorDirent) error {
		entry := dirEntryFromCursor(ent)

		// Try to get additional info (size, mtime, mode)
		childHandle, lookupErr := handle.Lookup(ctx, ent.GetName())
		if lookupErr == nil && childHandle != nil {
			defer childHandle.Release()

			if info, infoErr := childHandle.GetFileInfo(ctx); infoErr == nil {
				if info.Size() >= 0 {
					entry.Size = uint64(info.Size()) //nolint:gosec // file-info sizes are non-negative at this boundary.
				}
				entry.ModTime = info.ModTime().Unix()
				entry.Mode = uint32(info.Mode())
			}
		}

		return strm.Send(&s4wave_unixfs.HandleReaddirResponse{
			Entry: entry,
		})
	})
	if err != nil {
		return err
	}

	// Send final message indicating completion
	return strm.Send(&s4wave_unixfs.HandleReaddirResponse{
		Done: true,
	})
}

// Mknod creates a new file or directory.
func (r *FSHandleResource) Mknod(ctx context.Context, req *s4wave_unixfs.HandleMknodRequest) (*s4wave_unixfs.HandleMknodResponse, error) {
	// Read the requested node names, type, permissions, and existence policy.
	names := req.GetNames()
	nodeType := req.GetNodeType()
	mode := req.GetMode()
	checkExist := req.GetCheckExist()

	// Choose the filesystem node type for the creation request.
	var fsNodeType unixfs.FSCursorNodeType
	switch nodeType {
	case s4wave_unixfs.MknodType_MKNOD_TYPE_FILE:
		fsNodeType = unixfs.NewFSCursorNodeType_File()
	case s4wave_unixfs.MknodType_MKNOD_TYPE_DIR:
		fsNodeType = unixfs.NewFSCursorNodeType_Dir()
	default:
		fsNodeType = unixfs.NewFSCursorNodeType_File()
	}

	// Supply the default permissions for the chosen node type.
	if mode == 0 {
		mode = uint32(unixfs.DefaultPermissions(fsNodeType))
	}

	// Create the requested nodes under the resource tree write barrier.
	if err := r.mutate(func() error {
		return r.handle.Mknod(ctx, checkExist, names, fsNodeType, fs.FileMode(mode), time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleMknodResponse{}, nil
}

// Remove removes files or directories by name.
func (r *FSHandleResource) Remove(ctx context.Context, req *s4wave_unixfs.HandleRemoveRequest) (*s4wave_unixfs.HandleRemoveResponse, error) {
	names := req.GetNames()

	if err := r.mutate(func() error {
		return r.handle.Remove(ctx, names, time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleRemoveResponse{}, nil
}

// MkdirAll creates a directory and all parent directories.
func (r *FSHandleResource) MkdirAll(ctx context.Context, req *s4wave_unixfs.HandleMkdirAllRequest) (*s4wave_unixfs.HandleMkdirAllResponse, error) {
	// Read the directory path and permissions requested for creation.
	pathParts := req.GetPathParts()
	mode := req.GetMode()

	// Supply the directory permissions when the request omits them.
	if mode == 0 {
		mode = 0o755
	}

	// Create the directory path under the resource tree write barrier.
	if err := r.mutate(func() error {
		return r.handle.MkdirAll(ctx, pathParts, fs.FileMode(mode), time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleMkdirAllResponse{}, nil
}

// Rename renames an entry within a directory or moves it to a new location.
// When source_name is set, this handle is the parent directory containing the entry.
// When source_name is empty, returns an error (legacy path was broken).
func (r *FSHandleResource) Rename(ctx context.Context, req *s4wave_unixfs.HandleRenameRequest) (*s4wave_unixfs.HandleRenameResponse, error) {
	// Read the source and destination locations requested for the rename.
	sourceName := req.GetSourceName()
	destName := req.GetDestName()
	destParentResourceID := req.GetDestParentResourceId()

	// Require a source entry name within this directory handle.
	if sourceName == "" {
		return nil, errors.New("source_name is required for rename")
	}

	// Move the entry under the resource tree write barrier.
	if err := r.mutate(func() error {
		// Open the source entry handle for the move.
		sourceHandle, err := r.handle.Lookup(ctx, sourceName)
		if err != nil {
			return err
		}
		defer sourceHandle.Release()

		// Borrow the destination parent handle for the move.
		var destParentHandle *unixfs.FSHandle
		var releaseDestParent func()
		if destParentResourceID == 0 {
			destParentHandle, err = r.handle.Clone(ctx)
			if err == nil {
				releaseDestParent = destParentHandle.Release
			}
		} else {
			destParentHandle, releaseDestParent, err = borrowDestParentHandle(ctx, destParentResourceID)
		}
		if err != nil {
			return err
		}
		defer releaseDestParent()

		return sourceHandle.Rename(ctx, destParentHandle, destName, time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleRenameResponse{}, nil
}

// UploadFile uploads a file via client-streaming.
func (r *FSHandleResource) UploadFile(strm s4wave_unixfs.SRPCFSHandleResourceService_UploadFileStream) (*s4wave_unixfs.HandleUploadFileResponse, error) {
	// Read and validate the file metadata in the first upload message.
	ctx := strm.Context()
	first, err := strm.Recv()
	if err != nil {
		return nil, err
	}
	name := first.GetName()
	totalSize := first.GetTotalSize()
	mode := first.GetMode()
	if name == "" {
		return nil, errors.New("name is required in first message")
	}
	if totalSize <= 0 {
		return nil, errors.New("total_size must be positive")
	}

	// Connect the upload stream to filesystem ingestion through a pipe.
	pr, pw := io.Pipe()

	// Start the file ingestion and retain its completion result. Closing the
	// reader when ingestion returns unblocks a write it will never consume.
	var uploadErr error
	done := make(chan struct{})
	go func() {
		// Choose the file permissions and signal completion when ingestion ends.
		defer close(done)
		nodeType := unixfs.NewFSCursorNodeType_File()
		if mode == 0 {
			mode = uint32(unixfs.DefaultPermissions(nodeType))
		}

		// Ingest and publish the uploaded file under the resource tree write barrier.
		// MknodWithContent fuses blob ingest and the root commit, so writeMtx
		// is held across the streamed read. The legacy single-file path trades
		// upload-time write parallelism for the same lost-update safety the
		// batch UploadTree path already has.
		uploadErr = r.mutate(func() error {
			return r.handle.MknodWithContent(
				ctx, name, nodeType, totalSize, pr,
				fs.FileMode(mode), time.Now(),
			)
		})
		pr.CloseWithError(uploadErr)
	}()

	// abort ends the ingestion and returns its error, or err when it succeeded.
	abort := func(err error) error {
		pw.CloseWithError(err)
		<-done
		if uploadErr != nil {
			return uploadErr
		}
		return err
	}

	// Feed each data frame, starting with the first message, to the file
	// ingestion pipe until the client ends the stream.
	var bytesWritten int64
	msg := first
	for {
		if data := msg.GetData(); len(data) > 0 {
			if err := validateUploadDataFrame(data); err != nil {
				return nil, abort(err)
			}
			recordUploadMetric(ctx, UploadMetric{Stage: "receive-data", Bytes: len(data)})
			if _, err := pw.Write(data); err != nil {
				return nil, abort(errors.New("upload data exceeds total_size"))
			}
			bytesWritten += int64(len(data))
		}

		// Read the next upload message.
		msg, err = strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, abort(err)
		}
	}

	// Finish the ingestion pipe and report the filesystem commit result.
	pw.Close()
	<-done
	if uploadErr != nil {
		return nil, uploadErr
	}

	return &s4wave_unixfs.HandleUploadFileResponse{
		BytesWritten: bytesWritten,
	}, nil
}

// Readlink reads the target of a symbolic link at this handle.
func (r *FSHandleResource) Readlink(ctx context.Context, req *s4wave_unixfs.HandleReadlinkRequest) (*s4wave_unixfs.HandleReadlinkResponse, error) {
	// Borrow the symbolic link handle while the resource tree read barrier is held.
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Read the symbolic link target from the borrowed handle.
	parts, isAbsolute, err := handle.Readlink(ctx, "")
	if err != nil {
		return nil, err
	}
	return &s4wave_unixfs.HandleReadlinkResponse{
		Target: unixfs.JoinPath(parts, isAbsolute),
	}, nil
}

// Clone creates a copy of this handle pointing to the same location.
func (r *FSHandleResource) Clone(ctx context.Context, req *s4wave_unixfs.HandleCloneRequest) (*s4wave_unixfs.HandleCloneResponse, error) {
	// Borrow the filesystem handle while the resource tree read barrier is held.
	handle, releaseHandle, err := r.borrowHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseHandle()

	// Clone the handle at its current filesystem location.
	clonedHandle, err := handle.Clone(ctx)
	if err != nil {
		return nil, err
	}

	// Register the cloned handle as a child resource.
	resourceID, err := r.registerChildResource(ctx, clonedHandle, r.path)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleCloneResponse{
		ResourceId: resourceID,
	}, nil
}

func dirEntryFromCursor(ent unixfs.FSCursorDirent) *s4wave_unixfs.DirEntry {
	return &s4wave_unixfs.DirEntry{
		Name:      ent.GetName(),
		IsDir:     ent.GetIsDirectory(),
		IsSymlink: ent.GetIsSymlink(),
	}
}

// readWatchEntries reads all directory entries for WatchReaddir.
func readWatchEntries(ctx context.Context, handle *unixfs.FSHandle) ([]*s4wave_unixfs.DirEntry, error) {
	var entries []*s4wave_unixfs.DirEntry
	err := handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entries = append(entries, dirEntryFromCursor(ent))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// WatchReaddir watches directory entries and streams the full listing on each change.
func (r *FSHandleResource) WatchReaddir(req *s4wave_unixfs.HandleWatchReaddirRequest, strm s4wave_unixfs.SRPCFSHandleResourceService_WatchReaddirStream) error {
	// Open an independent watch handle and retain its last emitted revision.
	ctx := strm.Context()
	var prev *s4wave_unixfs.HandleWatchReaddirResponse
	var lastObjRev uint64
	var haveObjRev bool
	var watchHandle *unixfs.FSHandle
	if r.ws != nil && r.objKey != "" {
		var err error
		watchHandle, err = r.newObjectFSHandle(ctx)
		if err != nil {
			return err
		}
		defer func() { watchHandle.Release() }()
	} else {
		var err error
		r.handleMtx.RLock()
		watchHandle, err = r.handle.Clone(ctx)
		r.handleMtx.RUnlock()
		if err != nil {
			return err
		}
		defer watchHandle.Release()
	}

	// Publish each changed directory listing until the watch ends.
	for {
		// Capture the local change notification before reading the World revision.
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		objState, objRev, err := r.watchObjectRev(ctx)
		if err != nil {
			world.ReleaseObjectState(objState)
			return err
		}
		if haveObjRev && objState != nil && objRev != lastObjRev {
			nextWatchHandle, err := r.newObjectFSHandle(ctx)
			if err != nil {
				world.ReleaseObjectState(objState)
				return err
			}
			watchHandle.Release()
			watchHandle = nextWatchHandle
		}

		// Read the directory listing while the resource tree read barrier is held.
		r.handleMtx.RLock()
		entries, err := readWatchEntries(ctx, watchHandle)
		r.handleMtx.RUnlock()
		if err != nil {
			world.ReleaseObjectState(objState)
			return err
		}

		// Emit the directory listing only when its entries change.
		resp := &s4wave_unixfs.HandleWatchReaddirResponse{
			Entries: entries,
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				world.ReleaseObjectState(objState)
				return err
			}
			prev = resp
		}
		if objState != nil {
			lastObjRev = objRev
			haveObjRev = true
		}

		// Wait for a local notification or a change to the World object revision.
		err = r.waitReaddirChange(ctx, ch, objState, objRev)
		world.ReleaseObjectState(objState)
		if err != nil {
			return err
		}
	}
}

func (r *FSHandleResource) watchObjectRev(ctx context.Context) (world.ObjectState, uint64, error) {
	// Keep detached directory watches independent of World object revisions.
	if r.ws == nil || r.objKey == "" {
		return nil, 0, nil
	}

	// Open the backing World object and require it to remain present.
	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, 0, err
	}
	if !found {
		world.ReleaseObjectState(objState)
		return nil, 0, world.ErrObjectNotFound
	}

	// Read the root revision used to wait for the next directory change.
	_, rev, err := objState.GetRootRef(ctx)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, 0, err
	}
	return objState, rev, nil
}

func (r *FSHandleResource) waitReaddirChange(
	ctx context.Context,
	localChange <-chan struct{},
	objState world.ObjectState,
	objRev uint64,
) error {
	// Wait for a local change when the directory has no backing World object.
	if objState == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-localChange:
			return nil
		}
	}

	// Start a cancellable wait for the next backing World object revision.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	objChange := make(chan error, 1)
	go func() {
		_, err := objState.WaitRev(waitCtx, objRev+1, false)
		objChange <- err
	}()

	// Finish the directory wait on cancellation or either change notification.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-localChange:
		return nil
	case err := <-objChange:
		return err
	}
}

// _ is a type assertion
var _ s4wave_unixfs.SRPCFSHandleResourceServiceServer = (*FSHandleResource)(nil)
