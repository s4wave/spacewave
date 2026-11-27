package kernel

import (
	"context"
	"encoding/binary"
	"testing"
	"time"
)

// Guest memory addresses the socket tests use.
const (
	sockTestAddr = 1024
	sockTestLen  = 1200
	sockTestBuf  = 2048
	sockTestPair = 3072
)

// writeTestSockaddr writes a sockaddr_un holding path to sockTestAddr and
// returns its length.
func writeTestSockaddr(mem *memMemory, path string) uint32 {
	addr := binary.LittleEndian.AppendUint16(nil, guestAFUNIX)
	addr = append(append(addr, path...), 0)
	mem.Write(sockTestAddr, addr)
	return uint32(len(addr))
}

// listenTestSocket binds a socket to path, listens on it and returns its
// descriptor.
func listenTestSocket(t *testing.T, p *Process, mem *memMemory, path string, typ uint32) uint32 {
	// Create, bind and listen.
	t.Helper()
	fd := uint32(p.sysSocket(guestAFUNIX, typ, 0))
	if res := p.sysBind(fd, sockTestAddr, writeTestSockaddr(mem, path)); res != 0 {
		t.Fatalf("bind(%s) = %#x", path, res)
	}
	if res := p.sysListen(fd); res != 0 {
		t.Fatalf("listen = %#x", res)
	}
	return fd
}

// sendTest writes data to fd and checks the whole write took.
func sendTest(t *testing.T, p *Process, mem *memMemory, fd uint32, data string) {
	t.Helper()
	mem.Write(sockTestBuf, []byte(data))
	if res := p.sysWrite(context.Background(), fd, sockTestBuf, uint32(len(data))); res != uint64(len(data)) {
		t.Fatalf("write(%d, %q) = %#x", fd, data, res)
	}
}

// recvTest reads up to 64 bytes from fd and returns the result and the
// bytes read.
func recvTest(p *Process, mem *memMemory, fd uint32) (uint64, string) {
	res := p.sysRead(context.Background(), fd, sockTestBuf, 64)
	if errnoFailed(res) {
		return res, ""
	}
	buf, _ := mem.Read(sockTestBuf, uint32(res))
	return res, string(buf)
}

// TestUnixSocketConnectAccept checks a client connects to a bound listener,
// both directions carry data, the names report the bound path, and closing
// one end gives the other end of file.
func TestUnixSocketConnectAccept(t *testing.T) {
	// Connect a client to a listener and accept it.
	p, mem := fsTestProcess()
	listener := listenTestSocket(t, p, mem, "/tmp/sock", guestSockStream)
	client := uint32(p.sysSocket(guestAFUNIX, guestSockStream, 0))
	if res := p.sysConnect(client, sockTestAddr, writeTestSockaddr(mem, "/tmp/sock")); res != 0 {
		t.Fatalf("connect = %#x", res)
	}
	server := p.sysAccept(context.Background(), listener, 0)
	if errnoFailed(server) {
		t.Fatalf("accept = %#x", server)
	}

	// Send each way.
	sendTest(t, p, mem, client, "ping")
	if _, got := recvTest(p, mem, uint32(server)); got != "ping" {
		t.Fatalf("server read %q, want ping", got)
	}
	sendTest(t, p, mem, uint32(server), "pong")
	if _, got := recvTest(p, mem, client); got != "pong" {
		t.Fatalf("client read %q, want pong", got)
	}

	// The client's peer is the listener's path.
	mem.WriteUint32Le(sockTestLen, sockaddrUnSize)
	if res := p.sysSockname(client, sockTestAddr, sockTestLen, true); res != 0 {
		t.Fatalf("getpeername = %#x", res)
	}
	n, _ := mem.ReadUint32Le(sockTestLen)
	addr, _ := mem.Read(sockTestAddr, n)
	if string(addr[sockaddrPathOffset:]) != "/tmp/sock\x00" {
		t.Fatalf("peer name %q, want /tmp/sock", addr[sockaddrPathOffset:])
	}

	// Closing the server's end ends the client's reads.
	_ = p.sysClose(uint32(server))
	if res, _ := recvTest(p, mem, client); res != 0 {
		t.Fatalf("read after the peer closed = %#x, want 0", res)
	}
}

