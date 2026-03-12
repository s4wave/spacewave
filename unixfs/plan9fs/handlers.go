package plan9fs

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
)

// isEOF checks if an error represents end-of-file.
func isEOF(err error) bool {
	if err == io.EOF {
		return true
	}
	s := err.Error()
	return s == "EOF" || s == "short read" || strings.Contains(s, "EOF")
}

// handleVersion processes TVERSION: negotiate protocol version and msize.
// Per 9p spec, TVERSION resets the session: all existing fids are released.
func (s *Server) handleVersion(tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	msize := buf.ReadU32()
	version := buf.ReadString()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// TVERSION resets session state
	s.fids.ReleaseAll()

	// clamp msize to avoid underflow in iounit calculations
	minMsize := uint32(headerSize) + 4
	if msize < minMsize {
		msize = minMsize
	}
	if msize < s.msize {
		s.msize = msize
	}

	// only support 9P2000.L
	respVersion := versionString
	if version != versionString {
		respVersion = "unknown"
	}

	resp := NewWriteBuffer(32)
	resp.WriteU32(s.msize)
	resp.WriteString(respVersion)
	return buildMessage(RVERSION, tag, resp.Bytes()), nil
}

// handleAttach processes TATTACH: attach to the root filesystem.
func (s *Server) handleAttach(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU32()    // afid (unused, no auth)
	_ = buf.ReadString() // uname
	_ = buf.ReadString() // aname
	uid := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	handle, err := s.root.Clone(ctx)
	if err != nil {
		return nil, err
	}

	fid := &Fid{
		id:     fidID,
		handle: handle,
		uid:    uid,
	}

	if err := s.fids.Add(fidID, fid); err != nil {
		handle.Release()
		return nil, err
	}

	qidPath := s.fids.AllocQIDPath(handle)
	qid := QID{Type: QidDir, Version: 0, Path: qidPath}

	resp := NewWriteBuffer(13)
	resp.WriteQID(qid)
	return buildMessage(RATTACH, tag, resp.Bytes()), nil
}

// handleWalk processes TWALK: walk to a named child, component by component.
func (s *Server) handleWalk(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	newFidID := buf.ReadU32()
	nwname := buf.ReadU16()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	if nwname > maxWalkNames {
		return nil, errTooManyNames
	}

	names := make([]string, nwname)
	for i := range names {
		names[i] = buf.ReadString()
	}
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// clone the fid's handle for walking
	handle, err := fid.handle.Clone(ctx)
	if err != nil {
		return nil, err
	}

	// walk component by component, collecting QIDs
	qids := make([]QID, 0, len(names))
	for _, name := range names {
		child, lerr := handle.Lookup(ctx, name)
		if lerr != nil {
			if len(qids) == 0 {
				handle.Release()
				return nil, lerr
			}
			// partial walk: keep handle alive for the new fid
			break
		}
		handle.Release()
		handle = child

		qidType := qidTypeForHandle(ctx, handle)
		qidPath := s.fids.AllocQIDPath(handle)
		qids = append(qids, QID{Type: qidType, Version: 0, Path: qidPath})
	}

	// zero-length walk: clone fid
	if nwname == 0 {
		// just clone the fid
	}

	newFid := &Fid{
		id:     newFidID,
		handle: handle,
		uid:    fid.uid,
	}

	// if newfid == fid, replace
	if newFidID == fidID {
		old, _ := s.fids.Remove(fidID)
		if old != nil {
			old.handle.Release()
		}
	}

	if err := s.fids.Add(newFidID, newFid); err != nil {
		handle.Release()
		return nil, err
	}

	resp := NewWriteBuffer(2 + len(qids)*13)
	resp.WriteU16(uint16(len(qids)))
	for _, q := range qids {
		resp.WriteQID(q)
	}
	return buildMessage(RWALK, tag, resp.Bytes()), nil
}

// handleLopen processes TLOPEN: mark a fid as open.
func (s *Server) handleLopen(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU32() // flags (ignored per design)
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}
	fid.opened = true

	qidType := qidTypeForHandle(ctx, fid.handle)
	qidPath := s.fids.AllocQIDPath(fid.handle)
	qid := QID{Type: qidType, Version: 0, Path: qidPath}

	// iounit: 0 means no limit (use msize - headerSize - 4 for read/write header)
	iounit := s.msize - headerSize - 4

	resp := NewWriteBuffer(17)
	resp.WriteQID(qid)
	resp.WriteU32(iounit)
	return buildMessage(RLOPEN, tag, resp.Bytes()), nil
}

