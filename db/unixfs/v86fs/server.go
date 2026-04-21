package unixfs_v86fs

import (
	"context"
	"io/fs"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// MountResolver resolves a mount name to an FSHandle root.
// Called when the client sends a MOUNT request.
type MountResolver func(ctx context.Context, name string) (*unixfs.FSHandle, error)

// MountEntry is a dynamic mount table entry.
type MountEntry struct {
	// Name is the mount name used for v86fs MOUNT resolution.
	Name string
	// Path is the guest filesystem path to mount at.
	Path string
	// Handle is the FSHandle root for this mount.
	Handle *unixfs.FSHandle
}

// Server implements the V86fsService SRPC server.
// It relays v86fs operations from a browser VM to FSHandle storage.
type Server struct {
	resolver MountResolver

	mtx      sync.Mutex
	mounts   map[string]*MountEntry
	sessions map[*session]struct{}
}

// NewServer constructs a new v86fs relay server.
func NewServer(resolver MountResolver) *Server {
	return &Server{
		resolver: resolver,
		mounts:   make(map[string]*MountEntry),
		sessions: make(map[*session]struct{}),
	}
}

// AddMount registers a dynamic mount and sends MOUNT_NOTIFY to active sessions.
func (s *Server) AddMount(name, path string, handle *unixfs.FSHandle) {
	s.mtx.Lock()
	s.mounts[name] = &MountEntry{Name: name, Path: path, Handle: handle}
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mtx.Unlock()

	msg := &V86FsMessage{
		Body: &V86FsMessage_MountNotify{
			MountNotify: &V86FsMountNotify{
				Name:      name,
				MountPath: path,
			},
		},
	}
	for _, sess := range sessions {
		sess.queueNotification(msg)
	}
}

// RemoveMount removes a dynamic mount and sends UMOUNT_NOTIFY to active sessions.
func (s *Server) RemoveMount(name string) {
	s.mtx.Lock()
	entry := s.mounts[name]
	delete(s.mounts, name)
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mtx.Unlock()

	if entry == nil {
		return
	}

	msg := &V86FsMessage{
		Body: &V86FsMessage_UmountNotify{
			UmountNotify: &V86FsUmountNotify{
				MountPath: entry.Path,
			},
		},
	}
	for _, sess := range sessions {
		sess.queueNotification(msg)
	}
}

// ListMounts returns the current dynamic mount table.
func (s *Server) ListMounts() []*MountEntry {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	result := make([]*MountEntry, 0, len(s.mounts))
	for _, entry := range s.mounts {
		result = append(result, entry)
	}
	return result
}

// resolveMountName resolves a mount name: dynamic table first, then fallback resolver.
func (s *Server) resolveMountName(ctx context.Context, name string) (*unixfs.FSHandle, error) {
	s.mtx.Lock()
	entry := s.mounts[name]
	s.mtx.Unlock()
	if entry != nil {
		return entry.Handle.Clone(ctx)
	}
	if s.resolver != nil {
		return s.resolver(ctx, name)
	}
	return nil, unixfs_errors.ErrNotExist
}

// RelayV86Fs implements the bidirectional streaming RPC.
func (s *Server) RelayV86Fs(strm SRPCV86FsService_RelayV86FsStream) error {
	sess := &session{
		server:  s,
		strm:    strm,
		inodes:  make(map[uint64]*inodeEntry),
		handles: make(map[uint64]*handleEntry),
	}

	// Register the session and snapshot the current mount table so we can
	// seed the session with MOUNT_NOTIFY frames for pre-registered mounts.
	s.mtx.Lock()
	s.sessions[sess] = struct{}{}
	seed := make([]*V86FsMessage, 0, len(s.mounts))
	for _, entry := range s.mounts {
		seed = append(seed, &V86FsMessage{
			Body: &V86FsMessage_MountNotify{
				MountNotify: &V86FsMountNotify{
					Name:      entry.Name,
					MountPath: entry.Path,
				},
			},
		})
	}
	s.mtx.Unlock()
	for _, msg := range seed {
		sess.queueNotification(msg)
	}

	defer func() {
		s.mtx.Lock()
		delete(s.sessions, sess)
		s.mtx.Unlock()
		sess.cleanup()
	}()

	return sess.run()
}

// session tracks state for one RelayV86Fs stream.
type session struct {
	server *Server
	strm   SRPCV86FsService_RelayV86FsStream

	mtx       sync.Mutex
	inodeCtr  uint64
	handleCtr uint64
	inodes    map[uint64]*inodeEntry
	handles   map[uint64]*handleEntry

	// bcast guards pending and wakes the run loop when notifications are queued.
	bcast   broadcast.Broadcast
	pending []*V86FsMessage
}

// inodeEntry tracks an open FSHandle associated with an inode ID.
type inodeEntry struct {
	id     uint64
	handle *unixfs.FSHandle
}

// handleEntry tracks an open file for read/write.
type handleEntry struct {
	inodeID uint64
	handle  *unixfs.FSHandle
}

// queueNotification appends a notification and wakes the run loop.
func (ss *session) queueNotification(msg *V86FsMessage) {
	ss.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ss.pending = append(ss.pending, msg)
		broadcast()
	})
}