// TestUnixSocketAddressErrors checks a bound path refuses a second bind, a
// path whose listener closed refuses connections, and an abstract address
// refuses connections and cannot be bound.
func TestUnixSocketAddressErrors(t *testing.T) {
	// Bind the path twice.
	p, mem := fsTestProcess()
	listener := listenTestSocket(t, p, mem, "/tmp/sock", guestSockStream)
	other := uint32(p.sysSocket(guestAFUNIX, guestSockStream, 0))
	if res := p.sysBind(other, sockTestAddr, writeTestSockaddr(mem, "/tmp/sock")); res != negErrno(EADDRINUSE) {
		t.Fatalf("second bind = %#x, want EADDRINUSE", res)
	}

	// Connect after the listener closed.
	_ = p.sysClose(listener)
	if res := p.sysConnect(other, sockTestAddr, writeTestSockaddr(mem, "/tmp/sock")); res != negErrno(ECONNREFUSED) {
		t.Fatalf("connect to a closed listener = %#x, want ECONNREFUSED", res)
	}
	if res := p.sysSocket(guestAFUNIX+1, guestSockStream, 0); res != negErrno(EAFNOSUPPORT) {
		t.Fatalf("socket in another family = %#x, want EAFNOSUPPORT", res)
	}

	// Connect to and bind an abstract address.
	if res := p.sysConnect(other, sockTestAddr, writeTestSockaddr(mem, "\x00/tmp/.X11-unix/X0")); res != negErrno(ECONNREFUSED) {
		t.Fatalf("connect to an abstract address = %#x, want ECONNREFUSED", res)
	}
	if res := p.sysBind(other, sockTestAddr, writeTestSockaddr(mem, "\x00/tmp/.X11-unix/X0")); res != negErrno(EINVAL) {
		t.Fatalf("bind to an abstract address = %#x, want EINVAL", res)
	}
}

// TestUnixSocketNonblocking checks a nonblocking accept and read return
// EAGAIN instead of waiting, and that F_GETFL reports the guest's flags.
func TestUnixSocketNonblocking(t *testing.T) {
	// Accept with nothing queued.
	p, mem := fsTestProcess()
	listener := listenTestSocket(t, p, mem, "/tmp/sock", guestSockStream|guestSockNonblock)
	if res := p.sysAccept(context.Background(), listener, 0); res != negErrno(EAGAIN) {
		t.Fatalf("nonblocking accept = %#x, want EAGAIN", res)
	}

	// Read an empty socket made nonblocking by F_SETFL.
	if res := p.sysSocketpair(guestAFUNIX, guestSockStream, 0, sockTestPair); res != 0 {
		t.Fatalf("socketpair = %#x", res)
	}
	a, _ := mem.ReadUint32Le(sockTestPair)
	if res := p.sysFcntl(a, fSETFL, guestONONBLOCK); res != 0 {
		t.Fatalf("F_SETFL = %#x", res)
	}
	if res, _ := recvTest(p, mem, a); res != negErrno(EAGAIN) {
		t.Fatalf("nonblocking read = %#x, want EAGAIN", res)
	}
	if got, want := p.sysFcntl(a, fGETFL, 0), uint64(guestORDONLY|guestOWRONLY|guestONONBLOCK); got != want {
		t.Fatalf("F_GETFL = %#x, want %#x", got, want)
	}
}

// writeTestSubscription writes a poll_oneoff descriptor subscription at index
// i of the array at inPtr.
func writeTestSubscription(mem *memMemory, inPtr, i uint32, userdata uint64, eventType uint8, fd uint32) {
	// Clear the subscription, then fill its header.
	sub := inPtr + i*wasiSubscriptionSize
	mem.Write(sub, make([]byte, wasiSubscriptionSize))
	mem.WriteUint64Le(sub, userdata)
	mem.WriteByteAt(sub+8, eventType)
	mem.WriteUint32Le(sub+16, fd)
}

