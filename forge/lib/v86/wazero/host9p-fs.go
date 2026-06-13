package v86_wazero

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

const (
	p9TStatFS   = 8
	p9RStatFS   = 9
	p9TLOpen    = 12
	p9RLOpen    = 13
	p9TReadLink = 22
	p9RReadLink = 23
	p9TGetAttr  = 24
	p9RGetAttr  = 25
	p9TReadDir  = 40
	p9RReadDir  = 41
	p9TVersion  = 100
	p9RVersion  = 101
	p9TAttach   = 104
	p9RAttach   = 105
	p9RError    = 107
	p9TFlush    = 108
	p9RFlush    = 109
	p9TWalk     = 110
	p9RWalk     = 111
	p9TRead     = 116
	p9RRead     = 117
	p9TClunk    = 120
	p9RClunk    = 121

	p9ENOENT     = 2
	p9EIO        = 5
	p9ENOTDIR    = 20
	p9EOPNOTSUPP = 95

	host9pQIDFile    = 0x00
	host9pQIDDir     = 0x80
	host9pQIDSymlink = 0x02

	host9pModeDir     = 0o040000
	host9pModeSymlink = 0o120000
	host9pModeRegular = 0o100000

	host9pDTypeDir     = 4
	host9pDTypeRegular = 8
	host9pDTypeSymlink = 10
)

// Host9PFS serves the Bun v86 fs.json + flat/ root image over 9P2000.L.
type Host9PFS struct {
	flatDir         string
	inodes          []*host9pInode
	fids            map[uint32]*host9pInode
	requests        atomic.Uint64
	lastType        atomic.Uint32
	notifies        atomic.Uint64
	availIdx        atomic.Uint32
	availLastIdx    atomic.Uint32
	queueConfigured atomic.Uint32
}

type host9pInode struct {
	ino      uint64
	name     string
	size     uint64
	mtime    uint64
	mode     uint32
	uid      uint32
	gid      uint32
	flatFile string
	symlink  string
	parent   *host9pInode
	children []*host9pInode
}

// OpenHost9PFS loads a v86 fs.json directory produced for Bun handle9p boot.
func OpenHost9PFS(dir string) (*Host9PFS, error) {
	data, err := os.ReadFile(filepath.Join(dir, "fs.json"))
	if err != nil {
		return nil, errors.Wrap(err, "read fs.json")
	}
	var parser fastjson.Parser
	root, err := parser.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse fs.json")
	}
	entries := root.GetArray("fsroot")
	if entries == nil {
		return nil, errors.New("fs.json missing fsroot array")
	}
	fs := &Host9PFS{
		flatDir: filepath.Join(dir, "flat"),
		fids:    make(map[uint32]*host9pInode),
	}
	rootInode := &host9pInode{
		ino:  0,
		name: "",
		mode: host9pModeDir | 0o755,
	}
	fs.inodes = append(fs.inodes, rootInode)
	if err := fs.loadChildren(rootInode, entries); err != nil {
		return nil, err
	}
	return fs, nil
}

func (fs *Host9PFS) loadChildren(parent *host9pInode, values []*fastjson.Value) error {
	for _, value := range values {
		fields := value.GetArray()
		if len(fields) < 6 {
			return errors.New("fs.json entry has fewer than 6 fields")
		}
		name := string(fields[0].GetStringBytes())
		inode := &host9pInode{
			ino:    uint64(len(fs.inodes)),
			name:   name,
			size:   fields[1].GetUint64(),
			mtime:  fields[2].GetUint64(),
			mode:   uint32(fields[3].GetUint()),
			uid:    uint32(fields[4].GetUint()),
			gid:    uint32(fields[5].GetUint()),
			parent: parent,
		}
		parent.children = append(parent.children, inode)
		fs.inodes = append(fs.inodes, inode)
		if len(fields) > 6 {
			switch fields[6].Type() {
			case fastjson.TypeArray:
				inode.mode = host9pModeDir | inode.mode&0o7777
				if err := fs.loadChildren(inode, fields[6].GetArray()); err != nil {
					return err
				}
			case fastjson.TypeString:
				data := string(fields[6].GetStringBytes())
				if strings.HasSuffix(data, ".bin") {
					inode.mode = host9pModeRegular | inode.mode&0o7777
					inode.flatFile = data
				} else {
					inode.mode = host9pModeSymlink | inode.mode&0o7777
					inode.symlink = data
					inode.size = uint64(len(data))
				}
			}
		}
	}
	return nil
}