// allocInodeID allocates a new inode ID and registers the FSHandle.
// Starts a background goroutine to register change callbacks for push invalidation.
func (ss *session) allocInodeID(h *unixfs.FSHandle) uint64 {
	ss.mtx.Lock()
	ss.inodeCtr++
	id := ss.inodeCtr
	ss.inodes[id] = &inodeEntry{id: id, handle: h}
	ss.mtx.Unlock()

	// Register change callback in background to avoid blocking the session.
	go func() {
		ctx := ss.strm.Context()
		_ = h.AccessOps(ctx, func(cursor unixfs.FSCursor, _ unixfs.FSCursorOps) error {
			cursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
				if ch == nil {
					return true
				}
				ss.queueNotification(&V86FsMessage{
					Body: &V86FsMessage_Invalidate{
						Invalidate: &V86FsInvalidate{
							InodeId: id,
							Offset:  ch.Offset,
							Size:    ch.Size,
						},
					},
				})
				return !ch.Released
			})
			return nil
		})
	}()

	return id
}

// getInode returns the FSHandle for an inode ID.
func (ss *session) getInode(id uint64) *unixfs.FSHandle {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	entry := ss.inodes[id]
	if entry == nil {
		return nil
	}
	return entry.handle
}

// allocHandleID allocates a new file handle ID.
func (ss *session) allocHandleID(inodeID uint64, h *unixfs.FSHandle) uint64 {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	ss.handleCtr++
	id := ss.handleCtr
	ss.handles[id] = &handleEntry{inodeID: inodeID, handle: h}
	return id
}

// getHandle returns the handle entry for a handle ID.
func (ss *session) getHandle(id uint64) *handleEntry {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	return ss.handles[id]
}

// removeHandle removes and returns the handle entry.
func (ss *session) removeHandle(id uint64) *handleEntry {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	entry := ss.handles[id]
	delete(ss.handles, id)
	return entry
}

// cleanup releases all tracked handles and inodes.
func (ss *session) cleanup() {
	ss.mtx.Lock()
	defer ss.mtx.Unlock()
	for _, h := range ss.handles {
		h.handle.Release()
	}
	ss.handles = nil
	for _, e := range ss.inodes {
		e.handle.Release()
	}
	ss.inodes = nil
}

// recvMsg is a received message or error from the stream.
type recvMsg struct {
	msg *V86FsMessage
	err error
}

// run processes incoming messages and sends notifications.
func (ss *session) run() error {
	ctx := ss.strm.Context()
	recvCh := make(chan recvMsg, 1)

	// Receive goroutine.
	go func() {
		for {
			msg, err := ss.strm.Recv()
			recvCh <- recvMsg{msg, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		// Snapshot and drain pending notifications under bcast lock.
		var waitCh <-chan struct{}
		var pending []*V86FsMessage
		ss.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			pending = ss.pending
			ss.pending = nil
		})
		for _, msg := range pending {
			if err := ss.strm.Send(msg); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			// New notifications queued, loop back to drain.
		case rm := <-recvCh:
			if rm.err != nil {
				return rm.err
			}
			reply, err := ss.dispatch(ctx, rm.msg)
			if err != nil {
				reply = &V86FsMessage{
					Tag: rm.msg.GetTag(),
					Body: &V86FsMessage_ErrorReply{
						ErrorReply: &V86FsErrorReply{Status: errnoFromError(err)},
					},
				}
			}
			if reply != nil {
				if err := ss.strm.Send(reply); err != nil {
					return err
				}
			}
		}
	}
}