// TestPollOneoffWaitsForReadiness checks poll_oneoff reports a writable
// socket at once, waits for data on an empty one, and reports a hangup.
func TestPollOneoffWaitsForReadiness(t *testing.T) {
	// Connect a pair and subscribe to both directions of one end.
	const inPtr, outPtr, neventsPtr = 1024, 1536, 2000
	p, mem := fsTestProcess()
	if res := p.sysSocketpair(guestAFUNIX, guestSockStream, 0, sockTestPair); res != 0 {
		t.Fatalf("socketpair = %#x", res)
	}
	a, _ := mem.ReadUint32Le(sockTestPair)
	b, _ := mem.ReadUint32Le(sockTestPair + 4)
	writeTestSubscription(mem, inPtr, 0, 1, wasiEventtypeFDRead, a)
	writeTestSubscription(mem, inPtr, 1, 2, wasiEventtypeFDWrite, a)
	ctx := context.Background()

	// Only the write is ready.
	if errno := p.wasiPollOneoff(ctx, mem, inPtr, outPtr, 2, neventsPtr); errno != wasiErrnoSuccess {
		t.Fatalf("poll_oneoff = %d", errno)
	}
	if n, _ := mem.ReadUint32Le(neventsPtr); n != 1 {
		t.Fatalf("poll_oneoff reported %d events, want 1", n)
	}
	if userdata, _ := mem.ReadUint64Le(outPtr); userdata != 2 {
		t.Fatalf("poll_oneoff reported userdata %d, want 2", userdata)
	}

	// A read waits until the peer writes.
	peer := p.fds[b].tx
	go func() {
		time.Sleep(10 * time.Millisecond)
		_, _ = peer.write(ctx, []byte("x"), nil)
	}()
	if errno := p.wasiPollOneoff(ctx, mem, inPtr, outPtr, 1, neventsPtr); errno != wasiErrnoSuccess {
		t.Fatalf("poll_oneoff for data = %d", errno)
	}
	if userdata, _ := mem.ReadUint64Le(outPtr); userdata != 1 {
		t.Fatalf("poll_oneoff for data reported userdata %d, want 1", userdata)
	}

	// After the peer closes, the read reports a hangup.
	_ = p.sysClose(b)
	if errno := p.wasiPollOneoff(ctx, mem, inPtr, outPtr, 1, neventsPtr); errno != wasiErrnoSuccess {
		t.Fatalf("poll_oneoff after close = %d", errno)
	}
	if flags, _ := mem.Read(outPtr+24, 2); binary.LittleEndian.Uint16(flags) != wasiEventrwflagsHangup {
		t.Fatalf("poll_oneoff after close reported flags %v, want hangup", flags)
	}
}

// Guest memory addresses the descriptor passing tests use.
const (
	rightsTestMsg     = 4096
	rightsTestIov     = 4160
	rightsTestData    = 4200
	rightsTestControl = 4400
	rightsTestPipe    = 4600
)

// pairTest connects a socket pair and returns its descriptors.
func pairTest(t *testing.T, p *Process, mem *memMemory) (uint32, uint32) {
	// Connect the pair and read its descriptors.
	t.Helper()
	if res := p.sysSocketpair(guestAFUNIX, guestSockStream, 0, sockTestPair); res != 0 {
		t.Fatalf("socketpair = %#x", res)
	}
	a, _ := mem.ReadUint32Le(sockTestPair)
	b, _ := mem.ReadUint32Le(sockTestPair + 4)
	return a, b
}

// pipeTest makes a pipe and returns its read and write descriptors.
func pipeTest(t *testing.T, p *Process, mem *memMemory) (uint32, uint32) {
	// Make the pipe and read its descriptors.
	t.Helper()
	if res := p.sysPipe2(rightsTestPipe, 0); res != 0 {
		t.Fatalf("pipe2 = %#x", res)
	}
	r, _ := mem.ReadUint32Le(rightsTestPipe)
	w, _ := mem.ReadUint32Le(rightsTestPipe + 4)
	return r, w
}

