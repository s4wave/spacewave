package unixfs_v86fs

import (
	"encoding/binary"
	"math"

	"github.com/pkg/errors"
)

const (
	v86fsFrameHeaderSize = 7

	v86fsMsgMount         = 0x00
	v86fsMsgLookup        = 0x01
	v86fsMsgGetattr       = 0x02
	v86fsMsgReaddir       = 0x03
	v86fsMsgOpen          = 0x04
	v86fsMsgClose         = 0x05
	v86fsMsgRead          = 0x06
	v86fsMsgCreate        = 0x07
	v86fsMsgWrite         = 0x08
	v86fsMsgMkdir         = 0x09
	v86fsMsgSetattr       = 0x0a
	v86fsMsgFsync         = 0x0b
	v86fsMsgUnlink        = 0x0c
	v86fsMsgRename        = 0x0d
	v86fsMsgSymlink       = 0x0e
	v86fsMsgReadlink      = 0x0f
	v86fsMsgStatfs        = 0x10
	v86fsMsgInvalidate    = 0x20
	v86fsMsgInvalidateDir = 0x21
	v86fsMsgMountNotify   = 0x22
	v86fsMsgUmountNotify  = 0x23

	v86fsMsgMountReply    = 0x80
	v86fsMsgLookupReply   = 0x81
	v86fsMsgGetattrReply  = 0x82
	v86fsMsgReaddirReply  = 0x83
	v86fsMsgOpenReply     = 0x84
	v86fsMsgCloseReply    = 0x85
	v86fsMsgReadReply     = 0x86
	v86fsMsgCreateReply   = 0x87
	v86fsMsgWriteReply    = 0x88
	v86fsMsgMkdirReply    = 0x89
	v86fsMsgSetattrReply  = 0x8a
	v86fsMsgFsyncReply    = 0x8b
	v86fsMsgUnlinkReply   = 0x8c
	v86fsMsgRenameReply   = 0x8d
	v86fsMsgSymlinkReply  = 0x8e
	v86fsMsgReadlinkReply = 0x8f
	v86fsMsgStatfsReply   = 0x90
	v86fsMsgErrorReply    = 0xff
)