// dispatch handles a single incoming v86fs message.
func (ss *session) dispatch(ctx context.Context, msg *V86FsMessage) (*V86FsMessage, error) {
	tag := msg.GetTag()

	switch body := msg.GetBody().(type) {
	case *V86FsMessage_MountRequest:
		return ss.handleMount(ctx, tag, body.MountRequest)
	case *V86FsMessage_LookupRequest:
		return ss.handleLookup(ctx, tag, body.LookupRequest)
	case *V86FsMessage_GetattrRequest:
		return ss.handleGetattr(ctx, tag, body.GetattrRequest)
	case *V86FsMessage_ReaddirRequest:
		return ss.handleReaddir(ctx, tag, body.ReaddirRequest)
	case *V86FsMessage_OpenRequest:
		return ss.handleOpen(ctx, tag, body.OpenRequest)
	case *V86FsMessage_CloseRequest:
		return ss.handleClose(ctx, tag, body.CloseRequest)
	case *V86FsMessage_ReadRequest:
		return ss.handleRead(ctx, tag, body.ReadRequest)
	case *V86FsMessage_CreateRequest:
		return ss.handleCreate(ctx, tag, body.CreateRequest)
	case *V86FsMessage_WriteRequest:
		return ss.handleWrite(ctx, tag, body.WriteRequest)
	case *V86FsMessage_MkdirRequest:
		return ss.handleMkdir(ctx, tag, body.MkdirRequest)
	case *V86FsMessage_SetattrRequest:
		return ss.handleSetattr(ctx, tag, body.SetattrRequest)
	case *V86FsMessage_FsyncRequest:
		return ss.handleFsync(ctx, tag, body.FsyncRequest)
	case *V86FsMessage_UnlinkRequest:
		return ss.handleUnlink(ctx, tag, body.UnlinkRequest)
	case *V86FsMessage_RenameRequest:
		return ss.handleRename(ctx, tag, body.RenameRequest)
	case *V86FsMessage_SymlinkRequest:
		return ss.handleSymlink(ctx, tag, body.SymlinkRequest)
	case *V86FsMessage_ReadlinkRequest:
		return ss.handleReadlink(ctx, tag, body.ReadlinkRequest)
	case *V86FsMessage_StatfsRequest:
		return ss.handleStatfs(ctx, tag, body.StatfsRequest)
	default:
		return nil, errors.New("unknown message type")
	}
}

func (ss *session) handleMount(ctx context.Context, tag uint32, req *V86FsMountRequest) (*V86FsMessage, error) {
	handle, err := ss.server.resolveMountName(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	mode, err := getNodeMode(ctx, handle)
	if err != nil {
		handle.Release()
		return nil, err
	}
	id := ss.allocInodeID(handle)
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_MountReply{
			MountReply: &V86FsMountReply{
				Status:      0,
				RootInodeId: id,
				Mode:        mode,
			},
		},
	}, nil
}

func (ss *session) handleLookup(ctx context.Context, tag uint32, req *V86FsLookupRequest) (*V86FsMessage, error) {
	parent := ss.getInode(req.GetParentId())
	if parent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	child, err := parent.Lookup(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	mode, err := getNodeMode(ctx, child)
	if err != nil {
		child.Release()
		return nil, err
	}
	size, err := child.GetSize(ctx)
	if err != nil {
		child.Release()
		return nil, err
	}
	id := ss.allocInodeID(child)
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_LookupReply{
			LookupReply: &V86FsLookupReply{
				Status:  0,
				InodeId: id,
				Mode:    mode,
				Size:    size,
			},
		},
	}, nil
}

func (ss *session) handleGetattr(ctx context.Context, tag uint32, req *V86FsGetattrRequest) (*V86FsMessage, error) {
	h := ss.getInode(req.GetInodeId())
	if h == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	mode, err := getNodeMode(ctx, h)
	if err != nil {
		return nil, err
	}
	size, err := h.GetSize(ctx)
	if err != nil {
		return nil, err
	}
	mtime, err := h.GetModTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_GetattrReply{
			GetattrReply: &V86FsGetattrReply{
				Status:    0,
				Mode:      mode,
				Size:      size,
				MtimeSec:  mtime.Unix(),
				MtimeNsec: uint32(mtime.Nanosecond()),
			},
		},
	}, nil
}