// writeTestMsghdr writes a msghdr at rightsTestMsg with one iovec of length
// dataLen at rightsTestData and a control buffer of controlLen bytes.
func writeTestMsghdr(mem *memMemory, dataLen, controlLen uint32) {
	// Point the header at one iovec and the control buffer.
	mem.Write(rightsTestMsg, make([]byte, 28))
	mem.WriteUint32Le(rightsTestIov, rightsTestData)
	mem.WriteUint32Le(rightsTestIov+4, dataLen)
	mem.WriteUint32Le(rightsTestMsg+msghdrIov, rightsTestIov)
	mem.WriteUint32Le(rightsTestMsg+msghdrIovlen, 1)
	mem.WriteUint32Le(rightsTestMsg+msghdrControl, rightsTestControl)
	mem.WriteUint32Le(rightsTestMsg+msghdrControllen, controlLen)
}

// sendRightsTest sends data on fd with fds in one SCM_RIGHTS message.
func sendRightsTest(t *testing.T, p *Process, mem *memMemory, fd uint32, data string, fds ...uint32) {
	// Write the data and the control message.
	t.Helper()
	mem.Write(rightsTestData, []byte(data))
	var control []byte
	if len(fds) != 0 {
		control = binary.LittleEndian.AppendUint32(nil, cmsghdrSize+4*uint32(len(fds)))
		control = binary.LittleEndian.AppendUint32(control, guestSolSocket)
		control = binary.LittleEndian.AppendUint32(control, guestScmRights)
		for _, sent := range fds {
			control = binary.LittleEndian.AppendUint32(control, sent)
		}
		mem.Write(rightsTestControl, control)
	}
	writeTestMsghdr(mem, uint32(len(data)), uint32(len(control)))

	// Send them.
	if res := p.sysSendmsg(context.Background(), fd, rightsTestMsg, 0); res != uint64(len(data)) {
		t.Fatalf("sendmsg(%d, %q) = %#x", fd, data, res)
	}
}

// recvRightsTest receives up to 64 bytes from fd with a control buffer of
// controlLen bytes, and returns the data, the descriptors received and the
// message flags.
func recvRightsTest(t *testing.T, p *Process, mem *memMemory, fd, controlLen uint32) (string, []uint32, uint32) {
	// Receive.
	t.Helper()
	writeTestMsghdr(mem, 64, controlLen)
	res := p.sysRecvmsg(context.Background(), fd, rightsTestMsg, 0)
	if errnoFailed(res) {
		t.Fatalf("recvmsg(%d) = %#x", fd, res)
	}
	data, _ := mem.Read(rightsTestData, uint32(res))

	// Read the descriptors of the control message.
	var fds []uint32
	n, _ := mem.ReadUint32Le(rightsTestMsg + msghdrControllen)
	if n != 0 {
		msgLen, _ := mem.ReadUint32Le(rightsTestControl)
		if msgLen != n {
			t.Fatalf("control message length %d, control length %d", msgLen, n)
		}
		for off := uint32(cmsghdrSize); off < n; off += 4 {
			got, _ := mem.ReadUint32Le(rightsTestControl + off)
			fds = append(fds, got)
		}
	}
	flags, _ := mem.ReadUint32Le(rightsTestMsg + msghdrFlags)
	return string(data), fds, flags
}

// TestUnixSocketRights checks descriptors sent with SCM_RIGHTS arrive with
// the bytes they were sent with, after the sender closed its own, and that
// a receive never crosses into a later send's descriptors.
func TestUnixSocketRights(t *testing.T) {
	// Send a pipe's ends after plain data, and close the sender's copies.
	p, mem := fsTestProcess()
	a, b := pairTest(t, p, mem)
	r, w := pipeTest(t, p, mem)
	sendTest(t, p, mem, a, "ab")
	sendRightsTest(t, p, mem, a, "cd", r, w)
	_ = p.sysClose(r)
	_ = p.sysClose(w)

	// The first receive stops before the descriptors.
	data, fds, _ := recvRightsTest(t, p, mem, b, 64)
	if data != "ab" || len(fds) != 0 {
		t.Fatalf("first recvmsg = %q with %v, want ab alone", data, fds)
	}
	data, fds, flags := recvRightsTest(t, p, mem, b, 64)
	if data != "cd" || len(fds) != 2 || flags != 0 {
		t.Fatalf("second recvmsg = %q with %v and flags %#x, want cd with 2", data, fds, flags)
	}

	// The received ends are the pipe.
	sendTest(t, p, mem, fds[1], "hi")
	if _, got := recvTest(p, mem, fds[0]); got != "hi" {
		t.Fatalf("pipe read %q, want hi", got)
	}
}

