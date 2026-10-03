package v86_wazero

import (
	"encoding/binary"
	"io"
	"math"
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

// host9pInode is one node of the exported tree: its stat metadata plus
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
	// Read the root image metadata from fs.json.
	data, err := os.ReadFile(filepath.Join(dir, "fs.json"))
	if err != nil {
		return nil, errors.Wrap(err, "read fs.json")
	}

	// Parse the root image metadata and require its inode entries.
	var parser fastjson.Parser
	root, err := parser.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse fs.json")
	}
	entries := root.GetArray("fsroot")
	if entries == nil {
		return nil, errors.New("fs.json missing fsroot array")
	}

	// Build the Host9PFS root and load its inode tree.
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

// loadChildren recursively builds the inode tree from fs.json's fsroot
func (fs *Host9PFS) loadChildren(parent *host9pInode, values []*fastjson.Value) error {
	for _, value := range values {
		fields := value.GetArray()
		if len(fields) < 6 {
			return errors.New("fs.json entry has fewer than 6 fields")
		}
		name := string(fields[0].GetStringBytes())
		if len(name) > math.MaxUint16 {
			return errors.Errorf("fs.json entry name exceeds 9P length: %d", len(name))
		}
		inode := &host9pInode{
			ino:    uint64(len(fs.inodes)),
			name:   name,
			size:   fields[1].GetUint64(),
			mtime:  fields[2].GetUint64(),
			mode:   uint32(fields[3].GetUint()), //nolint:gosec // fs.json metadata is emitted as the 9P u32 mode field.
			uid:    uint32(fields[4].GetUint()), //nolint:gosec // fs.json metadata is emitted as the 9P u32 uid field.
			gid:    uint32(fields[5].GetUint()), //nolint:gosec // fs.json metadata is emitted as the 9P u32 gid field.
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

// Handle serves one 9P request frame and returns the reply frame bytes.
func (fs *Host9PFS) Handle(req []byte) []byte {
	// Require the 9P header and reject a declared size outside the frame.
	if len(req) < 7 {
		return nil
	}
	size := binary.LittleEndian.Uint32(req)
	if size < 7 || uint64(size) > uint64(len(req)) {
		return p9Error(binary.LittleEndian.Uint16(req[5:]), p9EIO)
	}

	// Record the 9P request type before dispatching its body.
	msgType := req[4]
	tag := binary.LittleEndian.Uint16(req[5:])
	body := req[7:size]
	fs.requests.Add(1)
	fs.lastType.Store(uint32(msgType))

	// Dispatch the 9P request to its protocol handler.
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
		return p9Error(tag, p9EOPNOTSUPP)
	}
}

// stats reports request, notify, and queue counters for diagnostics.
func (fs *Host9PFS) stats() (uint64, byte, uint64, uint32, uint32, bool) {
	if fs == nil {
		return 0, 0, 0, 0, 0, false
	}
	return fs.requests.Load(),
		byte(fs.lastType.Load()), //nolint:gosec // lastType stores the fixed-width 9P message type byte.
		fs.notifies.Load(),
		fs.availIdx.Load(),
		fs.availLastIdx.Load(),
		fs.queueConfigured.Load() != 0
}

// handleVersion negotiates the 9P2000.L protocol version.
func (fs *Host9PFS) handleVersion(tag uint16, body []byte) []byte {
	// Require the message-size field before negotiating the 9P protocol.
	if len(body) < 4 {
		return p9Error(tag, p9EIO)
	}

	// Encode the negotiated message size and 9P2000.L version.
	var out []byte
	out = p9AppendU32(out, binary.LittleEndian.Uint32(body))
	out = p9AppendString(out, "9P2000.L")
	return p9Reply(p9RVersion, tag, out)
}

// handleAttach binds a fid to the export root.
func (fs *Host9PFS) handleAttach(tag uint16, body []byte) []byte {
	if len(body) < 4 || len(fs.inodes) == 0 {
		return p9Error(tag, p9EIO)
	}
	fs.fids[binary.LittleEndian.Uint32(body)] = fs.inodes[0]
	return p9Reply(p9RAttach, tag, fs.inodes[0].qid())
}

// handleWalk resolves a walk path from a fid and returns the visited qids.
func (fs *Host9PFS) handleWalk(tag uint16, body []byte) []byte {
	// Require the 9P walk header before decoding its path.
	if len(body) < 10 {
		return p9Error(tag, p9EIO)
	}

	// Resolve the starting fid and decode the walk destination.
	fid := binary.LittleEndian.Uint32(body)
	newfid := binary.LittleEndian.Uint32(body[4:])
	count := int(binary.LittleEndian.Uint16(body[8:]))
	cursor := p9Cursor{data: body[10:]}
	node := fs.fids[fid]
	if node == nil {
		return p9Error(tag, p9ENOENT)
	}

	// Traverse the inode path and collect the visited qids.
	var qids []byte
	for range count {
		name, ok := cursor.string()
		if !ok {
			return p9Error(tag, p9EIO)
		}
		if !node.isDir() {
			return p9Error(tag, p9ENOTDIR)
		}
		next := node.child(name)
		if next == nil {
			return p9Error(tag, p9ENOENT)
		}
		node = next
		qids = append(qids, node.qid()...)
	}

	// Bind the destination fid and encode the visited qids.
	fs.fids[newfid] = node
	var out []byte
	out = p9AppendU16(out, uint16(count)) //nolint:gosec // TWalk's count is a uint16 protocol field.
	out = append(out, qids...)
	return p9Reply(p9RWalk, tag, out)
}

// handleLOpen prepares a fid for IO and returns its qid and IO unit.
func (fs *Host9PFS) handleLOpen(tag uint16, body []byte) []byte {
	// Require the fid field before opening its inode.
	if len(body) < 4 {
		return p9Error(tag, p9EIO)
	}

	// Resolve the inode bound to the requested fid.
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(tag, p9ENOENT)
	}

	// Encode the inode qid and supported IO unit.
	out := append([]byte{}, node.qid()...)
	out = p9AppendU32(out, 65536)
	return p9Reply(p9RLOpen, tag, out)
}

// handleReadLink returns the target string of a symlink fid.
func (fs *Host9PFS) handleReadLink(tag uint16, body []byte) []byte {
	if len(body) < 4 {
		return p9Error(tag, p9EIO)
	}
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(tag, p9ENOENT)
	}
	return p9Reply(p9RReadLink, tag, p9AppendString(nil, node.symlink))
}