// DecodeBinaryFrame converts the guest v86fs binary protocol into the typed
// relay message used by the UnixFS-backed server.
func DecodeBinaryFrame(frame []byte) (*V86FsMessage, error) {
	frame, typ, tag, err := parseBinaryFrameHeader(frame)
	if err != nil {
		return nil, err
	}
	r := binaryFrameReader{data: frame[v86fsFrameHeaderSize:]}
	msg := &V86FsMessage{Tag: uint32(tag)}

	switch typ {
	case v86fsMsgMount:
		name, err := r.string()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: name}}
	case v86fsMsgLookup:
		parentID, err := r.u64()
		if err != nil {
			return nil, err
		}
		name, err := r.string()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{ParentId: parentID, Name: name}}
	case v86fsMsgGetattr:
		inodeID, err := r.u64()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_GetattrRequest{GetattrRequest: &V86FsGetattrRequest{InodeId: inodeID}}
	case v86fsMsgReaddir:
		dirID, err := r.u64()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_ReaddirRequest{ReaddirRequest: &V86FsReaddirRequest{DirId: dirID}}
	case v86fsMsgOpen:
		inodeID, err := r.u64()
		if err != nil {
			return nil, err
		}
		flags, err := r.u32()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_OpenRequest{OpenRequest: &V86FsOpenRequest{InodeId: inodeID, Flags: flags}}
	case v86fsMsgClose:
		handleID, err := r.u64()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_CloseRequest{CloseRequest: &V86FsCloseRequest{HandleId: handleID}}
	case v86fsMsgRead:
		handleID, err := r.u64()
		if err != nil {
			return nil, err
		}
		offset, err := r.u64()
		if err != nil {
			return nil, err
		}
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_ReadRequest{ReadRequest: &V86FsReadRequest{HandleId: handleID, Offset: offset, Size: size}}
	case v86fsMsgCreate:
		parentID, name, mode, err := r.parentNameMode()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_CreateRequest{CreateRequest: &V86FsCreateRequest{ParentId: parentID, Name: name, Mode: mode}}
	case v86fsMsgWrite:
		inodeID, err := r.u64()
		if err != nil {
			return nil, err
		}
		offset, err := r.u64()
		if err != nil {
			return nil, err
		}
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		data, err := r.bytes(int(size))
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_WriteRequest{WriteRequest: &V86FsWriteRequest{InodeId: inodeID, Offset: offset, Data: data}}
	case v86fsMsgMkdir:
		parentID, name, mode, err := r.parentNameMode()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_MkdirRequest{MkdirRequest: &V86FsMkdirRequest{ParentId: parentID, Name: name, Mode: mode}}
	case v86fsMsgSetattr:
		inodeID, err := r.u64()
		if err != nil {
			return nil, err
		}
		valid, err := r.u32()
		if err != nil {
			return nil, err
		}
		mode, err := r.u32()
		if err != nil {
			return nil, err
		}
		size, err := r.u64()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_SetattrRequest{SetattrRequest: &V86FsSetattrRequest{InodeId: inodeID, Valid: valid, Mode: mode, Size: size}}
	case v86fsMsgFsync:
		inodeID, err := r.u64()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_FsyncRequest{FsyncRequest: &V86FsFsyncRequest{InodeId: inodeID}}
	case v86fsMsgUnlink:
		parentID, err := r.u64()
		if err != nil {
			return nil, err
		}
		name, err := r.string()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_UnlinkRequest{UnlinkRequest: &V86FsUnlinkRequest{ParentId: parentID, Name: name}}
	case v86fsMsgRename:
		oldParentID, err := r.u64()
		if err != nil {
			return nil, err
		}
		oldName, err := r.string()
		if err != nil {
			return nil, err
		}
		newParentID, err := r.u64()
		if err != nil {
			return nil, err
		}
		newName, err := r.string()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_RenameRequest{RenameRequest: &V86FsRenameRequest{
			OldParentId: oldParentID,
			OldName:     oldName,
			NewParentId: newParentID,
			NewName:     newName,
		}}
	case v86fsMsgSymlink:
		parentID, err := r.u64()
		if err != nil {
			return nil, err
		}
		name, err := r.string()
		if err != nil {
			return nil, err
		}
		target, err := r.string()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_SymlinkRequest{SymlinkRequest: &V86FsSymlinkRequest{ParentId: parentID, Name: name, Target: target}}
	case v86fsMsgReadlink:
		inodeID, err := r.u64()
		if err != nil {
			return nil, err
		}
		msg.Body = &V86FsMessage_ReadlinkRequest{ReadlinkRequest: &V86FsReadlinkRequest{InodeId: inodeID}}
	case v86fsMsgStatfs:
		msg.Body = &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}}
	default:
		return nil, errors.Errorf("unknown v86fs binary message type %#x", typ)
	}
	if r.remaining() != 0 {
		return nil, errors.Errorf("v86fs binary message type %#x has %d trailing bytes", typ, r.remaining())
	}
	return msg, nil
}

