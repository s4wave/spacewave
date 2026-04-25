package resource_unixfs

import (
	"context"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

// FSHandleResource implements FSHandleResourceService for a single FSHandle.
// Each instance wraps exactly one hydra/unixfs.FSHandle with 1:1 mapping.
type FSHandleResource struct {
	handle *unixfs.FSHandle
	mux    srpc.Mux
	bcast  *broadcast.Broadcast
	ws     world.WorldState
	objKey string
	fsType unixfs_world.FSType
	path   []string
}

// NewFSHandleResource creates a new FSHandleResource.
func NewFSHandleResource(handle *unixfs.FSHandle) *FSHandleResource {
	return newFSHandleResource(handle, nil, nil, "", 0, nil)
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
	return newFSHandleResource(handle, bcast, ws, objKey, fsType, path)
}

func newFSHandleResource(
	handle *unixfs.FSHandle,
	bcast *broadcast.Broadcast,
	ws world.WorldState,
	objKey string,
	fsType unixfs_world.FSType,
	path []string,
) *FSHandleResource {
	if bcast == nil {
		bcast = &broadcast.Broadcast{}
	}
	r := &FSHandleResource{
		handle: handle,
		bcast:  bcast,
		ws:     ws,
		objKey: objKey,
		fsType: fsType,
		path:   append([]string(nil), path...),
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

// getDisplayPath returns the current handle path in slash-separated form.
func (r *FSHandleResource) getDisplayPath() string {
	if len(r.path) == 0 {
		return "."
	}
	return path.Join(r.path...)
}

// reloadHandle reloads the current handle from world state at r.path.
func (r *FSHandleResource) reloadHandle(ctx context.Context) error {
	if r.ws == nil || r.objKey == "" {
		return nil
	}
	fsCursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		nil,
		r.ws,
		r.objKey,
		r.fsType,
		"",
	)

	var err error
	var nextHandle *unixfs.FSHandle
	if len(r.path) == 0 {
		nextHandle, err = unixfs.NewFSHandle(fsCursor)
		if err != nil {
			fsCursor.Release()
			return err
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
			fsCursor.Release()
			return err
		}
	}

	r.handle.Release()
	r.handle = nextHandle
	return nil
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

	destParentHandle, err := destParentResource.GetHandle().Clone(ctx)
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

	childHandle, err := r.handle.Lookup(ctx, name)
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

	childHandle, traversedPath, err := r.handle.LookupPath(ctx, path)
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

	// length=0 means "read entire file from offset"
	if length <= 0 {
		size, err := r.handle.GetSize(ctx)
		if err != nil {
			return nil, err
		}
		length = int64(size) - offset
		if length <= 0 {
			return &s4wave_unixfs.HandleReadAtResponse{
				Eof: true,
			}, nil
		}
	}

	// Cap at 64 MiB to prevent excessive allocation.
	const maxReadSize = 64 * 1024 * 1024
	if length > maxReadSize {
		length = maxReadSize
	}

	data := make([]byte, length)
	bytesRead, err := r.handle.ReadAt(ctx, offset, data)

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

	err := r.handle.WriteAt(ctx, offset, data, time.Now())
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return &s4wave_unixfs.HandleWriteAtResponse{
		BytesWritten: int64(len(data)),
	}, nil
}

// Truncate truncates the file to the given size.
func (r *FSHandleResource) Truncate(ctx context.Context, req *s4wave_unixfs.HandleTruncateRequest) (*s4wave_unixfs.HandleTruncateResponse, error) {
	size := req.GetSize()

	err := r.handle.Truncate(ctx, size, time.Now())
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return &s4wave_unixfs.HandleTruncateResponse{}, nil
}

// GetSize returns the current size of the file.
func (r *FSHandleResource) GetSize(ctx context.Context, req *s4wave_unixfs.HandleGetSizeRequest) (*s4wave_unixfs.HandleGetSizeResponse, error) {
	size, err := r.handle.GetSize(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleGetSizeResponse{
		Size: size,
	}, nil
}

// GetFileInfo returns file metadata for the handle's location.
func (r *FSHandleResource) GetFileInfo(ctx context.Context, req *s4wave_unixfs.HandleGetFileInfoRequest) (*s4wave_unixfs.HandleGetFileInfoResponse, error) {
	info, err := getFileInfo(ctx, r.handle)
	if err != nil {
		return nil, err
	}

	return &s4wave_unixfs.HandleGetFileInfoResponse{
		Info: info,
	}, nil
}

// GetNodeType returns the node type (file, directory, symlink).
func (r *FSHandleResource) GetNodeType(ctx context.Context, req *s4wave_unixfs.HandleGetNodeTypeRequest) (*s4wave_unixfs.HandleGetNodeTypeResponse, error) {
	nodeType, err := r.handle.GetNodeType(ctx)
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

	err := r.handle.ReaddirAll(ctx, skip, func(ent unixfs.FSCursorDirent) error {
		entry := &s4wave_unixfs.DirEntry{
			Name:      ent.GetName(),
			IsDir:     ent.GetIsDirectory(),
			IsSymlink: ent.GetIsSymlink(),
		}

		// Try to get additional info (size, mtime, mode)
		childHandle, lookupErr := r.handle.Lookup(ctx, ent.GetName())
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

	err := r.handle.Mknod(ctx, checkExist, names, fsNodeType, fs.FileMode(mode), time.Now())
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return &s4wave_unixfs.HandleMknodResponse{}, nil
}

// Remove removes files or directories by name.
func (r *FSHandleResource) Remove(ctx context.Context, req *s4wave_unixfs.HandleRemoveRequest) (*s4wave_unixfs.HandleRemoveResponse, error) {
	names := req.GetNames()

	err := r.handle.Remove(ctx, names, time.Now())
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return &s4wave_unixfs.HandleRemoveResponse{}, nil
}

// MkdirAll creates a directory and all parent directories.
func (r *FSHandleResource) MkdirAll(ctx context.Context, req *s4wave_unixfs.HandleMkdirAllRequest) (*s4wave_unixfs.HandleMkdirAllResponse, error) {
	pathParts := req.GetPathParts()
	mode := req.GetMode()

	if mode == 0 {
		mode = 0o755
	}

	err := r.handle.MkdirAll(ctx, pathParts, fs.FileMode(mode), time.Now())
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
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

	sourceHandle, err := r.handle.Lookup(ctx, sourceName)
	if err != nil {
		return nil, err
	}
	defer sourceHandle.Release()

	var destParentHandle *unixfs.FSHandle
	if destParentResourceID == 0 {
		destParentHandle, err = r.handle.Clone(ctx)
		if err != nil {
			return nil, err
		}
		defer destParentHandle.Release()
	} else {
		destParentHandle, err = resolveDestParentHandle(ctx, destParentResourceID)
		if err != nil {
			return nil, err
		}
		defer destParentHandle.Release()
	}

	err = sourceHandle.Rename(ctx, destParentHandle, destName, time.Now())
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
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
		uploadErr = r.handle.MknodWithContent(
			ctx, name, nodeType, totalSize, pr,
			fs.FileMode(mode), time.Now(),
		)
	}()

	var bytesWritten int64
	if len(first.GetData()) > 0 {
		_, err = pw.Write(first.GetData())
		if err != nil {
			pw.CloseWithError(err)
			<-done
			return nil, err
		}
		bytesWritten += int64(len(first.GetData()))
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

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return &s4wave_unixfs.HandleUploadFileResponse{
		BytesWritten: bytesWritten,
	}, nil
}

// Readlink reads the target of a symbolic link at this handle.
func (r *FSHandleResource) Readlink(ctx context.Context, req *s4wave_unixfs.HandleReadlinkRequest) (*s4wave_unixfs.HandleReadlinkResponse, error) {
	parts, isAbsolute, err := r.handle.Readlink(ctx, "")
	if err != nil {
		return nil, err
	}
	return &s4wave_unixfs.HandleReadlinkResponse{
		Target: unixfs.JoinPath(parts, isAbsolute),
	}, nil
}

// Clone creates a copy of this handle pointing to the same location.
func (r *FSHandleResource) Clone(ctx context.Context, req *s4wave_unixfs.HandleCloneRequest) (*s4wave_unixfs.HandleCloneResponse, error) {
	clonedHandle, err := r.handle.Clone(ctx)
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

// readAllEntries reads all directory entries and returns them as a slice.
func (r *FSHandleResource) readAllEntries(ctx context.Context) ([]*s4wave_unixfs.DirEntry, error) {
	var entries []*s4wave_unixfs.DirEntry
	err := r.handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entry := &s4wave_unixfs.DirEntry{
			Name:      ent.GetName(),
			IsDir:     ent.GetIsDirectory(),
			IsSymlink: ent.GetIsSymlink(),
		}

		childHandle, lookupErr := r.handle.Lookup(ctx, ent.GetName())
		if lookupErr == nil && childHandle != nil {
			defer childHandle.Release()
			if info, infoErr := childHandle.GetFileInfo(ctx); infoErr == nil {
				entry.Size = uint64(info.Size())
				entry.ModTime = info.ModTime().Unix()
				entry.Mode = uint32(info.Mode())
			}
		}

		entries = append(entries, entry)
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

	for {
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})

		entries, err := r.readAllEntries(ctx)
		if err != nil {
			return err
		}

		if err := strm.Send(&s4wave_unixfs.HandleWatchReaddirResponse{
			Entries: entries,
		}); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// _ is a type assertion
var _ s4wave_unixfs.SRPCFSHandleResourceServiceServer = (*FSHandleResource)(nil)
