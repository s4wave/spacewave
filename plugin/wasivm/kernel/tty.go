package kernel

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// tty is a terminal: the console or the slave of a pseudo-terminal. It holds
// the termios state, the window size and the foreground process group, takes
// input through its line discipline and processes the output its programs
// write.
type tty struct {
	// kernel is the kernel the terminal belongs to.
	kernel *Kernel
	// node is the terminal's device node: /dev/console or /dev/pts/N.
	node *VNode
	// pty is the pseudo-terminal whose slave this is, or nil for the console.
	pty *pty
	// terminal reports whether the device is a terminal. A pty always is.
	// The console is when its host side is a terminal or it is virtual.
	terminal bool
	// virt reports whether the kernel applies the line discipline. A pty
	// always does. The console does when it is a virtual terminal.
	virt bool
	// in is the input the terminal's readers take.
	in *ttyInput
	// stdout and stderr receive the console's output. A pty's are nil.
	stdout io.Writer
	stderr io.Writer

	// fgPgid is the foreground process group, or 0 until a process sets one,
	// when every process the terminal controls is in the foreground. The
	// kernel lock guards it.
	fgPgid int
	// sid is the session a pty controls, or 0. The console controls every
	// process without a pty. The kernel lock guards it.
	sid int

	// mtx guards termIOS, which the guest sets while input is processed.
	mtx sync.Mutex
	// termIOS is the termios state.
	termIOS termios
	// cols and rows are the window size.
	cols atomic.Uint32
	rows atomic.Uint32
}

// newTTY creates a terminal of k with an 80x24 window and the cooked-mode
// termios. The caller sets its node.
func newTTY(k *Kernel, terminal, virt bool) *tty {
	// Create the terminal with its input, size and modes.
	t := &tty{kernel: k, terminal: terminal, virt: virt}
	t.in = newTTYInput(t)
	t.cols.Store(80)
	t.rows.Store(24)
	t.initTermios()
	return t
}

// initTermios sets the termios to the Linux cooked-mode defaults.
func (t *tty) initTermios() {
	// A console that is not a terminal has no termios.
	if !t.terminal {
		return
	}

	// Translate input CR to NL and output NL to CRNL, run at 38400 baud
	// with 8-bit characters, and enable signals, canonical editing and echo.
	var tio termios
	tio.setFlags(icrnl, opost|onlcr, 0x04bf, 0x8a3b)
	copy(tio.cc(), defaultControlChars[:])
	t.setTermios(tio)
}

// defaultControlChars are the Linux default control characters.
var defaultControlChars = [...]byte{
	vintr:    0x03, // ^C
	vquit:    0x1c, // ^\
	verase:   0x7f, // DEL
	vkill:    0x15, // ^U
	veof:     0x04, // ^D
	vmin:     1,
	vstart:   0x11, // ^Q
	vstop:    0x13, // ^S
	vsusp:    0x1a, // ^Z
	vreprint: 0x12, // ^R
	vdiscard: 0x0f, // ^O
	vwerase:  0x17, // ^W
	vlnext:   0x16, // ^V
}

// termios returns the termios state.
func (t *tty) termios() termios {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.termIOS
}

// setTermios replaces the termios state. The console's host terminal follows
// its canonical mode.
func (t *tty) setTermios(tio termios) {
	t.mtx.Lock()
	t.termIOS = tio
	t.mtx.Unlock()
	if t == t.kernel.console {
		t.kernel.syncHostTermMode()
	}
}

// syncHostTermMode puts a host terminal in raw mode while the console's
// ICANON bit is clear and restores it while the bit is set.
func (k *Kernel) syncHostTermMode() {
	if k.termFD < 0 {
		return
	}

	// Switch the host terminal when its mode differs.
	tio := k.console.termios()
	wantRaw := tio.lflag()&icanon == 0
	switch {
	case wantRaw && !k.termRaw.Load() && k.termMakeRaw != nil:
		state, err := k.termMakeRaw(k.termFD)
		if err == nil {
			k.termOldState = state
			k.termRaw.Store(true)
		}
	case !wantRaw && k.termRaw.Load():
		if k.termOldState != nil && k.termRestore != nil {
			_ = k.termRestore(k.termFD, k.termOldState)
		}
		k.termRaw.Store(false)
	}
}

// The wasi-libc termios struct holds the four mode words and the line
// discipline at the kernel's offsets, then 32 control characters and the
// input and output speeds.
const (
	// termiosCC is the offset of c_cc.
	termiosCC = 17
	// termiosNCCS is the length of c_cc.
	termiosNCCS = 32
	// termiosIspeed is the offset of c_ispeed.
	termiosIspeed = 52
	// termiosOspeed is the offset of c_ospeed.
	termiosOspeed = 56
	// termiosB38400 is the speed code for 38400 baud.
	termiosB38400 = 0o17
)

