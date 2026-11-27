package kernel

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// The wasm32 struct sockaddr_un: a 16-bit family, then the path.
const (
	sockaddrUnSize     = 110
	sockaddrPathOffset = 2
)

// The fields of the wasm32 struct msghdr the kernel reads and writes.
const (
	msghdrNamelen    = 4
	msghdrIov        = 8
	msghdrIovlen     = 12
	msghdrControl    = 16
	msghdrControllen = 20
	msghdrFlags      = 24
)

// cmsghdrSize is the size of the wasm32 struct cmsghdr: the message length,
// level and type, each 32 bits. The message's data follows it.
const cmsghdrSize = 12

// scmMaxFD is the most files one send can carry, Linux's SCM_MAX_FD.
const scmMaxFD = 253

// uioMaxIov is the most iovecs one call takes, Linux's UIO_MAXIOV.
const uioMaxIov = 1024

// shutdown directions, which the guest's SHUT_RD and SHUT_WR share with the
// WASI sdflags.
const (
	shutRD = 1
	shutWR = 2
)

// unixSocket is a Unix stream socket. A connected socket's open file holds
// the connection's two pipes, so the socket keeps only its addresses and a
// listener's queue. The kernel lock guards its fields.
type unixSocket struct {
	// ino is the socket's inode number.
	ino uint64
	// path is the address bind gave the socket, or an accepted socket's
	// listener, or empty.
	path string
	// peerPath is the connected peer's address, or empty.
	peerPath string
	// cred is the credentials of the process that made the socket listen.
	// A connecting socket takes them as its peer's.
	cred ucred
	// peerCred is the credentials the connected peer had when it connected,
	// listened or called socketpair, as SO_PEERCRED reports them. It is
	// meaningful only while the socket file is connected.
	peerCred ucred
	// node is the filesystem node bind created, or nil.
	node *VNode
	// listener is a listening socket's queue, or nil.
	listener *unixListener
}

// ucred is the guest's struct ucred: the process id, user id and group id of
// a socket's peer.
type ucred struct {
	pid, uid, gid uint32
}

// ucredSize is the size of the wasm32 struct ucred.
const ucredSize = 12

// hostCred is the credentials of the host, which connects without a process.
var hostCred = ucred{}

// cred returns the process's credentials as a socket records them: its id and
// its effective user and group ids, as Linux does.
func (p *Process) cred() ucred {
	return ucred{pid: uint32(p.pid), uid: p.uids.eff, gid: p.gids.eff}
}

// vnode returns a node that describes the socket.
func (s *unixSocket) vnode() *VNode {
	return &VNode{ino: s.ino, mode: sIFSOCK | 0o777, nlink: 1}
}

// close stops a listening socket: later connects are refused, and the
// connections it has not accepted close.
func (s *unixSocket) close() {
	if s.listener == nil {
		return
	}
	if s.node != nil && s.node.socket == s {
		s.node.socket = nil
	}
	for _, conn := range s.listener.queue {
		conn.release()
	}
	s.listener.queue = nil
}

// unixListener holds a listening socket's connections that wait for accept.
// The kernel lock guards its fields.
type unixListener struct {
	// queue holds the server ends of the pending connections, oldest first.
	queue []*OpenFile
	// waitCh closes when a connection arrives.
	waitCh chan struct{}
}

// push queues a connection and wakes the waiters.
func (l *unixListener) push(conn *OpenFile) {
	l.queue = append(l.queue, conn)
	close(l.waitCh)
	l.waitCh = make(chan struct{})
}

// state reports whether accept would proceed without waiting, and a channel
// that closes when a connection arrives.
func (l *unixListener) state() (ready bool, change <-chan struct{}) {
	return len(l.queue) != 0, l.waitCh
}

// newSocketFile returns an open file for a new unconnected socket with the
// kernel's status flags.
func (k *Kernel) newSocketFile(status uint32) *OpenFile {
	return &OpenFile{flags: oRDWR | status, sock: &unixSocket{ino: k.vfs.allocIno()}}
}