func (fs *Host9PFS) Handle(req []byte) []byte {
	if len(req) < 7 {
		return nil
	}
	size := binary.LittleEndian.Uint32(req)
	if size > uint32(len(req)) {
		return p9Error(req[4], binary.LittleEndian.Uint16(req[5:]), p9EIO)
	}
	msgType := req[4]
	tag := binary.LittleEndian.Uint16(req[5:])
	body := req[7:size]
	fs.requests.Add(1)
	fs.lastType.Store(uint32(msgType))
	switch msgType {
	case p9TVersion:
		return fs.handleVersion(tag, body)
	case p9TAttach:
		return fs.handleAttach(tag, body)
	case p9TWalk:
		return fs.handleWalk(tag, body)
	case p9TLOpen:
		return fs.handleLOpen(tag, body)
	case p9TReadLink:
		return fs.handleReadLink(tag, body)
	case p9TGetAttr:
		return fs.handleGetAttr(tag, body)
	case p9TReadDir:
		return fs.handleReadDir(tag, body)
	case p9TRead:
		return fs.handleRead(tag, body)
	case p9TClunk:
		return fs.handleClunk(tag, body)
	case p9TStatFS:
		return fs.handleStatFS(tag)
	case p9TFlush:
		return p9Reply(p9RFlush, tag, nil)
	default:
		return p9Error(msgType, tag, p9EOPNOTSUPP)
	}
}

func (fs *Host9PFS) stats() (uint64, byte, uint64, uint32, uint32, bool) {
	if fs == nil {
		return 0, 0, 0, 0, 0, false
	}
	return fs.requests.Load(),
		byte(fs.lastType.Load()),
		fs.notifies.Load(),
		fs.availIdx.Load(),
		fs.availLastIdx.Load(),
		fs.queueConfigured.Load() != 0
}

func (fs *Host9PFS) handleVersion(tag uint16, body []byte) []byte {
	if len(body) < 4 {
		return p9Error(p9TVersion, tag, p9EIO)
	}
	var out []byte
	out = p9AppendU32(out, binary.LittleEndian.Uint32(body))
	out = p9AppendString(out, "9P2000.L")
	return p9Reply(p9RVersion, tag, out)
}

func (fs *Host9PFS) handleAttach(tag uint16, body []byte) []byte {
	if len(body) < 4 || len(fs.inodes) == 0 {
		return p9Error(p9TAttach, tag, p9EIO)
	}
	fs.fids[binary.LittleEndian.Uint32(body)] = fs.inodes[0]
	return p9Reply(p9RAttach, tag, fs.inodes[0].qid())
}

func (fs *Host9PFS) handleWalk(tag uint16, body []byte) []byte {
	if len(body) < 10 {
		return p9Error(p9TWalk, tag, p9EIO)
	}
	fid := binary.LittleEndian.Uint32(body)
	newfid := binary.LittleEndian.Uint32(body[4:])
	count := int(binary.LittleEndian.Uint16(body[8:]))
	cursor := p9Cursor{data: body[10:]}
	node := fs.fids[fid]
	if node == nil {
		return p9Error(p9TWalk, tag, p9ENOENT)
	}
	var qids []byte
	for range count {
		name, ok := cursor.string()
		if !ok {
			return p9Error(p9TWalk, tag, p9EIO)
		}
		if !node.isDir() {
			return p9Error(p9TWalk, tag, p9ENOTDIR)
		}
		next := node.child(name)
		if next == nil {
			return p9Error(p9TWalk, tag, p9ENOENT)
		}
		node = next
		qids = append(qids, node.qid()...)
	}
	fs.fids[newfid] = node
	var out []byte
	out = p9AppendU16(out, uint16(count))
	out = append(out, qids...)
	return p9Reply(p9RWalk, tag, out)
}

