package plan9fs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
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
	// Decode the version negotiation request.
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

	// Encode the negotiated message size and protocol version.
	resp := NewWriteBuffer(32)
	resp.WriteU32(s.msize)
	resp.WriteString(respVersion)
	return buildMessage(RVERSION, tag, resp.Bytes()), nil
}

// handleAttach processes TATTACH: attach to the root filesystem.
func (s *Server) handleAttach(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the root attachment request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU32()    // afid (unused, no auth)
	_ = buf.ReadString() // uname
	_ = buf.ReadString() // aname
	uid := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Clone the root filesystem handle for the new attachment.
	handle, err := s.root.Clone(ctx)
	if err != nil {
		return nil, err
	}

	// Build the root fid with the requested user identifier.
	fid := &Fid{
		id:     fidID,
		handle: handle,
		uid:    uid,
	}

	// Register the attachment and release its handle on conflict.
	if err := s.fids.Add(fidID, fid); err != nil {
		handle.Release()
		return nil, err
	}

	// Allocate the root attachment QID.
	qidPath := s.fids.AllocQIDPath(handle)
	qid := QID{Type: QidDir, Version: 0, Path: qidPath}

	// Encode the attachment QID response.
	resp := NewWriteBuffer(13)
	resp.WriteQID(qid)
	return buildMessage(RATTACH, tag, resp.Bytes()), nil
}

// handleWalk processes TWALK: walk to a named child, component by component.
func (s *Server) handleWalk(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the walk identifiers and component count.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	newFidID := buf.ReadU32()
	nwname := buf.ReadU16()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Reject a walk exceeding the protocol component limit.
	if nwname > maxWalkNames {
		return nil, errTooManyNames
	}

	// Decode the path components before walking the filesystem.
	names := make([]string, nwname)
	for i := range names {
		names[i] = buf.ReadString()
	}
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the source fid for the walk.
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

	// Build the destination fid at the last successful walk component.
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

	// Register the destination fid and release its handle on conflict.
	if err := s.fids.Add(newFidID, newFid); err != nil {
		handle.Release()
		return nil, err
	}

	// Encode the QIDs collected by the walk.
	resp := NewWriteBuffer(2 + len(qids)*13)
	resp.WriteU16(uint16(len(qids))) //nolint:gosec
	for _, q := range qids {
		resp.WriteQID(q)
	}
	return buildMessage(RWALK, tag, resp.Bytes()), nil
}

// handleLopen processes TLOPEN: mark a fid as open.
func (s *Server) handleLopen(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the file open request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU32() // flags (ignored per design)
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the requested fid and mark it open.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}
	fid.opened = true

	// Allocate the opened handle QID.
	qidType := qidTypeForHandle(ctx, fid.handle)
	qidPath := s.fids.AllocQIDPath(fid.handle)
	qid := QID{Type: qidType, Version: 0, Path: qidPath}

	// iounit: 0 means no limit (use msize - headerSize - 4 for read/write header)
	iounit := s.msize - headerSize - 4

	// Encode the opened handle QID and I/O limit.
	resp := NewWriteBuffer(17)
	resp.WriteQID(qid)
	resp.WriteU32(iounit)
	return buildMessage(RLOPEN, tag, resp.Bytes()), nil
}

// handleLcreate processes TLCREATE: create and open a file.
func (s *Server) handleLcreate(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the file creation request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	_ = buf.ReadU32() // flags (ignored)
	mode := buf.ReadU32()
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the directory fid for file creation.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Prepare the requested permissions and creation timestamp.
	perms := fs.FileMode(mode & 0o777)
	now := time.Now()

	// Create the file under the directory fid.
	if err := fid.handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), perms, now); err != nil {
		return nil, err
	}

	// Look up the newly created file handle.
	child, err := fid.handle.Lookup(ctx, name)
	if err != nil {
		return nil, err
	}

	// Replace the directory handle with the opened file handle.
	fid.handle.Release()
	fid.handle = child
	fid.opened = true

	// Allocate the created file QID and I/O limit.
	qidPath := s.fids.AllocQIDPath(child)
	qid := QID{Type: QidFile, Version: 0, Path: qidPath}
	iounit := s.msize - headerSize - 4

	// Encode the created file QID response.
	resp := NewWriteBuffer(17)
	resp.WriteQID(qid)
	resp.WriteU32(iounit)
	return buildMessage(RLCREATE, tag, resp.Bytes()), nil
}