// connectPair connects two socket files with a pipe in each direction.
func (k *Kernel) connectPair(a, b *OpenFile) {
	toA := newBufferedPipe(k.vfs.allocIno())
	toB := newBufferedPipe(k.vfs.allocIno())
	a.rx, a.tx = toA, toB
	b.rx, b.tx = toB, toA
}

// connectUnix connects the unconnected socket file of, whose process has the
// credentials cred, to the listener bound at the absolute path resolved. Each
// end records the other's credentials now: the listener's from when it
// listened, and cred. The connection waits in the listener's queue, so it
// returns at once. The caller holds the kernel lock.
func (k *Kernel) connectUnix(of *OpenFile, cred ucred, resolved string) uint64 {
	// Find the listener.
	node := k.vfs.lookup(resolved)
	switch {
	case node == nil:
		return negErrno(ENOENT)
	case node.socket == nil:
		return negErrno(ECONNREFUSED)
	}
	server := node.socket

	// Queue the server's end of a new connection.
	conn := &OpenFile{flags: oRDWR, sock: &unixSocket{
		ino:      k.vfs.allocIno(),
		path:     server.path,
		peerPath: of.sock.path,
		peerCred: cred,
	}}
	k.connectPair(of, conn)
	of.sock.peerPath = server.path
	of.sock.peerCred = server.cred
	server.listener.push(conn)
	return 0
}

// socketFile returns the open file of socket descriptor fd.
func (p *Process) socketFile(fd uint32) (*OpenFile, uint64) {
	of, ok := p.fds[fd]
	switch {
	case !ok:
		return nil, negErrno(EBADF)
	case of.sock == nil:
		return nil, negErrno(ENOTSOCK)
	}
	return of, 0
}

// socketStatus returns the kernel status flags of a socket type, which must
// be a stream socket of the default protocol in the Unix family, and whether
// its descriptors are close-on-exec.
func socketStatus(domain, typ, protocol uint32) (uint32, bool, uint64) {
	// Check the family, type and protocol.
	switch {
	case domain != guestAFUNIX:
		return 0, false, negErrno(EAFNOSUPPORT)
	case typ&^(guestSockNonblock|guestSockCloexec) != guestSockStream || protocol != 0:
		return 0, false, negErrno(EPROTONOSUPPORT)
	}

	// Read the flags.
	var status uint32
	if typ&guestSockNonblock != 0 {
		status = oNONBLOCK
	}
	return status, typ&guestSockCloexec != 0, 0
}

// sysSocket creates an unconnected Unix stream socket.
func (p *Process) sysSocket(domain, typ, protocol uint32) uint64 {
	status, cloexec, errno := socketStatus(domain, typ, protocol)
	if errno != 0 {
		return errno
	}
	return uint64(p.installFD(p.kernel.newSocketFile(status), cloexec))
}

// sysSocketpair creates two connected Unix stream sockets and writes their
// descriptors to svPtr.
func (p *Process) sysSocketpair(domain, typ, protocol, svPtr uint32) uint64 {
	// Connect two sockets.
	status, cloexec, errno := socketStatus(domain, typ, protocol)
	if errno != 0 {
		return errno
	}
	a, b := p.kernel.newSocketFile(status), p.kernel.newSocketFile(status)
	p.kernel.connectPair(a, b)
	a.sock.peerCred, b.sock.peerCred = p.cred(), p.cred()

	// Install them and return their numbers.
	afd, bfd := p.installFD(a, cloexec), p.installFD(b, cloexec)
	mem := p.inst.Memory()
	if !mem.WriteUint32Le(svPtr, afd) || !mem.WriteUint32Le(svPtr+4, bfd) {
		_ = p.sysClose(afd)
		_ = p.sysClose(bfd)
		return negErrno(EFAULT)
	}
	return 0
}