// handleLcreate processes TLCREATE: create and open a file.
func (s *Server) handleLcreate(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	_ = buf.ReadU32() // flags (ignored)
	mode := buf.ReadU32()
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	perms := fs.FileMode(mode & 0o777)
	now := time.Now()

	if err := fid.handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), perms, now); err != nil {
		return nil, err
	}

	child, err := fid.handle.Lookup(ctx, name)
	if err != nil {
		return nil, err
	}

	fid.handle.Release()
	fid.handle = child
	fid.opened = true

	qidPath := s.fids.AllocQIDPath(child)
	qid := QID{Type: QidFile, Version: 0, Path: qidPath}
	iounit := s.msize - headerSize - 4

	resp := NewWriteBuffer(17)
	resp.WriteQID(qid)
	resp.WriteU32(iounit)
	return buildMessage(RLCREATE, tag, resp.Bytes()), nil
}

// handleRead processes TREAD: read data from an open fid.
func (s *Server) handleRead(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	offset := buf.ReadU64()
	count := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	maxData := s.msize - headerSize - 4
	if count > maxData {
		count = maxData
	}

	// check if offset is past file size
	fsize, sizeErr := fid.handle.GetSize(ctx)
	if sizeErr == nil && int64(offset) >= int64(fsize) {
		resp := NewWriteBuffer(4)
		resp.WriteU32(0)
		return buildMessage(RREAD, tag, resp.Bytes()), nil
	}

	data := make([]byte, count)
	n, readErr := fid.handle.ReadAt(ctx, int64(offset), data)
	if readErr != nil && n == 0 {
		// EOF at offset is normal in 9p — return empty read.
		if isEOF(readErr) {
			n = 0
		} else {
			return nil, readErr
		}
	}

	resp := NewWriteBuffer(4 + int(n))
	resp.WriteU32(uint32(n))
	resp.WriteBytes(data[:n])
	return buildMessage(RREAD, tag, resp.Bytes()), nil
}

// handleWrite processes TWRITE: write data to an open fid.
func (s *Server) handleWrite(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	offset := buf.ReadU64()
	count := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	maxData := s.msize - headerSize - 4
	if count > maxData {
		count = maxData
	}

	data := buf.ReadBytes(int(count))
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := fid.handle.WriteAt(ctx, int64(offset), data, now); err != nil {
		return nil, err
	}

	resp := NewWriteBuffer(4)
	resp.WriteU32(count)
	return buildMessage(RWRITE, tag, resp.Bytes()), nil
}

// handleClunk processes TCLUNK: release a fid.
func (s *Server) handleClunk(tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Remove(fidID)
	if err != nil {
		return nil, err
	}
	fid.handle.Release()

	return buildMessage(RCLUNK, tag, nil), nil
}

// handleRemove processes TREMOVE: clunk the fid and return ENOTSUP.
// TREMOVE is deprecated in 9p2000.L (clients should use TUNLINKAT).
func (s *Server) handleRemove(_ context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Remove(fidID)
	if err != nil {
		return nil, err
	}
	fid.handle.Release()

	return nil, errUnsupported
}