// handleRead processes TREAD: read data from an open fid.
func (s *Server) handleRead(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the file read request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	offset := buf.ReadU64()
	count := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the fid for the file read.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Bound the read count to the negotiated response size.
	maxData := s.msize - headerSize - 4
	if count > maxData {
		count = maxData
	}

	// check if offset is past file size
	fsize, sizeErr := fid.handle.GetSize(ctx)
	if sizeErr == nil && int64(offset) >= int64(fsize) { //nolint:gosec
		resp := NewWriteBuffer(4)
		resp.WriteU32(0)
		return buildMessage(RREAD, tag, resp.Bytes()), nil
	}

	// Read the file bytes and translate end-of-file into an empty read.
	data := make([]byte, count)
	n, readErr := fid.handle.ReadAt(ctx, int64(offset), data) //nolint:gosec
	if readErr != nil && n == 0 {
		// EOF at offset is normal in 9p — return empty read.
		if isEOF(readErr) {
			n = 0
		} else {
			return nil, readErr
		}
	}

	// Encode the returned file bytes and their count.
	resp := NewWriteBuffer(4 + int(n))
	if n > math.MaxUint32 {
		return nil, errors.New("read count exceeds uint32")
	}
	resp.WriteU32(uint32(n)) // #nosec G115 -- bounded above.
	resp.WriteBytes(data[:n])
	return buildMessage(RREAD, tag, resp.Bytes()), nil
}

// handleWrite processes TWRITE: write data to an open fid.
func (s *Server) handleWrite(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the file write header.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	offset := buf.ReadU64()
	count := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Bound the write count to the negotiated message size.
	maxData := s.msize - headerSize - 4
	if count > maxData {
		count = maxData
	}

	// Decode the bounded write payload.
	data := buf.ReadBytes(int(count))
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the fid for the file write.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Write the payload at the requested file offset.
	now := time.Now()
	if err := fid.handle.WriteAt(ctx, int64(offset), data, now); err != nil { //nolint:gosec
		return nil, err
	}

	// Encode the written byte count.
	resp := NewWriteBuffer(4)
	resp.WriteU32(count)
	return buildMessage(RWRITE, tag, resp.Bytes()), nil
}

// handleClunk processes TCLUNK: release a fid.
func (s *Server) handleClunk(tag uint16, payload []byte) ([]byte, error) {
	// Decode the fid release request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Detach the fid and release its filesystem handle.
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
	// Decode the deprecated remove request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Release the removed fid before returning the unsupported-operation error.
	fid, err := s.fids.Remove(fidID)
	if err != nil {
		return nil, err
	}
	fid.handle.Release()

	return nil, errUnsupported
}

// handleGetattr processes TGETATTR: get file attributes.
func (s *Server) handleGetattr(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the attribute lookup request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU64() // request_mask
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the fid whose attributes are requested.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Read the node type for the attribute response.
	nodeType, err := fid.handle.GetNodeType(ctx)
	if err != nil {
		return nil, err
	}

	// Read the file size with zero for nodes that do not support it.
	size, err := fid.handle.GetSize(ctx)
	if err != nil {
		// non-file nodes may not support size
		size = 0
	}

	// Read permissions with the directory default on failure.
	perms, err := fid.handle.GetPermissions(ctx)
	if err != nil {
		perms = 0o755
	}

	// Read the modification timestamp with a zero-time fallback.
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

	// Allocate the attribute response QID.
	qidType := qidTypeFromNodeType(nodeType)
	qidPath := s.fids.AllocQIDPath(fid.handle)
	qid := QID{Type: qidType, Version: 0, Path: qidPath}

	// Convert the modification timestamp into protocol seconds and nanoseconds.
	mtimeSec := uint64(mtime.Unix())        //nolint:gosec
	mtimeNsec := uint64(mtime.Nanosecond()) //nolint:gosec

	// blocks = ceil(size / 512)
	blocks := size / 512
	if size%512 != 0 {
		blocks++
	}

	// Encode the attribute validity, identity, and link metadata.
	resp := NewWriteBuffer(160)
	resp.WriteU64(GetattrBasic) // valid mask
	resp.WriteQID(qid)
	resp.WriteU32(mode)    // mode
	resp.WriteU32(fid.uid) // uid
	resp.WriteU32(fid.uid) // gid
	resp.WriteU64(1)       // nlink
	resp.WriteU64(0)       // rdev

	// Encode the file size and allocated blocks.
	resp.WriteU64(size)   // size
	resp.WriteU64(4096)   // blksize
	resp.WriteU64(blocks) // blocks

	// Encode the access, modification, and change timestamps.
	resp.WriteU64(mtimeSec)  // atime_sec
	resp.WriteU64(mtimeNsec) // atime_nsec
	resp.WriteU64(mtimeSec)  // mtime_sec
	resp.WriteU64(mtimeNsec) // mtime_nsec
	resp.WriteU64(mtimeSec)  // ctime_sec
	resp.WriteU64(mtimeNsec) // ctime_nsec

	// Encode unavailable birth, generation, and data-version metadata.
	resp.WriteU64(0) // btime_sec
	resp.WriteU64(0) // btime_nsec
	resp.WriteU64(0) // gen
	resp.WriteU64(0) // data_version
	return buildMessage(RGETATTR, tag, resp.Bytes()), nil
}