// readSockaddrPath reads the path of the sockaddr_un of addrLen bytes at
// addrPtr. It returns the path as given and resolved against the working
// directory. The kernel has no abstract namespace: an abstract address, a name
// after a leading null byte, returns abstract set and no path.
func (p *Process) readSockaddrPath(addrPtr, addrLen uint32) (path, resolved string, abstract bool, errno uint64) {
	// Read the address.
	if addrLen <= sockaddrPathOffset || addrLen > sockaddrUnSize {
		return "", "", false, negErrno(EINVAL)
	}
	buf, ok := p.inst.Memory().Read(addrPtr, addrLen)
	switch {
	case !ok:
		return "", "", false, negErrno(EFAULT)
	case binary.LittleEndian.Uint16(buf) != guestAFUNIX:
		return "", "", false, negErrno(EINVAL)
	}

	// Take the path up to its terminator.
	name := buf[sockaddrPathOffset:]
	if len(name) > 1 && name[0] == 0 {
		return "", "", true, 0
	}
	if i := bytes.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	if len(name) == 0 {
		return "", "", false, negErrno(EINVAL)
	}
	path = string(name)
	return path, resolvePath(p.cwd, p.fds, atFDCWD, path), false, 0
}

// writeSockaddr writes a sockaddr_un holding path to addrPtr, truncated to
// the capacity at lenPtr, and its full length to lenPtr. A null addrPtr
// writes nothing.
func (p *Process) writeSockaddr(path string, addrPtr, lenPtr uint32) uint64 {
	// Build the address.
	if addrPtr == 0 {
		return 0
	}
	addr := binary.LittleEndian.AppendUint16(nil, guestAFUNIX)
	if path != "" {
		addr = append(append(addr, path...), 0)
	}

	// Write what fits, and the full length.
	mem := p.inst.Memory()
	capacity, ok := mem.ReadUint32Le(lenPtr)
	if !ok || !mem.Write(addrPtr, addr[:min(int(capacity), len(addr))]) || !mem.WriteUint32Le(lenPtr, uint32(len(addr))) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysBind binds a socket to a path, creating a socket node there.
func (p *Process) sysBind(fd, addrPtr, addrLen uint32) uint64 {
	// Read the address of an unbound socket.
	of, errno := p.socketFile(fd)
	if errno != 0 {
		return errno
	}
	if of.sock.path != "" {
		return negErrno(EINVAL)
	}
	path, resolved, abstract, errno := p.readSockaddrPath(addrPtr, addrLen)
	switch {
	case errno != 0:
		return errno
	case abstract:
		return negErrno(EINVAL)
	}

	// Create the node, which must not exist.
	parent, name, errno := p.newEntry(resolved)
	switch {
	case errno == negErrno(EEXIST):
		return negErrno(EADDRINUSE)
	case errno != 0:
		return errno
	}
	of.sock.node = p.kernel.vfs.mkNode(parent, name, sIFSOCK|0o777&^p.umask, 0)
	of.sock.path = path
	return 0
}

// sysListen makes a bound socket accept connections at its node. The kernel
// queues every connection, so it ignores the backlog.
func (p *Process) sysListen(fd uint32) uint64 {
	of, errno := p.socketFile(fd)
	switch {
	case errno != 0:
		return errno
	case of.sock.node == nil || of.rx != nil:
		return negErrno(EINVAL)
	case of.sock.listener == nil:
		of.sock.listener = &unixListener{waitCh: make(chan struct{})}
		of.sock.cred = p.cred()
		of.sock.node.socket = of.sock
		p.kernel.listens.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
	return 0
}

// sysConnect connects a socket to the listener bound at a path. An abstract
// address refuses the connection, as Linux does for a name no socket binds, so
// a client such as libxcb falls back to the path.
func (p *Process) sysConnect(fd, addrPtr, addrLen uint32) uint64 {
	// Read the address of an unconnected socket.
	of, errno := p.socketFile(fd)
	switch {
	case errno != 0:
		return errno
	case of.rx != nil:
		return negErrno(EISCONN)
	case of.sock.listener != nil:
		return negErrno(EINVAL)
	}
	_, resolved, abstract, errno := p.readSockaddrPath(addrPtr, addrLen)
	switch {
	case errno != 0:
		return errno
	case abstract:
		return negErrno(ECONNREFUSED)
	}
	return p.kernel.connectUnix(of, p.cred(), resolved)
}

// sysAccept takes the oldest connection waiting on a listening socket and
// returns its descriptor, with the kernel status flags in status. It waits
// for a connection unless the socket is nonblocking. WASI's sock_accept
// carries no close-on-exec flag, so the descriptor never has one.
func (p *Process) sysAccept(ctx context.Context, fd, status uint32) uint64 {
	// Find the listener.
	of, errno := p.socketFile(fd)
	if errno != 0 {
		return errno
	}
	l := of.sock.listener
	if l == nil {
		return negErrno(EINVAL)
	}

	// Wait for a connection.
	ctx, release := p.interruptible(ctx)
	defer release()
	for len(l.queue) == 0 {
		if wait(of.ioContext(ctx), l.waitCh) {
			continue
		}
		if interrupted(ctx) {
			return negErrno(EINTR)
		}
		return negErrno(EAGAIN)
	}

	// Install it.
	conn := l.queue[0]
	l.queue = l.queue[1:]
	conn.flags |= status
	return uint64(p.installFD(conn, false))
}

// sysSockname writes a socket's address, or its peer's when peer is set, to
// addrPtr and its length to lenPtr.
func (p *Process) sysSockname(fd, addrPtr, lenPtr uint32, peer bool) uint64 {
	// Find the address.
	of, errno := p.socketFile(fd)
	if errno != 0 {
		return errno
	}
	path := of.sock.path
	if peer {
		if of.rx == nil {
			return negErrno(ENOTCONN)
		}
		path = of.sock.peerPath
	}
	return p.writeSockaddr(path, addrPtr, lenPtr)
}

// sysShutdown closes the reading or writing direction of a connected
// socket, or both. A peer reading a closed direction sees end of file.
func (p *Process) sysShutdown(fd, how uint32) uint64 {
	// Check the socket and the directions.
	of, errno := p.socketFile(fd)
	switch {
	case errno != 0:
		return errno
	case how == 0 || how&^(shutRD|shutWR) != 0:
		return negErrno(EINVAL)
	case of.rx == nil:
		return negErrno(ENOTCONN)
	}

	// Close them.
	if how&shutRD != 0 {
		of.rx.close(true)
	}
	if how&shutWR != 0 {
		of.tx.close(false)
	}
	return 0
}

// sysGetsockopt writes a SOL_SOCKET option of a socket to optPtr, truncated
// to the capacity at lenPtr, and the option's full length to lenPtr. The
// options are the socket's type, pending error and listening state, its
// domain and protocol, and SO_PEERCRED, the credentials its connected peer
// had when it connected, listened or called socketpair. SO_PEERCRED on a
// socket with no peer fails with ENOTCONN. Any other option fails with
// ENOPROTOOPT.
func (p *Process) sysGetsockopt(fd, level, opt, optPtr, lenPtr uint32) uint64 {
	// Find the value.
	of, errno := p.socketFile(fd)
	switch {
	case errno != 0:
		return errno
	case level != guestSolSocket:
		return negErrno(ENOPROTOOPT)
	}
	var value []byte
	switch opt {
	case guestSoType:
		value = binary.LittleEndian.AppendUint32(nil, guestSockStream)
	case guestSoError, guestSoProtocol:
		value = binary.LittleEndian.AppendUint32(nil, 0)
	case guestSoDomain:
		value = binary.LittleEndian.AppendUint32(nil, guestAFUNIX)
	case guestSoAcceptconn:
		listening := uint32(0)
		if of.sock.listener != nil {
			listening = 1
		}
		value = binary.LittleEndian.AppendUint32(nil, listening)
	case guestSoPeercred:
		if of.rx == nil {
			return negErrno(ENOTCONN)
		}
		cred := of.sock.peerCred
		value = make([]byte, 0, ucredSize)
		for _, id := range []uint32{cred.pid, cred.uid, cred.gid} {
			value = binary.LittleEndian.AppendUint32(value, id)
		}
	default:
		return negErrno(ENOPROTOOPT)
	}

	// Write what fits, and the full length.
	mem := p.inst.Memory()
	capacity, ok := mem.ReadUint32Le(lenPtr)
	if !ok || !mem.Write(optPtr, value[:min(int(capacity), len(value))]) || !mem.WriteUint32Le(lenPtr, uint32(len(value))) {
		return negErrno(EFAULT)
	}
	return 0
}

// sysSetsockopt accepts every option on a socket. The kernel's sockets have
// no tunable buffers, timeouts or addresses to reuse.
func (p *Process) sysSetsockopt(fd uint32) uint64 {
	_, errno := p.socketFile(fd)
	return errno
}

// msgContext returns the context of a send or receive with the guest's
// message flags. MSG_DONTWAIT makes it nonblocking. MSG_WAITALL is accepted
// and a short read returns what arrived. The caller handles MSG_NOSIGNAL, and
// the receiver handles MSG_CMSG_CLOEXEC. Peeking is unsupported.
func msgContext(ctx context.Context, flags uint32) (context.Context, uint64) {
	switch {
	case flags&^(guestMsgDontwait|guestMsgWaitall|guestMsgNosignal|guestMsgCmsgCloexec) != 0:
		return nil, negErrno(EOPNOTSUPP)
	case flags&guestMsgDontwait != 0:
		return nonblocking(ctx), 0
	}
	return ctx, 0
}

// sendSocket writes data to the connected socket of with files attached to
// its first byte, taking over the caller's reference to each file.
func (p *Process) sendSocket(ctx context.Context, of *OpenFile, data []byte, files []*OpenFile, flags uint32) uint64 {
	ctx, errno := msgContext(ctx, flags)
	switch {
	case errno != 0:
	case of.tx == nil:
		errno = negErrno(ENOTCONN)
	default:
		return p.writePipe(ctx, of, data, files, flags&guestMsgNosignal != 0)
	}
	unrefFiles(files)
	return errno
}

// recvSocket reads up to count bytes from the connected socket of, and the
// files sent with the first byte read, whose references pass to the caller.
// A caught signal interrupts a blocking receive.
func (p *Process) recvSocket(ctx context.Context, of *OpenFile, count uint32, flags uint32) ([]byte, []*OpenFile, uint64) {
	// Check the flags and the connection.
	ctx, errno := msgContext(ctx, flags)
	switch {
	case errno != 0:
		return nil, nil, errno
	case of.rx == nil:
		return nil, nil, negErrno(ENOTCONN)
	case count == 0:
		return nil, nil, 0
	}

	// Read, letting a caught signal end the wait.
	ctx, release := p.interruptible(ctx)
	defer release()
	return p.readPipe(ctx, of, int(count))
}

// sysSendto writes count bytes at bufPtr to a connected socket. A connected
// stream socket takes no destination address.
func (p *Process) sysSendto(ctx context.Context, fd, bufPtr, count, flags, addrLen uint32) uint64 {
	// Check the socket and the address.
	of, errno := p.socketFile(fd)
	switch {
	case errno != 0:
		return errno
	case addrLen != 0:
		return negErrno(EISCONN)
	}

	// Write the data.
	data, ok := p.inst.Memory().Read(bufPtr, count)
	if !ok {
		return negErrno(EFAULT)
	}
	return p.sendSocket(ctx, of, data, nil, flags)
}

// sysRecvfrom reads up to count bytes from a connected socket to bufPtr. A
// stream socket reports no source address, and the read drops any files sent
// with the data.
func (p *Process) sysRecvfrom(ctx context.Context, fd, bufPtr, count, flags, lenPtr uint32) uint64 {
	// Read the data.
	of, errno := p.socketFile(fd)
	if errno != 0 {
		return errno
	}
	data, files, errno := p.recvSocket(ctx, of, count, flags)
	unrefFiles(files)
	if errno != 0 {
		return errno
	}

	// Copy it out and report an empty address.
	mem := p.inst.Memory()
	if !mem.Write(bufPtr, data) || lenPtr != 0 && !mem.WriteUint32Le(lenPtr, 0) {
		return negErrno(EFAULT)
	}
	return uint64(len(data))
}

// sysSendmsg writes the iovecs of the msghdr at msgPtr to a connected socket
// as one write, with the files of its SCM_RIGHTS messages attached to the
// first byte. A send of no bytes sends no files.
func (p *Process) sysSendmsg(ctx context.Context, fd, msgPtr, flags uint32) uint64 {
	// Read the header and the data.
	of, errno := p.socketFile(fd)
	if errno != 0 {
		return errno
	}
	hdr, errno := readMsghdr(p.inst.Memory(), msgPtr)
	if errno != 0 {
		return errno
	}
	data, errno := p.gatherIovecs(hdr.iovs)
	if errno != 0 {
		return errno
	}

	// Take the files and send.
	files, errno := p.readRights(hdr.control, hdr.controlLen)
	if errno != 0 {
		return errno
	}
	return p.sendSocket(ctx, of, data, files, flags)
}

// sysRecvmsg reads a connected socket into the iovecs of the msghdr at
// msgPtr, stopping before bytes that carry other files, and installs the
// files sent with the first byte read as descriptors in an SCM_RIGHTS
// message. Files that do not fit the control buffer close, and MSG_CTRUNC
// reports them.
func (p *Process) sysRecvmsg(ctx context.Context, fd, msgPtr, flags uint32) uint64 {
	// Read the header.
	of, errno := p.socketFile(fd)
	if errno != 0 {
		return errno
	}
	mem := p.inst.Memory()
	hdr, errno := readMsghdr(mem, msgPtr)
	if errno != 0 {
		return errno
	}

	// Read the data and the files sent with it.
	var total uint32
	for _, iov := range hdr.iovs {
		total += iov.length
	}
	data, files, errno := p.recvSocket(ctx, of, total, flags)
	if errno != 0 {
		return errno
	}

	// Scatter the data into the iovecs.
	rest := data
	for _, iov := range hdr.iovs {
		n := min(int(iov.length), len(rest))
		if !mem.Write(iov.base, rest[:n]) {
			unrefFiles(files)
			return negErrno(EFAULT)
		}
		rest = rest[n:]
	}

	// Install the files, then report no address, the control length and
	// the flags.
	controlLen, msgFlags, errno := p.writeRights(files, hdr.control, hdr.controlLen, flags&guestMsgCmsgCloexec != 0)
	if errno != 0 {
		return errno
	}
	if !mem.WriteUint32Le(msgPtr+msghdrNamelen, 0) ||
		!mem.WriteUint32Le(msgPtr+msghdrControllen, controlLen) ||
		!mem.WriteUint32Le(msgPtr+msghdrFlags, msgFlags) {
		return negErrno(EFAULT)
	}
	return uint64(len(data))
}

// gatherIovecs reads the data of iovecs into one buffer.
func (p *Process) gatherIovecs(iovs []iovec) ([]byte, uint64) {
	mem := p.inst.Memory()
	var data []byte
	for _, iov := range iovs {
		buf, ok := mem.Read(iov.base, iov.length)
		if !ok {
			return nil, negErrno(EFAULT)
		}
		data = append(data, buf...)
	}
	return data, 0
}

// readRights reads the SCM_RIGHTS messages in the control buffer of length
// bytes at ptr, and returns their open files with a reference taken on each.
// Any other control message fails with EINVAL, as on a Linux Unix socket.
func (p *Process) readRights(ptr, length uint32) ([]*OpenFile, uint64) {
	// Read the control buffer.
	if length == 0 {
		return nil, 0
	}
	buf, ok := p.inst.Memory().Read(ptr, length)
	if !ok {
		return nil, negErrno(EFAULT)
	}

	// Find the file of each descriptor in each message.
	var files []*OpenFile
	for len(buf) >= cmsghdrSize {
		msgLen := binary.LittleEndian.Uint32(buf)
		level := binary.LittleEndian.Uint32(buf[4:])
		typ := binary.LittleEndian.Uint32(buf[8:])
		if msgLen < cmsghdrSize || msgLen > uint32(len(buf)) || level != guestSolSocket || typ != guestScmRights {
			return nil, negErrno(EINVAL)
		}
		for fds := buf[cmsghdrSize:msgLen]; len(fds) >= 4; fds = fds[4:] {
			of, ok := p.fds[binary.LittleEndian.Uint32(fds)]
			if !ok {
				return nil, negErrno(EBADF)
			}
			files = append(files, of)
		}
		buf = buf[min(cmsgAlign(msgLen), uint32(len(buf))):]
	}

	// Take a reference on each, up to Linux's limit per message.
	if len(files) > scmMaxFD {
		return nil, negErrno(EINVAL)
	}
	for _, of := range files {
		of.ref()
	}
	return files, 0
}

// writeRights installs files as descriptors, close-on-exec when cloexec is
// set, and writes them as an SCM_RIGHTS message to the control buffer of
// capacity bytes at ptr. It returns the control length and the message flags:
// MSG_CTRUNC when files did not fit, which it releases. It takes over the
// references of files.
func (p *Process) writeRights(files []*OpenFile, ptr, capacity uint32, cloexec bool) (uint32, uint32, uint64) {
	// Fit what the buffer holds and release the rest.
	if len(files) == 0 {
		return 0, 0, 0
	}
	defer unrefFiles(files)
	fit := 0
	if capacity >= cmsghdrSize {
		fit = min(len(files), int(capacity-cmsghdrSize)/4)
	}
	var msgFlags uint32
	if fit < len(files) {
		msgFlags = guestMsgCtrunc
	}
	if fit == 0 {
		return 0, msgFlags, 0
	}

	// Install the descriptors and build the message.
	msgLen := cmsghdrSize + 4*uint32(fit)
	msg := binary.LittleEndian.AppendUint32(nil, msgLen)
	msg = binary.LittleEndian.AppendUint32(msg, guestSolSocket)
	msg = binary.LittleEndian.AppendUint32(msg, guestScmRights)
	fds := make([]uint32, 0, fit)
	for _, of := range files[:fit] {
		fd := p.installFD(of, cloexec)
		fds = append(fds, fd)
		msg = binary.LittleEndian.AppendUint32(msg, fd)
	}

	// Write it, closing the descriptors when the buffer is out of bounds.
	if !p.inst.Memory().Write(ptr, msg) {
		for _, fd := range fds {
			_ = p.sysClose(fd)
		}
		return 0, 0, negErrno(EFAULT)
	}
	return msgLen, msgFlags, 0
}

// iovec is a guest struct iovec: a buffer's address and length.
type iovec struct {
	// base is the buffer's address.
	base uint32
	// length is the buffer's length in bytes.
	length uint32
}

// msghdr holds the fields of a guest struct msghdr the kernel uses.
type msghdr struct {
	// iovs are the data buffers.
	iovs []iovec
	// control is the address of the control buffer.
	control uint32
	// controlLen is the control buffer's length in bytes.
	controlLen uint32
}

// readMsghdr reads the msghdr at ptr and its iovec array.
func readMsghdr(mem engine.Memory, ptr uint32) (msghdr, uint64) {
	// Read the header.
	iovsPtr, ok1 := mem.ReadUint32Le(ptr + msghdrIov)
	iovsLen, ok2 := mem.ReadUint32Le(ptr + msghdrIovlen)
	control, ok3 := mem.ReadUint32Le(ptr + msghdrControl)
	controlLen, ok4 := mem.ReadUint32Le(ptr + msghdrControllen)
	switch {
	case !ok1 || !ok2 || !ok3 || !ok4:
		return msghdr{}, negErrno(EFAULT)
	case iovsLen > uioMaxIov:
		return msghdr{}, negErrno(EMSGSIZE)
	}

	// Read the iovecs.
	hdr := msghdr{iovs: make([]iovec, 0, iovsLen), control: control, controlLen: controlLen}
	for i := range iovsLen {
		base, ok1 := mem.ReadUint32Le(iovsPtr + i*wasiIovecSize)
		length, ok2 := mem.ReadUint32Le(iovsPtr + i*wasiIovecSize + 4)
		if !ok1 || !ok2 {
			return msghdr{}, negErrno(EFAULT)
		}
		hdr.iovs = append(hdr.iovs, iovec{base: base, length: length})
	}
	return hdr, 0
}

// cmsgAlign rounds a control message length up to the guest's alignment of
// the next message, the size of a long.
func cmsgAlign(n uint32) uint32 {
	return (n + 3) &^ 3
}
