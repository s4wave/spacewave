package unixfs_v86fs

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDecodeBinaryFrameRequests(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
		check func(t *testing.T, msg *V86FsMessage)
	}{
		{
			name:  "mount",
			frame: frame(v86fsMsgMount, 7, str("workspace")),
			check: func(t *testing.T, msg *V86FsMessage) {
				if got := msg.GetMountRequest().GetName(); got != "workspace" {
					t.Fatalf("mount name = %q", got)
				}
			},
		},
		{
			name:  "lookup",
			frame: frame(v86fsMsgLookup, 8, u64(42), str("hello.txt")),
			check: func(t *testing.T, msg *V86FsMessage) {
				req := msg.GetLookupRequest()
				if req.GetParentId() != 42 || req.GetName() != "hello.txt" {
					t.Fatalf("lookup = %#v", req)
				}
			},
		},
		{
			name:  "read",
			frame: frame(v86fsMsgRead, 9, u64(5), u64(12), u32(4096)),
			check: func(t *testing.T, msg *V86FsMessage) {
				req := msg.GetReadRequest()
				if req.GetHandleId() != 5 || req.GetOffset() != 12 || req.GetSize() != 4096 {
					t.Fatalf("read = %#v", req)
				}
			},
		},
		{
			name:  "write",
			frame: frame(v86fsMsgWrite, 10, u64(11), u64(13), u32(3), []byte("abc")),
			check: func(t *testing.T, msg *V86FsMessage) {
				req := msg.GetWriteRequest()
				if req.GetInodeId() != 11 || req.GetOffset() != 13 || !bytes.Equal(req.GetData(), []byte("abc")) {
					t.Fatalf("write = %#v", req)
				}
			},
		},
		{
			name:  "rename",
			frame: frame(v86fsMsgRename, 11, u64(1), str("old"), u64(2), str("new")),
			check: func(t *testing.T, msg *V86FsMessage) {
				req := msg.GetRenameRequest()
				if req.GetOldParentId() != 1 || req.GetOldName() != "old" || req.GetNewParentId() != 2 || req.GetNewName() != "new" {
					t.Fatalf("rename = %#v", req)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg, err := DecodeBinaryFrame(test.frame)
			if err != nil {
				t.Fatalf("decode frame: %v", err)
			}
			test.check(t, msg)
		})
	}
}

func TestEncodeBinaryFrameReplies(t *testing.T) {
	tests := []struct {
		name string
		msg  *V86FsMessage
		want []byte
	}{
		{
			name: "mount",
			msg: &V86FsMessage{
				Tag: 7,
				Body: &V86FsMessage_MountReply{MountReply: &V86FsMountReply{
					Status:      0,
					RootInodeId: 99,
					Mode:        0o40755,
				}},
			},
			want: frame(v86fsMsgMountReply, 7, u32(0), u64(99), u32(0o40755)),
		},
		{
			name: "read",
			msg: &V86FsMessage{
				Tag: 8,
				Body: &V86FsMessage_ReadReply{ReadReply: &V86FsReadReply{
					Status: 0,
					Data:   []byte("data"),
				}},
			},
			want: frame(v86fsMsgReadReply, 8, u32(0), u32(4), []byte("data")),
		},
		{
			name: "readdir",
			msg: &V86FsMessage{
				Tag: 9,
				Body: &V86FsMessage_ReaddirReply{ReaddirReply: &V86FsReaddirReply{
					Status: 0,
					Entries: []*V86FsDirEntry{{
						InodeId: 12,
						DtType:  dtReg,
						Name:    "file",
					}},
				}},
			},
			want: frame(v86fsMsgReaddirReply, 9, u32(0), u32(1), u64(12), []byte{dtReg}, str("file")),
		},
		{
			name: "mount notify",
			msg: &V86FsMessage{
				Body: &V86FsMessage_MountNotify{MountNotify: &V86FsMountNotify{
					Name:      "workspace",
					MountPath: "/mnt/workspace",
				}},
			},
			want: frame(v86fsMsgMountNotify, 0, str("workspace"), str("/mnt/workspace")),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EncodeBinaryFrame(test.msg)
			if err != nil {
				t.Fatalf("encode frame: %v", err)
			}
			if !bytes.Equal(got, test.want) {
				t.Fatalf("encoded frame\n got %x\nwant %x", got, test.want)
			}
		})
	}
}

func frame(typ byte, tag uint16, parts ...[]byte) []byte {
	size := v86fsFrameHeaderSize
	for _, part := range parts {
		size += len(part)
	}
	out := make([]byte, 0, size)
	out = binary.LittleEndian.AppendUint32(out, uint32(size))
	out = append(out, typ)
	out = binary.LittleEndian.AppendUint16(out, tag)
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func str(value string) []byte {
	out := make([]byte, 0, 2+len(value))
	out = binary.LittleEndian.AppendUint16(out, uint16(len(value)))
	out = append(out, value...)
	return out
}

func u32(value uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, value)
}

func u64(value uint64) []byte {
	return binary.LittleEndian.AppendUint64(nil, value)
}