// TestUnixSocketRightsDropped checks the files a receiver does not take are
// released: by a plain read, beyond a short control buffer with MSG_CTRUNC,
// and in flight when the receiving end closes.
func TestUnixSocketRightsDropped(t *testing.T) {
	// A plain read drops the pipe's write end, so the pipe ends.
	p, mem := fsTestProcess()
	a, b := pairTest(t, p, mem)
	r, w := pipeTest(t, p, mem)
	sendRightsTest(t, p, mem, a, "x", w)
	_ = p.sysClose(w)
	if _, got := recvTest(p, mem, b); got != "x" {
		t.Fatalf("read %q, want x", got)
	}
	if res, _ := recvTest(p, mem, r); res != 0 {
		t.Fatalf("pipe read after a dropped write end = %#x, want end of file", res)
	}
	_ = p.sysClose(r)

	// A control buffer with room for one of two descriptors keeps the
	// first descriptor.
	r, w = pipeTest(t, p, mem)
	sendRightsTest(t, p, mem, a, "y", w, r)
	_ = p.sysClose(w)
	_ = p.sysClose(r)
	data, fds, flags := recvRightsTest(t, p, mem, b, cmsghdrSize+4)
	if data != "y" || len(fds) != 1 || flags != guestMsgCtrunc {
		t.Fatalf("short recvmsg = %q with %v and flags %#x, want y with 1 and MSG_CTRUNC", data, fds, flags)
	}
	if res := p.sysWrite(context.Background(), fds[0], sockTestBuf, 1); res != negErrno(EPIPE) {
		t.Fatalf("write to the pipe without its read end = %#x, want EPIPE", res)
	}
	_ = p.sysClose(fds[0])

	// A send of no bytes drops its files.
	r, w = pipeTest(t, p, mem)
	sendRightsTest(t, p, mem, a, "", w)
	_ = p.sysClose(w)
	if res, _ := recvTest(p, mem, r); res != 0 {
		t.Fatalf("pipe read after an empty send of its write end = %#x, want end of file", res)
	}
	_ = p.sysClose(r)

	// Closing the receiving end releases the files in flight.
	r, w = pipeTest(t, p, mem)
	sendRightsTest(t, p, mem, a, "z", w)
	_ = p.sysClose(w)
	_ = p.sysClose(b)
	if res, _ := recvTest(p, mem, r); res != 0 {
		t.Fatalf("pipe read after its write end closed in flight = %#x, want end of file", res)
	}
}

// TestUnixSocketRightsErrors checks a send rejects control messages other
// than SCM_RIGHTS and unknown descriptors without sending.
func TestUnixSocketRightsErrors(t *testing.T) {
	// Build a control message of another type.
	p, mem := fsTestProcess()
	a, _ := pairTest(t, p, mem)
	control := binary.LittleEndian.AppendUint32(nil, cmsghdrSize+4)
	control = binary.LittleEndian.AppendUint32(control, guestSolSocket)
	control = binary.LittleEndian.AppendUint32(control, guestScmRights+1)
	control = binary.LittleEndian.AppendUint32(control, a)
	mem.Write(rightsTestControl, control)
	writeTestMsghdr(mem, 1, uint32(len(control)))

	// Send it.
	if res := p.sysSendmsg(context.Background(), a, rightsTestMsg, 0); res != negErrno(EINVAL) {
		t.Fatalf("sendmsg with SCM_CREDENTIALS = %#x, want EINVAL", res)
	}

	// Send an unknown descriptor.
	binary.LittleEndian.PutUint32(control[8:], guestScmRights)
	binary.LittleEndian.PutUint32(control[12:], 99)
	mem.Write(rightsTestControl, control)
	if res := p.sysSendmsg(context.Background(), a, rightsTestMsg, 0); res != negErrno(EBADF) {
		t.Fatalf("sendmsg of an unknown descriptor = %#x, want EBADF", res)
	}
}

// Guest memory addresses the getsockopt tests use.
const (
	sockTestOpt    = 5000
	sockTestOptLen = 5100
)

