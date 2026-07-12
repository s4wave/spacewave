package resource_unixfs

import (
	"context"
	"io"
	"io/fs"
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
	// handleMtx guards the handle pointer against the reloadHandle swap so
	// unsynchronized read RPCs never observe a torn pointer or use the handle
	// after Release frees its cursor. Readers clone under RLock; reload swaps
	// under Lock. Per-instance: each resource owns its own handle.
	handleMtx sync.RWMutex
	ws        world.WorldState
	objKey    string
	fsType    unixfs_world.FSType
	path      []string
}

// NewFSHandleResource creates a new FSHandleResource.
func NewFSHandleResource(handle *unixfs.FSHandle) *FSHandleResource {
	return newFSHandleResource(handle, nil, nil, nil, "", 0, nil)
}

// NewFSHandleObjectResource creates a new FSHandleResource bound to a world
// object path so batch tree uploads can target the same filesystem subtree.
func NewFSHandleObjectResource(
	handle *unixfs.FSHandle,
	bcast *broadcast.Broadcast,
	ws world.WorldState,
	objKey string,
	fsType unixfs_world.FSType,
	path []string,
) *FSHandleResource {
	return newFSHandleResource(handle, bcast, nil, ws, objKey, fsType, path)
}

func newFSHandleResource(
	handle *unixfs.FSHandle,
	bcast *broadcast.Broadcast,
	writeMtx *sync.Mutex,
	ws world.WorldState,
	objKey string,
	fsType unixfs_world.FSType,
	path []string,
) *FSHandleResource {
	if bcast == nil {
		bcast = &broadcast.Broadcast{}
	}
	if writeMtx == nil {
		writeMtx = &sync.Mutex{}
	}
	r := &FSHandleResource{
		handle:   handle,
		bcast:    bcast,
		writeMtx: writeMtx,
		ws:       ws,
		objKey:   objKey,
		fsType:   fsType,
		path:     append([]string(nil), path...),
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
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		childHandle.Release()
		return 0, err
	}

	childResource := newFSHandleResource(
		childHandle,
		r.bcast,
		r.writeMtx,
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
	if relPath == "" || relPath == "." {
		return append([]string(nil), r.path...)
	}
	next := append([]string(nil), r.path...)
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
	r.writeMtx.Lock()
	defer r.writeMtx.Unlock()
	if err := fn(); err != nil {
		return err
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return nil
}

// cloneHandle returns an independent clone of the current handle, snapshotting
// the handle pointer under handleMtx so a concurrent reloadHandle swap can
// neither tear the read nor free the handle while the caller reads it. Clone
// adds its own reference under the inode lock, so releasing the resource's
// handle later leaves the returned clone valid. Callers Release the clone.
func (r *FSHandleResource) cloneHandle(ctx context.Context) (*unixfs.FSHandle, error) {
	r.handleMtx.RLock()
	defer r.handleMtx.RUnlock()
	return r.handle.Clone(ctx)
}

// reloadHandle reloads the current handle from world state at r.path. Callers
// hold writeMtx (reload runs only after a committed root change), so this never
// races another writer; handleMtx additionally fences unsynchronized readers
// during the pointer swap. The previous handle is released after the swap so any
// reader that already cloned it keeps its own reference until done.
func (r *FSHandleResource) reloadHandle(ctx context.Context) error {
	if r.ws == nil || r.objKey == "" {
		return nil
	}
	nextHandle, err := r.newObjectFSHandle(ctx)
	if err != nil {
		return err
	}
	r.handleMtx.Lock()
	prev := r.handle
	r.handle = nextHandle
	r.handleMtx.Unlock()
	prev.Release()
	return nil
}

func (r *FSHandleResource) newObjectFSHandle(ctx context.Context) (*unixfs.FSHandle, error) {
	if r.ws == nil || r.objKey == "" {
		return nil, errors.New("object-backed filesystem handle unavailable")
	}

	le := logrus.NewEntry(logrus.StandardLogger())
	var fsCursor *unixfs_world.FSCursor
	if r.ws.GetReadOnly() {
		fsCursor = unixfs_world.NewFSCursor(le, r.ws, r.objKey, r.fsType, nil, true)
	} else {
		fsCursor, _ = unixfs_world.NewFSCursorWithWriter(ctx, le, r.ws, r.objKey, r.fsType, "")
	}
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

// resolveDestParentHandle resolves a destination parent resource ID to a FSHandle.
func resolveDestParentHandle(
	ctx context.Context,
	destParentResourceID uint32,
) (*unixfs.FSHandle, error) {
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	value, err := client.GetResourceValue(destParentResourceID)
	if err != nil {
		return nil, err
	}

	destParentResource, ok := value.(*FSHandleResource)
	if !ok {
		return nil, errors.New("destination parent is not a unixfs handle resource")
	}

	destParentHandle, err := destParentResource.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	return destParentHandle, nil
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
	name := req.GetName()

	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

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
	path := req.GetPath()

	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

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
	offset := req.GetOffset()
	length := req.GetLength()

	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

	// length<=0 requests the remaining file from offset, but only when that
	// fits in one bounded resource response. Larger reads must use GetSize and
	// issue chunked positive-length ReadAt calls.
	if length <= 0 {
		size, err := handle.GetSize(ctx)
		if err != nil {
			return nil, err
		}
		length = int64(size) - offset
		if length <= 0 {
			return &s4wave_unixfs.HandleReadAtResponse{
				Eof: true,
			}, nil
		}
		if length > fsHandleMaxReadSize {
			return nil, errors.Errorf(
				"unixfs ReadAt length=0 would exceed max response size %d; use GetSize and chunked ReadAt",
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
	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

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
	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

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
	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

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
	ctx := strm.Context()
	skip := req.GetSkip()

	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return err
	}
	defer handle.Release()

	err = handle.ReaddirAll(ctx, skip, func(ent unixfs.FSCursorDirent) error {
		entry := dirEntryFromCursor(ent)

		// Try to get additional info (size, mtime, mode)
		childHandle, lookupErr := handle.Lookup(ctx, ent.GetName())
		if lookupErr == nil && childHandle != nil {
			defer childHandle.Release()

			if info, infoErr := childHandle.GetFileInfo(ctx); infoErr == nil {
				entry.Size = uint64(info.Size())
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
	names := req.GetNames()
	nodeType := req.GetNodeType()
	mode := req.GetMode()
	checkExist := req.GetCheckExist()

	var fsNodeType unixfs.FSCursorNodeType
	switch nodeType {
	case s4wave_unixfs.MknodType_MKNOD_TYPE_FILE:
		fsNodeType = unixfs.NewFSCursorNodeType_File()
	case s4wave_unixfs.MknodType_MKNOD_TYPE_DIR:
		fsNodeType = unixfs.NewFSCursorNodeType_Dir()
	default:
		fsNodeType = unixfs.NewFSCursorNodeType_File()
	}

	if mode == 0 {
		mode = uint32(unixfs.DefaultPermissions(fsNodeType))
	}

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
	pathParts := req.GetPathParts()
	mode := req.GetMode()

	if mode == 0 {
		mode = 0o755
	}

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
	sourceName := req.GetSourceName()
	destName := req.GetDestName()
	destParentResourceID := req.GetDestParentResourceId()

	if sourceName == "" {
		return nil, errors.New("source_name is required for rename")
	}

	if err := r.mutate(func() error {
		sourceHandle, err := r.handle.Lookup(ctx, sourceName)
		if err != nil {
			return err
		}
		defer sourceHandle.Release()

		var destParentHandle *unixfs.FSHandle
		if destParentResourceID == 0 {
			destParentHandle, err = r.handle.Clone(ctx)
		} else {
			destParentHandle, err = resolveDestParentHandle(ctx, destParentResourceID)
		}
		if err != nil {
			return err
		}
		defer destParentHandle.Release()

		return sourceHandle.Rename(ctx, destParentHandle, destName, time.Now())
	}); err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleRenameResponse{}, nil
}

// UploadFile uploads a file via client-streaming.
func (r *FSHandleResource) UploadFile(strm s4wave_unixfs.SRPCFSHandleResourceService_UploadFileStream) (*s4wave_unixfs.HandleUploadFileResponse, error) {
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
	pr, pw := io.Pipe()

	var uploadErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		nodeType := unixfs.NewFSCursorNodeType_File()
		if mode == 0 {
			mode = uint32(unixfs.DefaultPermissions(nodeType))
		}
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
	}()

	var bytesWritten int64
	if data := first.GetData(); len(data) > 0 {
		if err := validateUploadDataFrame(data); err != nil {
			pw.CloseWithError(err)
			<-done
			return nil, err
		}
		recordUploadMetric(ctx, UploadMetric{Stage: "receive-data", Bytes: len(data)})
		_, err = pw.Write(data)
		if err != nil {
			pw.CloseWithError(err)
			<-done
			return nil, err
		}
		bytesWritten += int64(len(data))
	}

	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			pw.CloseWithError(err)
			<-done
			return nil, err
		}
		data := msg.GetData()
		if len(data) > 0 {
			if err := validateUploadDataFrame(data); err != nil {
				pw.CloseWithError(err)
				<-done
				return nil, err
			}
			recordUploadMetric(ctx, UploadMetric{Stage: "receive-data", Bytes: len(data)})
			_, err = pw.Write(data)
			if err != nil {
				pw.CloseWithError(err)
				<-done
				return nil, err
			}
			bytesWritten += int64(len(data))
		}
	}

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
	handle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Release()

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
	clonedHandle, err := r.cloneHandle(ctx)
	if err != nil {
		return nil, err
	}

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
	ctx := strm.Context()

	var prev *s4wave_unixfs.HandleWatchReaddirResponse
	var lastObjRev uint64
	var haveObjRev bool
	watchHandle := r.handle
	if r.ws != nil && r.objKey != "" {
		var err error
		watchHandle, err = r.newObjectFSHandle(ctx)
		if err != nil {
			return err
		}
		defer watchHandle.Release()
	}
	for {
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		objState, objRev, err := r.watchObjectRev(ctx)
		if err != nil {
			return err
		}
		if haveObjRev && objState != nil && objRev != lastObjRev {
			nextWatchHandle, err := r.newObjectFSHandle(ctx)
			if err != nil {
				return err
			}
			watchHandle.Release()
			watchHandle = nextWatchHandle
		}

		entries, err := readWatchEntries(ctx, watchHandle)
		if err != nil {
			return err
		}

		resp := &s4wave_unixfs.HandleWatchReaddirResponse{
			Entries: entries,
		}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}
		if objState != nil {
			lastObjRev = objRev
			haveObjRev = true
		}

		if err := r.waitReaddirChange(ctx, ch, objState, objRev); err != nil {
			return err
		}
	}
}

func (r *FSHandleResource) watchObjectRev(ctx context.Context) (world.ObjectState, uint64, error) {
	if r.ws == nil || r.objKey == "" {
		return nil, 0, nil
	}
	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return nil, 0, err
	}
	if !found {
		return nil, 0, world.ErrObjectNotFound
	}
	_, rev, err := objState.GetRootRef(ctx)
	if err != nil {
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
	if objState == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-localChange:
			return nil
		}
	}

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	objChange := make(chan error, 1)
	go func() {
		_, err := objState.WaitRev(waitCtx, objRev+1, false)
		objChange <- err
	}()

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