// handleGetattr processes TGETATTR: get file attributes.
func (s *Server) handleGetattr(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU64() // request_mask
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	nodeType, err := fid.handle.GetNodeType(ctx)
	if err != nil {
		return nil, err
	}

	size, err := fid.handle.GetSize(ctx)
	if err != nil {
		// non-file nodes may not support size
		size = 0
	}

	perms, err := fid.handle.GetPermissions(ctx)
	if err != nil {
		perms = 0o755
	}

	mtime, err := fid.handle.GetModTimestamp(ctx)
	if err != nil {
		mtime = time.Time{}
	}

	// build mode
	mode := uint32(perms & fs.ModePerm)
	if nodeType.GetIsDirectory() {
		mode |= 0o40000 // S_IFDIR
	} else if nodeType.GetIsSymlink() {
		mode |= 0o120000 // S_IFLNK
	} else {
		mode |= 0o100000 // S_IFREG
	}

	qidType := qidTypeFromNodeType(nodeType)
	qidPath := s.fids.AllocQIDPath(fid.handle)
	qid := QID{Type: qidType, Version: 0, Path: qidPath}

	mtimeSec := uint64(mtime.Unix())
	mtimeNsec := uint64(mtime.Nanosecond())

	// blocks = ceil(size / 512)
	blocks := size / 512
	if size%512 != 0 {
		blocks++
	}

	resp := NewWriteBuffer(160)
	resp.WriteU64(GetattrBasic) // valid mask
	resp.WriteQID(qid)
	resp.WriteU32(mode)      // mode
	resp.WriteU32(fid.uid)   // uid
	resp.WriteU32(fid.uid)   // gid
	resp.WriteU64(1)         // nlink
	resp.WriteU64(0)         // rdev
	resp.WriteU64(size)      // size
	resp.WriteU64(4096)      // blksize
	resp.WriteU64(blocks)    // blocks
	resp.WriteU64(mtimeSec)  // atime_sec
	resp.WriteU64(mtimeNsec) // atime_nsec
	resp.WriteU64(mtimeSec)  // mtime_sec
	resp.WriteU64(mtimeNsec) // mtime_nsec
	resp.WriteU64(mtimeSec)  // ctime_sec
	resp.WriteU64(mtimeNsec) // ctime_nsec
	resp.WriteU64(0)         // btime_sec
	resp.WriteU64(0)         // btime_nsec
	resp.WriteU64(0)         // gen
	resp.WriteU64(0)         // data_version
	return buildMessage(RGETATTR, tag, resp.Bytes()), nil
}

// handleSetattr processes TSETATTR: set file attributes.
func (s *Server) handleSetattr(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	valid := buf.ReadU32()
	mode := buf.ReadU32()
	_ = buf.ReadU32() // uid
	_ = buf.ReadU32() // gid
	size := buf.ReadU64()
	atimeSec := buf.ReadU64()
	atimeNsec := buf.ReadU64()
	_ = atimeSec
	_ = atimeNsec
	mtimeSec := buf.ReadU64()
	mtimeNsec := buf.ReadU64()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	if valid&SetattrMode != 0 {
		perms := fs.FileMode(mode & 0o777)
		now := time.Now()
		if err := fid.handle.SetPermissions(ctx, perms, now); err != nil {
			return nil, err
		}
	}

	if valid&SetattrSize != 0 {
		now := time.Now()
		if err := fid.handle.Truncate(ctx, size, now); err != nil {
			return nil, err
		}
	}

	if valid&SetattrMtimeSet != 0 {
		mtime := time.Unix(int64(mtimeSec), int64(mtimeNsec))
		if err := fid.handle.SetModTimestamp(ctx, mtime); err != nil {
			return nil, err
		}
	} else if valid&SetattrMtime != 0 {
		now := time.Now()
		if err := fid.handle.SetModTimestamp(ctx, now); err != nil {
			return nil, err
		}
	}

	return buildMessage(RSETATTR, tag, nil), nil
}