func (ss *session) handleReaddir(ctx context.Context, tag uint32, req *V86FsReaddirRequest) (*V86FsMessage, error) {
	h := ss.getInode(req.GetDirId())
	if h == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	var entries []*V86FsDirEntry
	err := h.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		dtType := nodeTypeToDtType(ent)
		// Allocate an inode for this entry via lookup.
		child, lerr := h.Lookup(ctx, ent.GetName())
		if lerr != nil {
			return lerr
		}
		childID := ss.allocInodeID(child)
		entries = append(entries, &V86FsDirEntry{
			InodeId: childID,
			DtType:  dtType,
			Name:    ent.GetName(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_ReaddirReply{
			ReaddirReply: &V86FsReaddirReply{
				Status:  0,
				Entries: entries,
			},
		},
	}, nil
}

func (ss *session) handleOpen(ctx context.Context, tag uint32, req *V86FsOpenRequest) (*V86FsMessage, error) {
	h := ss.getInode(req.GetInodeId())
	if h == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	clone, err := h.Clone(ctx)
	if err != nil {
		return nil, err
	}
	hid := ss.allocHandleID(req.GetInodeId(), clone)
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_OpenReply{
			OpenReply: &V86FsOpenReply{
				Status:   0,
				HandleId: hid,
			},
		},
	}, nil
}

func (ss *session) handleClose(_ context.Context, tag uint32, req *V86FsCloseRequest) (*V86FsMessage, error) {
	entry := ss.removeHandle(req.GetHandleId())
	if entry != nil {
		entry.handle.Release()
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_CloseReply{
			CloseReply: &V86FsCloseReply{Status: 0},
		},
	}, nil
}

func (ss *session) handleRead(ctx context.Context, tag uint32, req *V86FsReadRequest) (*V86FsMessage, error) {
	entry := ss.getHandle(req.GetHandleId())
	if entry == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	buf := make([]byte, req.GetSize())
	n, err := entry.handle.ReadAt(ctx, int64(req.GetOffset()), buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_ReadReply{
			ReadReply: &V86FsReadReply{
				Status: 0,
				Data:   buf[:n],
			},
		},
	}, nil
}

func (ss *session) handleCreate(ctx context.Context, tag uint32, req *V86FsCreateRequest) (*V86FsMessage, error) {
	parent := ss.getInode(req.GetParentId())
	if parent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	perm := fs.FileMode(req.GetMode()) & fs.ModePerm
	err := parent.Mknod(ctx, true, []string{req.GetName()}, unixfs.NewFSCursorNodeType_File(), perm, time.Now())
	if err != nil {
		return nil, err
	}
	child, err := parent.Lookup(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	id := ss.allocInodeID(child)
	mode, err := getNodeMode(ctx, child)
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_CreateReply{
			CreateReply: &V86FsCreateReply{
				Status:  0,
				InodeId: id,
				Mode:    mode,
			},
		},
	}, nil
}

func (ss *session) handleWrite(ctx context.Context, tag uint32, req *V86FsWriteRequest) (*V86FsMessage, error) {
	h := ss.getInode(req.GetInodeId())
	if h == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	data := req.GetData()
	err := h.WriteAt(ctx, int64(req.GetOffset()), data, time.Now())
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_WriteReply{
			WriteReply: &V86FsWriteReply{
				Status:       0,
				BytesWritten: uint32(len(data)),
			},
		},
	}, nil
}

func (ss *session) handleMkdir(ctx context.Context, tag uint32, req *V86FsMkdirRequest) (*V86FsMessage, error) {
	parent := ss.getInode(req.GetParentId())
	if parent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	perm := fs.FileMode(req.GetMode()) & fs.ModePerm
	err := parent.Mknod(ctx, true, []string{req.GetName()}, unixfs.NewFSCursorNodeType_Dir(), perm, time.Now())
	if err != nil {
		return nil, err
	}
	child, err := parent.Lookup(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	id := ss.allocInodeID(child)
	mode, err := getNodeMode(ctx, child)
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_MkdirReply{
			MkdirReply: &V86FsMkdirReply{
				Status:  0,
				InodeId: id,
				Mode:    mode,
			},
		},
	}, nil
}