// handleSetattr processes TSETATTR: set file attributes.
func (s *Server) handleSetattr(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the attribute update mask and identity fields.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	valid := buf.ReadU32()
	mode := buf.ReadU32()
	_ = buf.ReadU32() // uid
	_ = buf.ReadU32() // gid

	// Decode the requested size and timestamps.
	size := buf.ReadU64()
	atimeSec := buf.ReadU64()
	atimeNsec := buf.ReadU64()
	_ = atimeSec
	_ = atimeNsec
	mtimeSec := buf.ReadU64()
	mtimeNsec := buf.ReadU64()

	// Reject an incomplete attribute update payload.
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the fid to receive attribute updates.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Apply the requested permission update.
	if valid&SetattrMode != 0 {
		perms := fs.FileMode(mode & 0o777)
		now := time.Now()
		if err := fid.handle.SetPermissions(ctx, perms, now); err != nil {
			return nil, err
		}
	}

	// Apply the requested file size update.
	if valid&SetattrSize != 0 {
		now := time.Now()
		if err := fid.handle.Truncate(ctx, size, now); err != nil {
			return nil, err
		}
	}

	// Apply the requested explicit or current modification timestamp.
	if valid&SetattrMtimeSet != 0 {
		mtime := time.Unix(int64(mtimeSec), int64(mtimeNsec)) //nolint:gosec
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
	// Decode the directory read request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	offset := buf.ReadU64()
	count := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the fid whose directory entries are requested.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// serialize all entries, then slice to offset
	var entries []byte
	var entryIndex uint64
	err = fid.handle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		// Assign the directory entry index and QID type.
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

	// Truncate directory results at a complete entry boundary.
	if uint32(len(result)) > count { //nolint:gosec
		result = truncateEntries(result, count)
	}

	// Encode the serialized directory entries.
	resp := NewWriteBuffer(4 + len(result))
	resp.WriteU32(uint32(len(result))) //nolint:gosec
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
		if uint32(r.off) > maxBytes { //nolint:gosec
			break
		}
		lastEnd = r.off
	}
	return entries[:lastEnd]
}

// handleMkdir processes TMKDIR: create a directory.
func (s *Server) handleMkdir(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the directory creation request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	mode := buf.ReadU32()
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the parent fid for directory creation.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Create the directory with the requested permissions.
	perms := fs.FileMode(mode & 0o777)
	now := time.Now()
	if err := fid.handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_Dir(), perms, now); err != nil {
		return nil, err
	}

	// Encode the new directory QID.
	resp := NewWriteBuffer(13)
	resp.WriteQID(QID{Type: QidDir, Version: 0, Path: s.fids.qidPath.Add(1)})
	return buildMessage(RMKDIR, tag, resp.Bytes()), nil
}

// handleSymlink processes TSYMLINK: create a symbolic link.
func (s *Server) handleSymlink(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the symbolic link creation request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	target := buf.ReadString()
	_ = buf.ReadU32() // gid
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the parent fid for symbolic link creation.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Create the symbolic link with its absolute or relative target.
	isAbs := len(target) > 0 && target[0] == '/'
	targetParts := splitPath(target)
	now := time.Now()
	if err := fid.handle.Symlink(ctx, true, name, targetParts, isAbs, now); err != nil {
		return nil, err
	}

	// Encode the new symbolic link QID.
	resp := NewWriteBuffer(13)
	resp.WriteQID(QID{Type: QidSymlink, Version: 0, Path: s.fids.qidPath.Add(1)})
	return buildMessage(RSYMLINK, tag, resp.Bytes()), nil
}