// handleReaddir processes TREADDIR: read directory entries.
func (s *Server) handleReaddir(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	offset := buf.ReadU64()
	count := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// serialize all entries, then slice to offset
	var entries []byte
	var entryIndex uint64
	err = fid.handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entryIndex++
		name := ent.GetName()
		qidType := qidTypeFromNodeType(ent)
		// each entry: qid(13) + offset(8) + type(1) + name(2+len)
		entBuf := NewWriteBuffer(24 + len(name))
		entBuf.WriteQID(QID{Type: qidType, Version: 0, Path: entryIndex})
		entBuf.WriteU64(entryIndex) // offset = sequential index
		entBuf.WriteU8(qidType)     // type
		entBuf.WriteString(name)
		entries = append(entries, entBuf.Bytes()...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// slice entries from offset
	result := entries
	if offset > 0 && len(entries) > 0 {
		if offset >= entryIndex {
			result = nil
		} else {
			result = sliceEntriesFromOffset(entries, offset)
		}
	}

	if uint32(len(result)) > count {
		result = truncateEntries(result, count)
	}

	resp := NewWriteBuffer(4 + len(result))
	resp.WriteU32(uint32(len(result)))
	resp.WriteBytes(result)
	return buildMessage(RREADDIR, tag, resp.Bytes()), nil
}

// sliceEntriesFromOffset returns entries starting from the given offset.
func sliceEntriesFromOffset(entries []byte, offset uint64) []byte {
	r := NewReadBuffer(entries)
	for r.Remaining() > 0 {
		start := r.off
		r.ReadQID()        // qid
		off := r.ReadU64() // offset
		r.ReadU8()         // type
		r.ReadString()     // name
		if r.Err() != nil {
			return nil
		}
		if off > offset {
			return entries[start:]
		}
	}
	return nil
}

// truncateEntries truncates serialized entries to fit within maxBytes.
func truncateEntries(entries []byte, maxBytes uint32) []byte {
	r := NewReadBuffer(entries)
	lastEnd := 0
	for r.Remaining() > 0 {
		r.ReadQID()    // qid
		r.ReadU64()    // offset
		r.ReadU8()     // type
		r.ReadString() // name
		if r.Err() != nil {
			break
		}
		if uint32(r.off) > maxBytes {
			break
		}
		lastEnd = r.off
	}
	return entries[:lastEnd]
}

// handleMkdir processes TMKDIR: create a directory.
func (s *Server) handleMkdir(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	mode := buf.ReadU32()
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	perms := fs.FileMode(mode & 0o777)
	now := time.Now()
	if err := fid.handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_Dir(), perms, now); err != nil {
		return nil, err
	}

	resp := NewWriteBuffer(13)
	resp.WriteQID(QID{Type: QidDir, Version: 0, Path: s.fids.qidPath.Add(1)})
	return buildMessage(RMKDIR, tag, resp.Bytes()), nil
}

// handleSymlink processes TSYMLINK: create a symbolic link.
func (s *Server) handleSymlink(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	target := buf.ReadString()
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	isAbs := len(target) > 0 && target[0] == '/'
	targetParts := splitPath(target)
	now := time.Now()
	if err := fid.handle.Symlink(ctx, true, name, targetParts, isAbs, now); err != nil {
		return nil, err
	}

	resp := NewWriteBuffer(13)
	resp.WriteQID(QID{Type: QidSymlink, Version: 0, Path: s.fids.qidPath.Add(1)})
	return buildMessage(RSYMLINK, tag, resp.Bytes()), nil
}

// handleReadlink processes TREADLINK: read a symbolic link.
func (s *Server) handleReadlink(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	parts, isAbs, err := fid.handle.Readlink(ctx, "")
	if err != nil {
		return nil, err
	}

	target := strings.Join(parts, "/")
	if isAbs {
		target = "/" + target
	}

	resp := NewWriteBuffer(2 + len(target))
	resp.WriteString(target)
	return buildMessage(RREADLINK, tag, resp.Bytes()), nil
}

// handleUnlinkat processes TUNLINKAT: remove a directory entry.
func (s *Server) handleUnlinkat(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	_ = buf.ReadU32() // flags
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := fid.handle.Remove(ctx, []string{name}, now); err != nil {
		return nil, err
	}

	return buildMessage(RUNLINKAT, tag, nil), nil
}

// handleRenameat processes TRENAMEAT: rename/move an entry.
func (s *Server) handleRenameat(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	oldDirFidID := buf.ReadU32()
	oldName := buf.ReadString()
	newDirFidID := buf.ReadU32()
	newName := buf.ReadString()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	oldDirFid, err := s.fids.Get(oldDirFidID)
	if err != nil {
		return nil, err
	}

	newDirFid, err := s.fids.Get(newDirFidID)
	if err != nil {
		return nil, err
	}

	src, err := oldDirFid.handle.Lookup(ctx, oldName)
	if err != nil {
		return nil, err
	}
	defer src.Release()

	now := time.Now()
	if err := src.Rename(ctx, newDirFid.handle, newName, now); err != nil {
		return nil, err
	}

	return buildMessage(RRENAMEAT, tag, nil), nil
}