func (fs *Host9PFS) handleLOpen(tag uint16, body []byte) []byte {
	if len(body) < 4 {
		return p9Error(p9TLOpen, tag, p9EIO)
	}
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(p9TLOpen, tag, p9ENOENT)
	}
	out := append([]byte{}, node.qid()...)
	out = p9AppendU32(out, 65536)
	return p9Reply(p9RLOpen, tag, out)
}

func (fs *Host9PFS) handleReadLink(tag uint16, body []byte) []byte {
	if len(body) < 4 {
		return p9Error(p9TReadLink, tag, p9EIO)
	}
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(p9TReadLink, tag, p9ENOENT)
	}
	return p9Reply(p9RReadLink, tag, p9AppendString(nil, node.symlink))
}

func (fs *Host9PFS) handleGetAttr(tag uint16, body []byte) []byte {
	if len(body) < 4 {
		return p9Error(p9TGetAttr, tag, p9EIO)
	}
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(p9TGetAttr, tag, p9ENOENT)
	}
	var out []byte
	out = p9AppendU64(out, 0x7ff)
	out = append(out, node.qid()...)
	out = p9AppendU32(out, node.mode)
	out = p9AppendU32(out, node.uid)
	out = p9AppendU32(out, node.gid)
	out = p9AppendU64(out, 1)
	out = p9AppendU64(out, 0)
	out = p9AppendU64(out, node.size)
	out = p9AppendU64(out, 4096)
	out = p9AppendU64(out, (node.size+511)/512)
	for range 4 {
		out = p9AppendU64(out, node.mtime)
		out = p9AppendU64(out, 0)
	}
	out = p9AppendU64(out, 0)
	out = p9AppendU64(out, 0)
	return p9Reply(p9RGetAttr, tag, out)
}

func (fs *Host9PFS) handleReadDir(tag uint16, body []byte) []byte {
	if len(body) < 16 {
		return p9Error(p9TReadDir, tag, p9EIO)
	}
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(p9TReadDir, tag, p9ENOENT)
	}
	if !node.isDir() {
		return p9Error(p9TReadDir, tag, p9ENOTDIR)
	}
	offset := binary.LittleEndian.Uint64(body[4:])
	count := int(binary.LittleEndian.Uint32(body[12:]))
	var entries []byte
	for i := int(offset); i < len(node.children); i++ {
		child := node.children[i]
		entry := append([]byte{}, child.qid()...)
		entry = p9AppendU64(entry, uint64(i+1))
		entry = append(entry, child.dtype())
		entry = p9AppendString(entry, child.name)
		if len(entries)+len(entry) > count {
			break
		}
		entries = append(entries, entry...)
	}
	out := p9AppendU32(nil, uint32(len(entries)))
	out = append(out, entries...)
	return p9Reply(p9RReadDir, tag, out)
}

func (fs *Host9PFS) handleRead(tag uint16, body []byte) []byte {
	if len(body) < 16 {
		return p9Error(p9TRead, tag, p9EIO)
	}
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(p9TRead, tag, p9ENOENT)
	}
	offset := binary.LittleEndian.Uint64(body[4:])
	count := binary.LittleEndian.Uint32(body[12:])
	data, err := fs.readFile(node, offset, count)
	if err != nil {
		return p9Error(p9TRead, tag, p9EIO)
	}
	out := p9AppendU32(nil, uint32(len(data)))
	out = append(out, data...)
	return p9Reply(p9RRead, tag, out)
}