// handleGetAttr returns the stat metadata of a fid's inode.
func (fs *Host9PFS) handleGetAttr(tag uint16, body []byte) []byte {
	// Require the fid field before reading inode attributes.
	if len(body) < 4 {
		return p9Error(tag, p9EIO)
	}

	// Resolve the inode bound to the requested fid.
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(tag, p9ENOENT)
	}

	// Encode the inode identity, permissions, and link metadata.
	var out []byte
	out = p9AppendU64(out, 0x7ff)
	out = append(out, node.qid()...)
	out = p9AppendU32(out, node.mode)
	out = p9AppendU32(out, node.uid)
	out = p9AppendU32(out, node.gid)
	out = p9AppendU64(out, 1)
	out = p9AppendU64(out, 0)

	// Encode the inode storage size and block allocation.
	out = p9AppendU64(out, node.size)
	out = p9AppendU64(out, 4096)
	out = p9AppendU64(out, (node.size+511)/512)

	// Encode the inode timestamps and unused generation fields.
	for range 4 {
		out = p9AppendU64(out, node.mtime)
		out = p9AppendU64(out, 0)
	}
	out = p9AppendU64(out, 0)
	out = p9AppendU64(out, 0)
	return p9Reply(p9RGetAttr, tag, out)
}

// handleReadDir streams one directory entry per child of a directory fid.
func (fs *Host9PFS) handleReadDir(tag uint16, body []byte) []byte {
	// Require the 9P directory read header before decoding its range.
	if len(body) < 16 {
		return p9Error(tag, p9EIO)
	}

	// Resolve the fid and require a directory inode.
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(tag, p9ENOENT)
	}
	if !node.isDir() {
		return p9Error(tag, p9ENOTDIR)
	}

	// Decode the directory range and handle an exhausted child list.
	offset := binary.LittleEndian.Uint64(body[4:])
	count := int(binary.LittleEndian.Uint32(body[12:]))
	var entries []byte
	if offset >= uint64(len(node.children)) {
		return p9Reply(p9RReadDir, tag, p9AppendU32(nil, 0))
	}

	// Encode directory entries within the requested byte count.
	for i := int(offset); i < len(node.children); i++ { //nolint:gosec // offset is bounded by the child slice length above.
		child := node.children[i]
		entry := append([]byte{}, child.qid()...)
		entry = p9AppendU64(entry, uint64(i+1)) //nolint:gosec // directory offsets are nonnegative slice indexes.
		entry = append(entry, child.dtype())
		entry = p9AppendString(entry, child.name)
		if len(entries)+len(entry) > count {
			break
		}
		entries = append(entries, entry...)
	}

	// Frame the encoded directory entries with their byte length.
	out := p9AppendU32(nil, uint32(len(entries))) //nolint:gosec // entries is bounded by the negotiated 9P message size.
	out = append(out, entries...)
	return p9Reply(p9RReadDir, tag, out)
}

