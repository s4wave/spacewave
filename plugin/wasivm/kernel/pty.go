package kernel

import (
	"bytes"
	"strconv"
)

// Device numbers of the pseudo-terminals: the /dev/ptmx multiplexer, and the
// major number of the /dev/pts slaves, whose minor is the pty's index.
const (
	devPtmx  = 5<<8 | 2
	ptsMajor = 136
)

// maxPtys is the number of pseudo-terminals open at once, as the minor
// number of a slave holds the index.
const maxPtys = 256

// pty is a pseudo-terminal: a master, which the program that opened
// /dev/ptmx holds, and a slave terminal at /dev/pts/N. What the master writes
// is the slave's input, and the master reads what programs write to the
// slave. The kernel lock guards its fields.
type pty struct {
	// index is N in /dev/pts/N.
	index int
	// term is the slave terminal.
	term *tty
	// out carries the slave's output to the master.
	out *bufferedPipe
	// locked is set until unlockpt, while the slave cannot be opened.
	locked bool
	// slaves counts the open files of the slave.
	slaves int
	// hungUp is set once the master has closed.
	hungUp bool
}

// openPtmx creates a pseudo-terminal and opens its master with the open
// flags, close-on-exec when cloexec is set. The slave appears at /dev/pts/N,
// locked until unlockpt.
func (p *Process) openPtmx(flags uint32, cloexec bool) uint64 {
	// Take the lowest free index.
	k := p.kernel
	index := 0
	for k.ptys[index] != nil {
		index++
	}
	if index == maxPtys {
		return negErrno(ENOSPC)
	}

	// Create the slave terminal and its node.
	pt := &pty{index: index, out: newBufferedPipe(k.vfs.allocIno()), locked: true}
	pt.term = newTTY(k, true, true)
	pt.term.pty = pt
	pt.term.node = k.vfs.mkCharDev(k.pts, strconv.Itoa(index), ptsMajor<<8|uint64(index))
	pt.term.node.mode = sIFCHR | 0o620
	pt.term.node.tty = pt.term
	k.ptys[index] = pt

	// Open the master, which reads the slave's output.
	ptmx := k.vfs.lookup("/dev/ptmx")
	of := &OpenFile{node: ptmx, path: "/dev/ptmx", flags: flags, ptm: pt, rx: pt.out}
	return uint64(p.installFD(of, cloexec))
}

// openSlave records an open of the slave. It fails with EIO while the slave
// is locked or after the master closed.
func (pt *pty) openSlave() uint64 {
	if pt.locked || pt.hungUp {
		return negErrno(EIO)
	}
	if pt.slaves == 0 {
		pt.out.reopenWrite()
	}
	pt.slaves++
	return 0
}

// closeSlave records the close of a slave's open file. Once none remains,
// the master reads end of file, which a master read reports as EIO.
func (pt *pty) closeSlave() {
	pt.slaves--
	if pt.slaves == 0 {
		pt.out.close(false)
	}
}

// write takes a write to the master as the slave's input. The signals of its
// control characters reach the slave's foreground processes. The caller
// holds the kernel lock.
func (pt *pty) write(buf []byte) uint64 {
	for _, sig := range pt.term.in.receive(bytes.Clone(buf), false) {
		pt.term.signalForeground(sig)
	}
	return uint64(len(buf))
}

// hangup hangs up the slave after the master closed: the slave's readers see
// end of file and its writers EIO, the session leader it controls receives
// SIGHUP, and its node disappears. The processes it controlled
// keep it as their dead controlling terminal.
func (pt *pty) hangup() {
	// End the slave's input and output.
	t := pt.term
	k := t.kernel
	pt.hungUp = true
	pt.out.close(true)
	t.in.hangup()

	// Tell the session leader.
	if leader := k.procs.get(t.sid); t.sid != 0 && leader != nil {
		leader.deliverSignal(SIGHUP)
	}
	t.sid, t.fgPgid = 0, 0

	// Remove the slave.
	delete(k.pts.children, strconv.Itoa(pt.index))
	delete(k.ptys, pt.index)
}

// terminal returns p's controlling terminal: its pty, or the console when
// the console is a terminal. It returns nil for a process with none.
func (p *Process) terminal() *tty {
	if p.ctty != nil {
		return p.ctty
	}
	if c := p.kernel.console; c != nil && c.terminal {
		return c
	}
	return nil
}