func (ss *session) handleSetattr(ctx context.Context, tag uint32, req *V86FsSetattrRequest) (*V86FsMessage, error) {
	h := ss.getInode(req.GetInodeId())
	if h == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	valid := req.GetValid()
	now := time.Now()
	if valid&attrMode != 0 {
		perm := fs.FileMode(req.GetMode()) & fs.ModePerm
		if err := h.SetPermissions(ctx, perm, now); err != nil {
			return nil, err
		}
	}
	if valid&attrSize != 0 {
		if err := h.Truncate(ctx, req.GetSize(), now); err != nil {
			return nil, err
		}
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_SetattrReply{
			SetattrReply: &V86FsSetattrReply{Status: 0},
		},
	}, nil
}

func (ss *session) handleFsync(_ context.Context, tag uint32, _ *V86FsFsyncRequest) (*V86FsMessage, error) {
	// Fsync is a no-op for block storage (writes are synchronous).
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_FsyncReply{
			FsyncReply: &V86FsFsyncReply{Status: 0},
		},
	}, nil
}

func (ss *session) handleUnlink(ctx context.Context, tag uint32, req *V86FsUnlinkRequest) (*V86FsMessage, error) {
	parent := ss.getInode(req.GetParentId())
	if parent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	err := parent.Remove(ctx, []string{req.GetName()}, time.Now())
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_UnlinkReply{
			UnlinkReply: &V86FsUnlinkReply{Status: 0},
		},
	}, nil
}

func (ss *session) handleRename(ctx context.Context, tag uint32, req *V86FsRenameRequest) (*V86FsMessage, error) {
	oldParent := ss.getInode(req.GetOldParentId())
	if oldParent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	newParent := ss.getInode(req.GetNewParentId())
	if newParent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	src, err := oldParent.Lookup(ctx, req.GetOldName())
	if err != nil {
		return nil, err
	}
	err = src.Rename(ctx, newParent, req.GetNewName(), time.Now())
	src.Release()
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_RenameReply{
			RenameReply: &V86FsRenameReply{Status: 0},
		},
	}, nil
}

func (ss *session) handleSymlink(ctx context.Context, tag uint32, req *V86FsSymlinkRequest) (*V86FsMessage, error) {
	parent := ss.getInode(req.GetParentId())
	if parent == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	target := req.GetTarget()
	isAbs := len(target) > 0 && target[0] == '/'
	targetParts, _ := unixfs.SplitPath(target)
	err := parent.Symlink(ctx, true, req.GetName(), targetParts, isAbs, time.Now())
	if err != nil {
		return nil, err
	}
	child, err := parent.Lookup(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	id := ss.allocInodeID(child)
	mode, err := getNodeMode(ctx, child)
	if err != nil {
		return nil, err
	}
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_SymlinkReply{
			SymlinkReply: &V86FsSymlinkReply{
				Status:  0,
				InodeId: id,
				Mode:    mode,
			},
		},
	}, nil
}

func (ss *session) handleReadlink(ctx context.Context, tag uint32, req *V86FsReadlinkRequest) (*V86FsMessage, error) {
	h := ss.getInode(req.GetInodeId())
	if h == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	parts, isAbs, err := h.Readlink(ctx, "")
	if err != nil {
		return nil, err
	}
	target := unixfs.JoinPath(parts, isAbs)
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_ReadlinkReply{
			ReadlinkReply: &V86FsReadlinkReply{
				Status: 0,
				Target: target,
			},
		},
	}, nil
}

func (ss *session) handleStatfs(_ context.Context, tag uint32, _ *V86FsStatfsRequest) (*V86FsMessage, error) {
	return &V86FsMessage{
		Tag: tag,
		Body: &V86FsMessage_StatfsReply{
			StatfsReply: &V86FsStatfsReply{
				Status: 0,
				Blocks: 1 << 20,
				Bfree:  1 << 19,
				Bavail: 1 << 19,
				Files:  1 << 16,
				Ffree:  1 << 15,
				Bsize:  4096,
			},
		},
	}, nil
}

// _ is a type assertion.
var _ SRPCV86FsServiceServer = (*Server)(nil)