// handleRead returns a byte range of a regular file or symlink target.
func (fs *Host9PFS) handleRead(tag uint16, body []byte) []byte {
	// Require the 9P file read header before decoding its range.
	if len(body) < 16 {
		return p9Error(tag, p9EIO)
	}

	// Resolve the inode bound to the requested fid.
	node := fs.fids[binary.LittleEndian.Uint32(body)]
	if node == nil {
		return p9Error(tag, p9ENOENT)
	}

	// Read the requested byte range from the inode backing data.
	offset := binary.LittleEndian.Uint64(body[4:])
	count := binary.LittleEndian.Uint32(body[12:])
	data, err := fs.readFile(node, offset, count)
	if err != nil {
		return p9Error(tag, p9EIO)
	}

	// Frame the file bytes with their actual read length.
	out := p9AppendU32(nil, uint32(len(data))) //nolint:gosec // data is bounded by the negotiated 9P read size.
	out = append(out, data...)
	return p9Reply(p9RRead, tag, out)
}

// handleClunk drops a fid mapping.
func (fs *Host9PFS) handleClunk(tag uint16, body []byte) []byte {
	if len(body) >= 4 {
		delete(fs.fids, binary.LittleEndian.Uint32(body))
	}
	return p9Reply(p9RClunk, tag, nil)
}

// handleStatFS answers with fixed filesystem capacity figures.
func (fs *Host9PFS) handleStatFS(tag uint16) []byte {
	// Encode the filesystem type, block size, and block capacity.
	var out []byte
	out = p9AppendU32(out, 0x01021997)
	out = p9AppendU32(out, 4096)
	out = p9AppendU64(out, 1000000)
	out = p9AppendU64(out, 500000)
	out = p9AppendU64(out, 500000)

	// Encode the filesystem inode capacity and name length limit.
	out = p9AppendU64(out, uint64(len(fs.inodes)))
	out = p9AppendU64(out, 100000)
	out = p9AppendU64(out, 0)
	out = p9AppendU32(out, 255)
	return p9Reply(p9RStatFS, tag, out)
}