// writeWasiTermios writes the termios state to the wasi-libc termios struct
// at ptr. Control characters past the kernel's 19 are zero.
func (t *tty) writeWasiTermios(mem engine.Memory, ptr uint32) {
	// Write the modes, the control characters and the speeds.
	tio := t.termios()
	mem.Write(ptr, tio[:termiosCC])
	var cc [termiosNCCS]byte
	copy(cc[:], tio.cc())
	mem.Write(ptr+termiosCC, cc[:])
	mem.WriteUint32Le(ptr+termiosIspeed, termiosB38400)
	mem.WriteUint32Le(ptr+termiosOspeed, termiosB38400)
}

// readWasiTermios sets the termios state from the wasi-libc termios struct at
// ptr.
func (t *tty) readWasiTermios(mem engine.Memory, ptr uint32) {
	// Read the modes and control characters, then apply them.
	buf, ok := mem.Read(ptr, uint32(len(termios{})))
	if !ok {
		return
	}
	var tio termios
	copy(tio[:], buf)
	t.setTermios(tio)
}

// size returns the window size. A console on a host terminal reports the
// host terminal's size.
func (t *tty) size() (cols, rows uint32) {
	k := t.kernel
	if t == k.console && k.termFD >= 0 && k.termGetSize != nil {
		if c, r, err := k.termGetSize(k.termFD); err == nil {
			return uint32(c), uint32(r)
		}
	}
	return t.cols.Load(), t.rows.Load()
}

// resize sets the window size and sends SIGWINCH to the foreground processes
// when it changed. The caller holds the kernel lock.
func (t *tty) resize(cols, rows uint32) {
	oldCols, oldRows := t.cols.Swap(cols), t.rows.Swap(rows)
	if oldCols == cols && oldRows == rows {
		return
	}
	t.signalForeground(SIGWINCH)
}

// controls reports whether the terminal controls p: a pty controls the
// processes whose controlling terminal it is, and the console every process
// without one.
func (t *tty) controls(p *Process) bool {
	if t.pty == nil {
		return p.ctty == nil
	}
	return p.ctty == t
}

// foreground returns the foreground process group as p sees it.
func (t *tty) foreground(p *Process) int {
	if t.fgPgid == 0 {
		return p.pgid
	}
	return t.fgPgid
}

// signalForeground sends sig to the foreground process group, as a terminal
// sends the signal of a typed character whether or not anything reads it.
// Before a process sets a foreground group, every process the terminal
// controls is in the foreground, and each one's disposition decides what the
// signal does. The caller holds the kernel lock.
func (t *tty) signalForeground(sig uint32) {
	for _, p := range t.kernel.procs.filter(func(p *Process) bool {
		return t.controls(p) && (t.fgPgid == 0 || p.pgid == t.fgPgid)
	}) {
		p.deliverSignal(sig)
	}
}

// write processes buf as terminal output and writes it: to the console's
// stdout, or its stderr when stderr is set, or to a pty's master, waiting
// while the master's queue is full until ctx ends. It returns the bytes of buf
// written, and reports false once a pty's master has closed.
func (t *tty) write(ctx context.Context, buf []byte, stderr bool) (int, bool) {
	// Write the console's host stream, which processes the output.
	if t.pty == nil {
		w := t.stdout
		if stderr {
			w = t.stderr
		}
		n, _ := w.Write(buf)
		return n, true
	}

	// Queue the processed output for the master.
	out := t.processOutput(buf)
	n, ok := t.pty.out.write(ctx, out, nil)
	return inputBytes(len(buf), n, len(out)), ok
}

// echo writes the line discipline's echo of input. A pty drops what does not
// fit in the master's queue, as Linux does, so a master that writes input
// without reading output never waits on its own echo.
func (t *tty) echo(b []byte) {
	if t.pty == nil {
		_, _ = t.stdout.Write(b)
		return
	}
	_, _ = t.pty.out.write(nonblocking(context.Background()), t.processOutput(b), nil)
}

// processOutput translates \n to \r\n when OPOST and ONLCR are set and the
// kernel processes output: on a virtual terminal or a pty, or on a host
// terminal in raw mode. It returns buf when nothing changes.
func (t *tty) processOutput(buf []byte) []byte {
	// Pass the output through unless the kernel processes it with ONLCR set.
	if !t.virt && !t.kernel.termRaw.Load() {
		return buf
	}
	tio := t.termios()
	if oflag := tio.oflag(); oflag&opost == 0 || oflag&onlcr == 0 {
		return buf
	}

	// Count the bare newlines.
	extra := 0
	for i, b := range buf {
		if b == '\n' && (i == 0 || buf[i-1] != '\r') {
			extra++
		}
	}
	if extra == 0 {
		return buf
	}

	// Put a carriage return before each bare newline.
	out := make([]byte, 0, len(buf)+extra)
	for i, b := range buf {
		if b == '\n' && (i == 0 || buf[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, b)
	}
	return out
}

// inputBytes maps the bytes written of processed output back to a count of
// the caller's bytes.
func inputBytes(origLen, written, expandedLen int) int {
	if expandedLen == 0 {
		return 0
	}
	if written >= expandedLen {
		return origLen
	}
	return written * origLen / expandedLen
}