// EncodeBinaryFrame converts a typed relay reply or notification into the
// compact guest v86fs binary protocol.
func EncodeBinaryFrame(msg *V86FsMessage) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("nil v86fs message")
	}
	tag := uint16(msg.GetTag())
	switch body := msg.GetBody().(type) {
	case *V86FsMessage_MountReply:
		r := body.MountReply
		w := newBinaryFrameWriter(v86fsMsgMountReply, tag, 16)
		w.u32(r.GetStatus())
		w.u64(r.GetRootInodeId())
		w.u32(r.GetMode())
		return w.bytes(), nil
	case *V86FsMessage_LookupReply:
		r := body.LookupReply
		w := newBinaryFrameWriter(v86fsMsgLookupReply, tag, 24)
		w.u32(r.GetStatus())
		w.u64(r.GetInodeId())
		w.u32(r.GetMode())
		w.u64(r.GetSize())
		return w.bytes(), nil
	case *V86FsMessage_GetattrReply:
		r := body.GetattrReply
		w := newBinaryFrameWriter(v86fsMsgGetattrReply, tag, 28)
		w.u32(r.GetStatus())
		w.u32(r.GetMode())
		w.u64(r.GetSize())
		w.u64(uint64(r.GetMtimeSec()))
		w.u32(r.GetMtimeNsec())
		return w.bytes(), nil
	case *V86FsMessage_ReaddirReply:
		return encodeReaddirReply(tag, body.ReaddirReply)
	case *V86FsMessage_OpenReply:
		r := body.OpenReply
		w := newBinaryFrameWriter(v86fsMsgOpenReply, tag, 12)
		w.u32(r.GetStatus())
		w.u64(r.GetHandleId())
		return w.bytes(), nil
	case *V86FsMessage_CloseReply:
		return encodeStatusReply(v86fsMsgCloseReply, tag, body.CloseReply.GetStatus()), nil
	case *V86FsMessage_ReadReply:
		r := body.ReadReply
		w := newBinaryFrameWriter(v86fsMsgReadReply, tag, 8+len(r.GetData()))
		w.u32(r.GetStatus())
		w.u32(uint32(len(r.GetData())))
		w.raw(r.GetData())
		return w.bytes(), nil
	case *V86FsMessage_CreateReply:
		return encodeInodeModeReply(v86fsMsgCreateReply, tag, body.CreateReply.GetStatus(), body.CreateReply.GetInodeId(), body.CreateReply.GetMode()), nil
	case *V86FsMessage_WriteReply:
		r := body.WriteReply
		w := newBinaryFrameWriter(v86fsMsgWriteReply, tag, 8)
		w.u32(r.GetStatus())
		w.u32(r.GetBytesWritten())
		return w.bytes(), nil
	case *V86FsMessage_MkdirReply:
		return encodeInodeModeReply(v86fsMsgMkdirReply, tag, body.MkdirReply.GetStatus(), body.MkdirReply.GetInodeId(), body.MkdirReply.GetMode()), nil
	case *V86FsMessage_SetattrReply:
		return encodeStatusReply(v86fsMsgSetattrReply, tag, body.SetattrReply.GetStatus()), nil
	case *V86FsMessage_FsyncReply:
		return encodeStatusReply(v86fsMsgFsyncReply, tag, body.FsyncReply.GetStatus()), nil
	case *V86FsMessage_UnlinkReply:
		return encodeStatusReply(v86fsMsgUnlinkReply, tag, body.UnlinkReply.GetStatus()), nil
	case *V86FsMessage_RenameReply:
		return encodeStatusReply(v86fsMsgRenameReply, tag, body.RenameReply.GetStatus()), nil
	case *V86FsMessage_SymlinkReply:
		return encodeInodeModeReply(v86fsMsgSymlinkReply, tag, body.SymlinkReply.GetStatus(), body.SymlinkReply.GetInodeId(), body.SymlinkReply.GetMode()), nil
	case *V86FsMessage_ReadlinkReply:
		r := body.ReadlinkReply
		w := newBinaryFrameWriter(v86fsMsgReadlinkReply, tag, 4+2+len(r.GetTarget()))
		w.u32(r.GetStatus())
		w.string(r.GetTarget())
		return w.bytes(), nil
	case *V86FsMessage_StatfsReply:
		r := body.StatfsReply
		w := newBinaryFrameWriter(v86fsMsgStatfsReply, tag, 48)
		w.u32(r.GetStatus())
		w.u64(r.GetBlocks())
		w.u64(r.GetBfree())
		w.u64(r.GetBavail())
		w.u64(r.GetFiles())
		w.u64(r.GetFfree())
		w.u32(r.GetBsize())
		return w.bytes(), nil
	case *V86FsMessage_Invalidate:
		r := body.Invalidate
		w := newBinaryFrameWriter(v86fsMsgInvalidate, 0, 24)
		w.u64(r.GetInodeId())
		w.u64(r.GetOffset())
		w.u64(r.GetSize())
		return w.bytes(), nil
	case *V86FsMessage_InvalidateDir:
		r := body.InvalidateDir
		w := newBinaryFrameWriter(v86fsMsgInvalidateDir, 0, 8)
		w.u64(r.GetDirId())
		return w.bytes(), nil
	case *V86FsMessage_MountNotify:
		r := body.MountNotify
		w := newBinaryFrameWriter(v86fsMsgMountNotify, 0, 2+len(r.GetName())+2+len(r.GetMountPath()))
		w.string(r.GetName())
		w.string(r.GetMountPath())
		return w.bytes(), nil
	case *V86FsMessage_UmountNotify:
		r := body.UmountNotify
		w := newBinaryFrameWriter(v86fsMsgUmountNotify, 0, 2+len(r.GetMountPath()))
		w.string(r.GetMountPath())
		return w.bytes(), nil
	case *V86FsMessage_ErrorReply:
		return encodeStatusReply(v86fsMsgErrorReply, tag, body.ErrorReply.GetStatus()), nil
	default:
		return nil, errors.Errorf("unsupported v86fs binary message body %T", body)
	}
}

type binaryFrameReader struct {
	data []byte
}

func (r *binaryFrameReader) remaining() int {
	return len(r.data)
}