// readFile reads up to count bytes at an offset from an inode's backing
func (fs *Host9PFS) readFile(node *host9pInode, offset uint64, count uint32) ([]byte, error) {
	// Resolve empty ranges and in-memory symlink data before opening a file.
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

	// Open the inode backing file with a buffer bounded by its remaining size.
	limit := min(uint64(count), node.size-offset)
	data := make([]byte, limit)
	f, err := os.Open(filepath.Join(fs.flatDir, node.flatFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Require a file offset representable by the host file API.
	if offset > math.MaxInt64 {
		return nil, errors.New("9P file offset exceeds int64 range")
	}

	// Read the backing file range while accepting a partial final read.
	n, err := f.ReadAt(data, int64(offset)) //nolint:gosec // the preceding MaxInt64 check protects os.File.ReadAt.
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data[:n], nil
}

// child resolves a single path component against the inode.
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

// isDir reports whether the inode is a directory.
func (n *host9pInode) isDir() bool {
	return n.mode&host9pModeDir == host9pModeDir
}

// qid renders the 13-byte 9P qid for the inode.
func (n *host9pInode) qid() []byte {
	// Select the 9P qid type from the inode mode.
	var typ byte = host9pQIDFile
	if n.isDir() {
		typ = host9pQIDDir
	} else if n.mode&host9pModeSymlink == host9pModeSymlink {
		typ = host9pQIDSymlink
	}

	// Encode the qid type, version, and inode identity.
	out := []byte{typ}
	out = p9AppendU32(out, 0)
	out = p9AppendU64(out, n.ino)
	return out
}

// dtype renders the dirent type byte for readdir output.
func (n *host9pInode) dtype() byte {
	if n.isDir() {
		return host9pDTypeDir
	}
	if n.mode&host9pModeSymlink == host9pModeSymlink {
		return host9pDTypeSymlink
	}
	return host9pDTypeRegular
}

// p9Reply frames a reply: size, message type, tag, then body.
func p9Reply(typ byte, tag uint16, body []byte) []byte {
	// Encode the 9P reply header and append its payload.
	out := make([]byte, 7, 7+len(body))
	binary.LittleEndian.PutUint32(out, uint32(7+len(body))) //nolint:gosec // 9P frames use a uint32 byte length.
	out[4] = typ
	binary.LittleEndian.PutUint16(out[5:], tag)
	return append(out, body...)
}

// p9Error frames an error reply carrying one errno word.
func p9Error(tag uint16, errno uint32) []byte {
	return p9Reply(p9RError, tag, p9AppendU32(nil, errno))
}

// p9AppendString writes a length-prefixed string.
func p9AppendString(dst []byte, value string) []byte {
	dst = p9AppendU16(dst, uint16(len(value))) //nolint:gosec // loadChildren rejects names beyond the uint16 9P field.
	return append(dst, value...)
}

// p9AppendU16 writes a little-endian uint16.
func p9AppendU16(dst []byte, value uint16) []byte {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], value)
	return append(dst, data[:]...)
}

// p9AppendU32 writes a little-endian uint32.
func p9AppendU32(dst []byte, value uint32) []byte {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	return append(dst, data[:]...)
}

// p9AppendU64 writes a little-endian uint64.
func p9AppendU64(dst []byte, value uint64) []byte {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], value)
	return append(dst, data[:]...)
}

// p9Cursor reads length-prefixed fields out of one 9P request body.
type p9Cursor struct {
	data []byte
	pos  int
}

// string consumes one length-prefixed string, reporting ok=false on
func (c *p9Cursor) string() (string, bool) {
	// Require the 9P string length prefix before decoding its size.
	if c.pos+2 > len(c.data) {
		return "", false
	}

	// Consume the length prefix and require the complete string payload.
	size := int(binary.LittleEndian.Uint16(c.data[c.pos:]))
	c.pos += 2
	if c.pos+size > len(c.data) {
		return "", false
	}

	// Consume the 9P string payload and advance the request cursor.
	value := string(c.data[c.pos : c.pos+size])
	c.pos += size
	return value, true
}