// peerCredTest returns the SO_PEERCRED credentials of fd through the
// kernel's getsockopt import, or the failing result.
func peerCredTest(t *testing.T, p *Process, mem *memMemory, fd uint32) (ucred, uint64) {
	// Call the import the guest calls.
	t.Helper()
	mem.Write(sockTestOpt, make([]byte, ucredSize))
	mem.WriteUint32Le(sockTestOptLen, ucredSize)
	args := []uint64{uint64(fd), guestSolSocket, guestSoPeercred, sockTestOpt, sockTestOptLen}
	if res := posixFunctions()["getsockopt"].call(context.Background(), p, args); res != 0 {
		// The import reports failure as -1 and sets the guest's errno, which
		// this process lacks, so ask the call behind it for the errno.
		if res != fail {
			t.Fatalf("getsockopt import = %#x, want 0 or -1", res)
		}
		return ucred{}, p.sysGetsockopt(fd, guestSolSocket, guestSoPeercred, sockTestOpt, sockTestOptLen)
	}

	// Read the struct ucred and its length.
	if n, _ := mem.ReadUint32Le(sockTestOptLen); n != ucredSize {
		t.Fatalf("SO_PEERCRED length = %d, want %d", n, ucredSize)
	}
	pid, _ := mem.ReadUint32Le(sockTestOpt)
	uid, _ := mem.ReadUint32Le(sockTestOpt + 4)
	gid, _ := mem.ReadUint32Le(sockTestOpt + 8)
	return ucred{pid: pid, uid: uid, gid: gid}, 0
}

// asTest gives p the process id and ids a later call runs under.
func asTest(p *Process, pid int, uid, gid uint32) {
	p.pid = pid
	p.uids.set(uid)
	p.gids.set(gid)
}

// TestSocketPeerCred checks SO_PEERCRED on a socket pair reports the process
// that made it, on a connection reports the other end's credentials as they
// were when it listened and connected, and fails on a socket with no peer.
func TestSocketPeerCred(t *testing.T) {
	// A pair reports the process that made it, at both ends.
	p, mem := fsTestProcess()
	asTest(p, 10, 1000, 100)
	a, b := pairTest(t, p, mem)
	asTest(p, 11, 2000, 200)
	for _, fd := range []uint32{a, b} {
		if got, errno := peerCredTest(t, p, mem, fd); errno != 0 || got != (ucred{10, 1000, 100}) {
			t.Fatalf("socketpair SO_PEERCRED = %v, %#x, want {10 1000 100}", got, errno)
		}
	}

	// A server listens, a client connects and the server accepts.
	asTest(p, 20, 3000, 300)
	listener := listenTestSocket(t, p, mem, "/tmp/sock", guestSockStream)
	asTest(p, 21, 4000, 400)
	client := uint32(p.sysSocket(guestAFUNIX, guestSockStream, 0))
	if res := p.sysConnect(client, sockTestAddr, writeTestSockaddr(mem, "/tmp/sock")); res != 0 {
		t.Fatalf("connect = %#x", res)
	}
	asTest(p, 22, 5000, 500)
	server := p.sysAccept(context.Background(), listener, 0)
	if errnoFailed(server) {
		t.Fatalf("accept = %#x", server)
	}

	// Each end sees the other's credentials, whoever asks.
	if got, errno := peerCredTest(t, p, mem, client); errno != 0 || got != (ucred{20, 3000, 300}) {
		t.Fatalf("client SO_PEERCRED = %v, %#x, want {20 3000 300}", got, errno)
	}
	if got, errno := peerCredTest(t, p, mem, uint32(server)); errno != 0 || got != (ucred{21, 4000, 400}) {
		t.Fatalf("accepted SO_PEERCRED = %v, %#x, want {21 4000 400}", got, errno)
	}

	// Sockets with no peer fail with ENOTCONN.
	unconnected := uint32(p.sysSocket(guestAFUNIX, guestSockStream, 0))
	for _, fd := range []uint32{unconnected, listener} {
		if _, errno := peerCredTest(t, p, mem, fd); errno != negErrno(ENOTCONN) {
			t.Fatalf("SO_PEERCRED on fd %d with no peer = %#x, want ENOTCONN", fd, errno)
		}
	}
}