func (r *binaryFrameReader) bytes(size int) ([]byte, error) {
	if size < 0 || len(r.data) < size {
		return nil, errors.New("short v86fs binary frame")
	}
	value := append([]byte(nil), r.data[:size]...)
	r.data = r.data[size:]
	return value, nil
}

func (r *binaryFrameReader) string() (string, error) {
	size, err := r.u16()
	if err != nil {
		return "", err
	}
	data, err := r.bytes(int(size))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *binaryFrameReader) u16() (uint16, error) {
	data, err := r.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(data), nil
}

func (r *binaryFrameReader) u32() (uint32, error) {
	data, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

func (r *binaryFrameReader) u64() (uint64, error) {
	data, err := r.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(data), nil
}

func (r *binaryFrameReader) parentNameMode() (uint64, string, uint32, error) {
	parentID, err := r.u64()
	if err != nil {
		return 0, "", 0, err
	}
	name, err := r.string()
	if err != nil {
		return 0, "", 0, err
	}
	mode, err := r.u32()
	if err != nil {
		return 0, "", 0, err
	}
	return parentID, name, mode, nil
}

type binaryFrameWriter struct {
	data []byte
}

func newBinaryFrameWriter(typ byte, tag uint16, payloadSize int) *binaryFrameWriter {
	w := &binaryFrameWriter{data: make([]byte, v86fsFrameHeaderSize, v86fsFrameHeaderSize+payloadSize)}
	binary.LittleEndian.PutUint32(w.data[:4], uint32(v86fsFrameHeaderSize+payloadSize))
	w.data[4] = typ
	binary.LittleEndian.PutUint16(w.data[5:7], tag)
	return w
}

func (w *binaryFrameWriter) bytes() []byte {
	binary.LittleEndian.PutUint32(w.data[:4], uint32(len(w.data)))
	return w.data
}

func (w *binaryFrameWriter) raw(data []byte) {
	w.data = append(w.data, data...)
}

func (w *binaryFrameWriter) string(value string) {
	if len(value) > math.MaxUint16 {
		panic("v86fs binary string exceeds uint16 length")
	}
	w.u16(uint16(len(value)))
	w.raw([]byte(value))
}

func (w *binaryFrameWriter) u16(value uint16) {
	w.data = binary.LittleEndian.AppendUint16(w.data, value)
}

func (w *binaryFrameWriter) u32(value uint32) {
	w.data = binary.LittleEndian.AppendUint32(w.data, value)
}

func (w *binaryFrameWriter) u64(value uint64) {
	w.data = binary.LittleEndian.AppendUint64(w.data, value)
}

func parseBinaryFrameHeader(frame []byte) ([]byte, byte, uint16, error) {
	if len(frame) < v86fsFrameHeaderSize {
		return nil, 0, 0, errors.New("short v86fs binary frame header")
	}
	size := binary.LittleEndian.Uint32(frame[:4])
	if uint64(size) != uint64(len(frame)) {
		return nil, 0, 0, errors.Errorf("v86fs binary frame length %d, want %d", size, len(frame))
	}
	return frame, frame[4], binary.LittleEndian.Uint16(frame[5:7]), nil
}

func encodeStatusReply(typ byte, tag uint16, status uint32) []byte {
	w := newBinaryFrameWriter(typ, tag, 4)
	w.u32(status)
	return w.bytes()
}

func encodeInodeModeReply(typ byte, tag uint16, status uint32, inodeID uint64, mode uint32) []byte {
	w := newBinaryFrameWriter(typ, tag, 16)
	w.u32(status)
	w.u64(inodeID)
	w.u32(mode)
	return w.bytes()
}

func encodeReaddirReply(tag uint16, reply *V86FsReaddirReply) ([]byte, error) {
	payloadSize := 8
	for _, ent := range reply.GetEntries() {
		if len(ent.GetName()) > math.MaxUint16 {
			return nil, errors.Errorf("v86fs dirent name too long: %q", ent.GetName())
		}
		payloadSize += 8 + 1 + 2 + len(ent.GetName())
	}
	w := newBinaryFrameWriter(v86fsMsgReaddirReply, tag, payloadSize)
	w.u32(reply.GetStatus())
	w.u32(uint32(len(reply.GetEntries())))
	for _, ent := range reply.GetEntries() {
		w.u64(ent.GetInodeId())
		w.raw([]byte{byte(ent.GetDtType())})
		w.string(ent.GetName())
	}
	return w.bytes(), nil
}
