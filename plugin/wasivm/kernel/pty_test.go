package kernel

import (
	"context"
	"encoding/binary"
	"testing"
)

// TestPtyPair checks a pseudo-terminal from open to hangup: the slave stays
// locked until unlocked, becomes the controlling terminal of a session leader
// that opens it without O_NOCTTY, runs the line discipline on what the master
// writes, signals its foreground group, carries the window size, and hangs up
// when the master closes.
func TestPtyPair(t *testing.T) {
	// Open the master and read the pty's index.
	p, mem := newWASITestProcess(t)
	p.cancel = func() {}
	for _, sig := range []uint32{SIGINT, SIGWINCH, SIGHUP} {
		p.sigActions[sig] = sigAction{handler: 0x1000}
	}
	master := openTestFD(t, p, "/dev/ptmx")
	const arg = 4096
	if res := p.sysIoctl(master, tiocgptn, arg); res != 0 {
		t.Fatalf("TIOCGPTN = %#x", res)
	}
	if index, _ := mem.ReadUint32Le(arg); index != 0 {
		t.Fatalf("pty index %d, want 0", index)
	}

	// The slave opens once unlocked. An open with O_NOCTTY leaves it free,
	// and one without makes it the controlling terminal.
	if res := p.openResolved("/dev/pts/0", oRDWR, 0); res != negErrno(EIO) {
		t.Fatalf("open of a locked slave = %#x, want EIO", res)
	}
	mem.WriteUint32Le(arg, 0)
	if res := p.sysIoctl(master, tiocsptlck, arg); res != 0 {
		t.Fatalf("TIOCSPTLCK = %#x", res)
	}
	if res := p.openResolved("/dev/pts/0", oRDWR|oNOCTTY, 0); errnoFailed(res) || p.ctty != nil {
		t.Fatalf("open with O_NOCTTY = %#x, controlling terminal %v", res, p.ctty)
	}
	slave := openTestFD(t, p, "/dev/pts/0")
	if p.ctty == nil || p.ctty != p.fds[slave].tty {
		t.Fatal("slave is not the controlling terminal")
	}
	if devTTY := openTestFD(t, p, "/dev/tty"); p.fds[devTTY].tty != p.ctty {
		t.Fatal("/dev/tty does not name the slave")
	}

	// A line the master types reaches the slave edited, and its echo reaches
	// the master.
	writeTestFD(t, p, master, "hellx\x7fo\r")
	if got := readTestFD(t, p, slave); got != "hello\n" {
		t.Fatalf("slave read %q, want %q", got, "hello\n")
	}
	if got, want := readTestFD(t, p, master), "hellx\b \bo\r\n"; got != want {
		t.Fatalf("master read %q, want %q", got, want)
	}
	writeTestFD(t, p, slave, "out\n")
	if got := readTestFD(t, p, master); got != "out\r\n" {
		t.Fatalf("master read %q, want %q", got, "out\r\n")
	}

	// ^C signals the foreground group.
	writeTestFD(t, p, master, "\x03")
	if p.sigCaught&sigBit(SIGINT) == 0 {
		t.Fatal("^C sent no SIGINT")
	}

	// A resize on the master reaches the slave with SIGWINCH.
	var ws [8]byte
	binary.LittleEndian.PutUint16(ws[0:], 40)
	binary.LittleEndian.PutUint16(ws[2:], 100)
	mem.Write(arg, ws[:])
	if res := p.sysIoctl(master, tiocswinsz, arg); res != 0 {
		t.Fatalf("TIOCSWINSZ = %#x", res)
	}

	// The slave reads the new size.
	mem.Write(arg, make([]byte, 8))
	if res := p.sysIoctl(slave, tiocgwinsz, arg); res != 0 {
		t.Fatalf("TIOCGWINSZ = %#x", res)
	}
	got, _ := mem.Read(arg, 4)
	if rows, cols := binary.LittleEndian.Uint16(got), binary.LittleEndian.Uint16(got[2:]); rows != 40 || cols != 100 {
		t.Fatalf("window %dx%d, want 100x40", cols, rows)
	}
	if p.sigCaught&sigBit(SIGWINCH) == 0 {
		t.Fatal("resize sent no SIGWINCH")
	}

	// Closing the master hangs up the slave.
	p.sysClose(master)
	if got := readTestFD(t, p, slave); got != "" {
		t.Fatalf("slave read %q after hangup, want end of file", got)
	}
	if res := p.writeOutBytes(context.Background(), slave, []byte("x")); res != negErrno(EIO) {
		t.Fatalf("slave write after hangup = %#x, want EIO", res)
	}
	if p.sigCaught&sigBit(SIGHUP) == 0 {
		t.Fatal("hangup sent no SIGHUP")
	}
	if p.kernel.vfs.lookup("/dev/pts/0") != nil {
		t.Fatal("/dev/pts/0 remains after hangup")
	}
}

// TestPtyMasterReadsSlaveClose checks that a master read reports EIO once
// every slave file closed, and waits again after the slave reopens.
func TestPtyMasterReadsSlaveClose(t *testing.T) {
	// Open an unlocked pair.
	p, _ := newWASITestProcess(t)
	master := openTestFD(t, p, "/dev/ptmx")
	p.fds[master].ptm.locked = false
	slave := openTestFD(t, p, "/dev/pts/0")

	// Close the slave, then reopen it.
	p.sysClose(slave)
	if _, errno := p.readFile(nonblocking(context.Background()), p.fds[master], 16); errno != negErrno(EIO) {
		t.Fatalf("master read = %#x, want EIO", errno)
	}
	openTestFD(t, p, "/dev/pts/0")
	if _, errno := p.readFile(nonblocking(context.Background()), p.fds[master], 16); errno != negErrno(EAGAIN) {
		t.Fatalf("master read = %#x, want EAGAIN", errno)
	}
}

// openTestFD opens path read-write as p and returns the descriptor.
func openTestFD(t *testing.T, p *Process, path string) uint32 {
	t.Helper()
	res := p.openResolved(path, oRDWR, 0)
	if errnoFailed(res) {
		t.Fatalf("open %s: %#x", path, res)
	}
	return uint32(res)
}

// writeTestFD writes s to fd as p.
func writeTestFD(t *testing.T, p *Process, fd uint32, s string) {
	t.Helper()
	if res := p.writeOutBytes(context.Background(), fd, []byte(s)); res != uint64(len(s)) {
		t.Fatalf("write %q to %d = %#x", s, fd, res)
	}
}

// readTestFD reads what fd holds now as p, without waiting.
func readTestFD(t *testing.T, p *Process, fd uint32) string {
	t.Helper()
	data, errno := p.readFile(nonblocking(context.Background()), p.fds[fd], 256)
	if errno != 0 {
		t.Fatalf("read %d: %#x", fd, errno)
	}
	return string(data)
}