func (fs *Host9PFS) handleClunk(tag uint16, body []byte) []byte {
	if len(body) >= 4 {
		delete(fs.fids, binary.LittleEndian.Uint32(body))
	}
	return p9Reply(p9RClunk, tag, nil)
}

func (fs *Host9PFS) handleStatFS(tag uint16) []byte {
	var out []byte
	out = p9AppendU32(out, 0x01021997)
	out = p9AppendU32(out, 4096)
	out = p9AppendU64(out, 1000000)
	out = p9AppendU64(out, 500000)
	out = p9AppendU64(out, 500000)
	out = p9AppendU64(out, uint64(len(fs.inodes)))
	out = p9AppendU64(out, 100000)
	out = p9AppendU64(out, 0)
	out = p9AppendU32(out, 255)
	return p9Reply(p9RStatFS, tag, out)
}

func (fs *Host9PFS) readFile(node *host9pInode, offset uint64, count uint32) ([]byte, error) {
	if count == 0 || offset >= node.size {
		return nil, nil
	}
	if node.symlink != "" {
		data := []byte(node.symlink)
		return data[offset:min(uint64(len(data)), offset+uint64(count))], nil
	}
	if node.flatFile == "" {
		return nil, nil
	}
	limit := min(uint64(count), node.size-offset)
	data := make([]byte, limit)
	f, err := os.Open(filepath.Join(fs.flatDir, node.flatFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n, err := f.ReadAt(data, int64(offset))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data[:n], nil
}

func (n *host9pInode) child(name string) *host9pInode {
	if name == "." {
		return n
	}
	if name == ".." {
		if n.parent != nil {
			return n.parent
		}
		return n
	}
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	return nil
}

func (n *host9pInode) isDir() bool {
	return n.mode&host9pModeDir == host9pModeDir
}

func (n *host9pInode) qid() []byte {
	var typ byte = host9pQIDFile
	if n.isDir() {
		typ = host9pQIDDir
	} else if n.mode&host9pModeSymlink == host9pModeSymlink {
		typ = host9pQIDSymlink
	}
	out := []byte{typ}
	out = p9AppendU32(out, 0)
	out = p9AppendU64(out, n.ino)
	return out
}

func (n *host9pInode) dtype() byte {
	if n.isDir() {
		return host9pDTypeDir
	}
	if n.mode&host9pModeSymlink == host9pModeSymlink {
		return host9pDTypeSymlink
	}
	return host9pDTypeRegular
}

func p9Reply(typ byte, tag uint16, body []byte) []byte {
	out := make([]byte, 7, 7+len(body))
	binary.LittleEndian.PutUint32(out, uint32(7+len(body)))
	out[4] = typ
	binary.LittleEndian.PutUint16(out[5:], tag)
	return append(out, body...)
}

func p9Error(reqType byte, tag uint16, errno uint32) []byte {
	_ = reqType
	return p9Reply(p9RError, tag, p9AppendU32(nil, errno))
}

func p9AppendString(dst []byte, value string) []byte {
	dst = p9AppendU16(dst, uint16(len(value)))
	return append(dst, value...)
}

func p9AppendU16(dst []byte, value uint16) []byte {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], value)
	return append(dst, data[:]...)
}

func p9AppendU32(dst []byte, value uint32) []byte {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	return append(dst, data[:]...)
}

func p9AppendU64(dst []byte, value uint64) []byte {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], value)
	return append(dst, data[:]...)
}

type p9Cursor struct {
	data []byte
	pos  int
}

func (c *p9Cursor) string() (string, bool) {
	if c.pos+2 > len(c.data) {
		return "", false
	}
	size := int(binary.LittleEndian.Uint16(c.data[c.pos:]))
	c.pos += 2
	if c.pos+size > len(c.data) {
		return "", false
	}
	value := string(c.data[c.pos : c.pos+size])
	c.pos += size
	return value, true
}