// handleReadlink processes TREADLINK: read a symbolic link.
func (s *Server) handleReadlink(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the symbolic link read request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the symbolic link fid.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Read the symbolic link target components.
	parts, isAbs, err := fid.handle.Readlink(ctx, "")
	if err != nil {
		return nil, err
	}

	// Reconstruct the absolute or relative target path.
	target := strings.Join(parts, "/")
	if isAbs {
		target = "/" + target
	}

	// Encode the symbolic link target path.
	resp := NewWriteBuffer(2 + len(target))
	resp.WriteString(target)
	return buildMessage(RREADLINK, tag, resp.Bytes()), nil
}

// handleUnlinkat processes TUNLINKAT: remove a directory entry.
func (s *Server) handleUnlinkat(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the directory entry removal request.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	name := buf.ReadString()
	_ = buf.ReadU32() // flags
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the parent fid for directory entry removal.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Remove the named directory entry.
	now := time.Now()
	if err := fid.handle.Remove(ctx, []string{name}, now); err != nil {
		return nil, err
	}

	return buildMessage(RUNLINKAT, tag, nil), nil
}

// handleRenameat processes TRENAMEAT: rename/move an entry.
func (s *Server) handleRenameat(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the source and destination rename locations.
	buf := NewReadBuffer(payload)
	oldDirFidID := buf.ReadU32()
	oldName := buf.ReadString()
	newDirFidID := buf.ReadU32()
	newName := buf.ReadString()
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Find the source directory fid for the rename.
	oldDirFid, err := s.fids.Get(oldDirFidID)
	if err != nil {
		return nil, err
	}

	// Find the destination directory fid for the rename.
	newDirFid, err := s.fids.Get(newDirFidID)
	if err != nil {
		return nil, err
	}

	// Open the source entry for the rename.
	src, err := oldDirFid.handle.Lookup(ctx, oldName)
	if err != nil {
		return nil, err
	}
	defer src.Release()

	// Rename the source into the destination directory.
	now := time.Now()
	if err := src.Rename(ctx, newDirFid.handle, newName, now); err != nil {
		return nil, err
	}

	return buildMessage(RRENAMEAT, tag, nil), nil
}

// handleMknod processes TMKNOD: create a file node.
func (s *Server) handleMknod(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Decode the file node creation request.
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

	// Find the parent fid for file node creation.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}

	// Create the file node with the requested permissions.
	perms := fs.FileMode(mode & 0o777)
	now := time.Now()
	if err := fid.handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), perms, now); err != nil {
		return nil, err
	}

	// Encode the new file node QID.
	resp := NewWriteBuffer(13)
	resp.WriteQID(QID{Type: QidFile, Version: 0, Path: s.fids.qidPath.Add(1)})
	return buildMessage(RMKNOD, tag, resp.Bytes()), nil
}

// handleLink processes TLINK: hard link (stub: ENOTSUP).
func (s *Server) handleLink(tag uint16, _ []byte) ([]byte, error) {
	return buildErrorResponse(tag, ENOTSUP), nil
}

// handleFsync processes TFSYNC: commit the buffered writes of the fid's tree.
func (s *Server) handleFsync(ctx context.Context, tag uint16, payload []byte) ([]byte, error) {
	// Parse the fid; datasync commits the same writes.
	buf := NewReadBuffer(payload)
	fidID := buf.ReadU32()
	_ = buf.ReadU32() // datasync
	if buf.Err() != nil {
		return nil, buf.Err()
	}

	// Commit the buffered writes.
	fid, err := s.fids.Get(fidID)
	if err != nil {
		return nil, err
	}
	if err := fid.handle.Sync(ctx); err != nil {
		return nil, err
	}
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
	// Decode the lock query range and client identity.
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

	// Encode the lock query response using the supplied range and identity.
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
	// Choose the filesystem capacity and block size reported to clients.
	const tb = 1024 * 1024 * 1024 * 1024 // 1 TB
	const blockSize = 4096
	totalBlocks := uint64(tb / blockSize)

	// Encode the filesystem type and block capacity.
	resp := NewWriteBuffer(60)
	resp.WriteU32(0x01021997)  // type: V9FS_MAGIC
	resp.WriteU32(blockSize)   // bsize
	resp.WriteU64(totalBlocks) // blocks
	resp.WriteU64(totalBlocks) // bfree
	resp.WriteU64(totalBlocks) // bavail

	// Encode the file capacity and filesystem identifier limits.
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
	// Normalize the path into nonempty components without dot entries.
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