// handleMknod processes TMKNOD: create a file node.
func (s *Server) handleMknod(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	mode := buf.ReadU32()
	_ = buf.ReadU32() // major
	_ = buf.ReadU32() // minor
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	perms := fs.FileMode(mode & 0o777)
	now := time.Now()
	if err := fid.handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), perms, now); err != nil {
		return nil, err
	}

	resp := NewWriteBuffer(13)
	resp.WriteQID(QID{Type: QidFile, Version: 0, Path: s.fids.qidPath.Add(1)})
	return buildMessage(RMKNOD, tag, resp.Bytes()), nil
}

// handleLink processes TLINK: hard link (stub: ENOTSUP).
func (s *Server) handleLink(tag uint16, _ []byte) ([]byte, error) {
	return buildErrorResponse(tag, ENOTSUP), nil
}

// handleFsync processes TFSYNC: no-op.
func (s *Server) handleFsync(tag uint16, _ []byte) ([]byte, error) {
	return buildMessage(RFSYNC, tag, nil), nil
}

// handleLock processes TLOCK: stub (always succeed).
func (s *Server) handleLock(tag uint16, _ []byte) ([]byte, error) {
	resp := NewWriteBuffer(1)
	resp.WriteU8(LockSuccess)
	return buildMessage(RLOCK, tag, resp.Bytes()), nil
}

// handleGetlock processes TGETLOCK: stub (always unlocked).
func (s *Server) handleGetlock(tag uint16, payload []byte) ([]byte, error) {
	buf := NewReadBuffer(payload)
	_ = buf.ReadU32() // fid
	typ := buf.ReadU8()
	start := buf.ReadU64()
	length := buf.ReadU64()
	procID := buf.ReadU32()
	clientID := buf.ReadString()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	resp := NewWriteBuffer(32)
	resp.WriteU8(typ)
	resp.WriteU64(start)
	resp.WriteU64(length)
	resp.WriteU32(procID)
	resp.WriteString(clientID)
	return buildMessage(RGETLOCK, tag, resp.Bytes()), nil
}

// handleStatfs processes TSTATFS: return hardcoded generous values.
func (s *Server) handleStatfs(tag uint16, _ []byte) ([]byte, error) {
	const tb = 1024 * 1024 * 1024 * 1024 // 1 TB
	const blockSize = 4096
	totalBlocks := uint64(tb / blockSize)

	resp := NewWriteBuffer(60)
	resp.WriteU32(0x01021997)  // type: V9FS_MAGIC
	resp.WriteU32(blockSize)   // bsize
	resp.WriteU64(totalBlocks) // blocks
	resp.WriteU64(totalBlocks) // bfree
	resp.WriteU64(totalBlocks) // bavail
	resp.WriteU64(totalBlocks) // files
	resp.WriteU64(totalBlocks) // ffree
	resp.WriteU64(0)           // fsid
	resp.WriteU32(256)         // namelen
	return buildMessage(RSTATFS, tag, resp.Bytes()), nil
}

// handleXattrwalk processes TXATTRWALK: stub (ENOTSUP).
func (s *Server) handleXattrwalk(tag uint16, _ []byte) ([]byte, error) {
	return buildErrorResponse(tag, ENOTSUP), nil
}

// handleXattrcreate processes TXATTRCREATE: stub (ENOTSUP).
func (s *Server) handleXattrcreate(tag uint16, _ []byte) ([]byte, error) {
	return buildErrorResponse(tag, ENOTSUP), nil
}

// handleFlush processes TFLUSH: stub (return RFLUSH immediately).
func (s *Server) handleFlush(tag uint16, _ []byte) ([]byte, error) {
	return buildMessage(RFLUSH, tag, nil), nil
}

// qidTypeForHandle returns the QID type byte for an FSHandle.
func qidTypeForHandle(ctx context.Context, h *unixfs.FSHandle) uint8 {
	nt, err := h.GetNodeType(ctx)
	if err != nil {
		return QidFile
	}
	return qidTypeFromNodeType(nt)
}

// qidTypeFromNodeType converts a FSCursorNodeType to a QID type byte.
func qidTypeFromNodeType(nt unixfs.FSCursorNodeType) uint8 {
	if nt.GetIsDirectory() {
		return QidDir
	}
	if nt.GetIsSymlink() {
		return QidSymlink
	}
	return QidFile
}

// splitPath splits a path string into components, removing empty parts.
func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" && p != "." {
			result = append(result, p)
		}
	}
	return result
}
