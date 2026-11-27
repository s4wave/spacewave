package kernel

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

// ttyInput is a terminal's input queue and its input layer. With ISIG set it
// removes the signal characters and returns their signals, and on a virtual
// terminal or a pty it applies the line discipline. The console's input comes
// from a background goroutine that reads the host source, and a pty's from
// writes to its master. Buffering the input lets poll tell whether a read
// would block.
type ttyInput struct {
	// term is the terminal the input belongs to.
	term *tty
	// cancel stops the console's background goroutine.
	cancel context.CancelFunc
	// line holds the canonical line being edited. One writer at a time
	// receives input, the console's goroutine or a master write under the
	// kernel lock, so only it uses line.
	line []byte

	// mu guards the fields below.
	mu sync.Mutex
	// buf holds the input no read has taken yet.
	buf bytes.Buffer
	// eofMarks counts the end-of-file marks typed on a virtual terminal.
	// Each makes one read return end of file without ending the input.
	eofMarks int
	// eof is set when the input source has ended.
	eof bool
	// readyCh closes when a read would return at once. A read that drains
	// the buffer replaces a closed channel.
	readyCh chan struct{}
}

// newTTYInput creates the empty input of t.
func newTTYInput(t *tty) *ttyInput {
	return &ttyInput{term: t, cancel: func() {}, readyCh: make(chan struct{})}
}

// start reads r in the background until ctx ends, close is called or r
// fails.
func (in *ttyInput) start(ctx context.Context, r io.Reader) {
	ctx, in.cancel = context.WithCancel(ctx)
	go in.readLoop(ctx, r)
}

// close stops the background read.
func (in *ttyInput) close() {
	in.cancel()
}

// readLoop moves input from r into the queue and sends the signals of its
// control characters. An error from r ends the input.
func (in *ttyInput) readLoop(ctx context.Context, r io.Reader) {
	k := in.term.kernel
	dat := make([]byte, 256)
	for ctx.Err() == nil {
		n, err := r.Read(dat)
		if sigs := in.receive(dat[:n], err != nil); len(sigs) != 0 {
			k.mu.Lock()
			for _, sig := range sigs {
				in.term.signalForeground(sig)
			}
			k.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// receive runs data through the input processing into the queue, ending the
// input when eof is set. It returns the signals the control characters in
// data send, which the caller delivers under the kernel lock.
func (in *ttyInput) receive(data []byte, eof bool) []uint32 {
	// Take the signal characters, apply the discipline and queue the rest.
	data, sigs := in.takeSignalChars(data)
	eofMarks := 0
	if in.term.virt {
		data, eofMarks = in.discipline(data)
	}
	in.push(data, eofMarks, eof)
	return sigs
}

// hangup ends the input, so reads return end of file.
func (in *ttyInput) hangup() {
	in.push(nil, 0, true)
}

// pending returns the number of bytes a read could take now.
func (in *ttyInput) pending() int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.buf.Len()
}

// push adds processed input to the buffer and wakes the waiting readers.
func (in *ttyInput) push(data []byte, eofMarks int, eof bool) {
	// Buffer the input.
	in.mu.Lock()
	defer in.mu.Unlock()
	in.buf.Write(data)
	in.eofMarks += eofMarks
	in.eof = in.eof || eof

	// Wake the readers once a read would return.
	if !in.readyLocked() {
		return
	}
	select {
	case <-in.readyCh:
	default:
		close(in.readyCh)
	}
}

// read takes up to len(p) bytes of input, waiting until some arrives or ctx
// ends. It returns io.EOF at an end-of-file mark or once the input ended.
func (in *ttyInput) read(ctx context.Context, p []byte) (int, error) {
	for {
		// Take buffered input or an end of file.
		n, eof, readyCh := in.take(p)
		if n > 0 {
			return n, nil
		}
		if eof {
			return 0, io.EOF
		}

		// Wait for more input.
		if !wait(ctx, readyCh) {
			return 0, ctx.Err()
		}
	}
}

// take moves buffered input to p, or consumes an end-of-file mark. It returns
// the bytes taken, whether the read is at end of file, and the channel that
// closes when a read would next return.
func (in *ttyInput) take(p []byte) (int, bool, <-chan struct{}) {
	// Take the input.
	in.mu.Lock()
	defer in.mu.Unlock()
	n, _ := in.buf.Read(p)
	eof := in.eof
	if n == 0 && in.eofMarks > 0 {
		in.eofMarks--
		eof = true
	}

	// Replace a closed ready channel once nothing is left to read.
	if !in.readyLocked() {
		select {
		case <-in.readyCh:
			in.readyCh = make(chan struct{})
		default:
		}
	}
	return n, eof, in.readyCh
}

// ready reports whether a read would return at once, and returns the channel
// that closes when one would.
func (in *ttyInput) ready() (bool, <-chan struct{}) {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.readyLocked(), in.readyCh
}

// readyLocked reports whether a read would return at once. The caller holds
// mu.
func (in *ttyInput) readyLocked() bool {
	return in.buf.Len() > 0 || in.eofMarks > 0 || in.eof
}

// takeSignalChars removes the signal characters from data while ISIG is set
// and returns their signals, which the terminal sends at once, as Linux does
// even while no process reads the terminal. It reuses data's storage.
func (in *ttyInput) takeSignalChars(data []byte) ([]byte, []uint32) {
	// Without ISIG, every byte is input.
	tio := in.term.termios()
	if tio.lflag()&isig == 0 {
		return data, nil
	}

	// Keep the other bytes in order.
	cc := tio.cc()
	kept := data[:0]
	var sigs []uint32
	for _, b := range data {
		if sig := signalForChar(b, cc); sig != 0 {
			in.signalChar(sig, tio.lflag()&echo != 0)
			sigs = append(sigs, sig)
			continue
		}
		kept = append(kept, b)
	}
	return kept, sigs
}

// signalChar acts on a typed signal character: it echoes it and discards the
// line being edited, as the Linux terminal flushes its input queue.
func (in *ttyInput) signalChar(sig uint32, echoing bool) {
	if echoing {
		in.term.echo([]byte(signalCharEcho[sig]))
	}
	if in.term.virt {
		in.line = in.line[:0]
	}
}

// signalForChar returns the signal a terminal control character sends, or
// zero for any other byte.
func signalForChar(b byte, cc []byte) uint32 {
	switch b {
	case cc[vintr]:
		return SIGINT
	case cc[vquit]:
		return SIGQUIT
	case cc[vsusp]:
		return SIGTSTP
	}
	return 0
}

// signalCharEcho is the echo of each signal's control character.
var signalCharEcho = map[uint32]string{
	SIGINT:  "^C\n",
	SIGQUIT: "^\\\n",
	SIGTSTP: "^Z\n",
}

// discipline applies the line discipline to input whose signal characters
// are gone. It returns the bytes readers receive now and
// the number of end-of-file marks typed.
//
// In canonical mode, input collects in the edited line, which readers
// receive when a newline or VEOF ends it. VERASE removes the last character
// and VKILL the whole line. VEOF on an empty line is an end-of-file mark.
// In raw mode, input passes through. Input is echoed when ECHO is set.
func (in *ttyInput) discipline(data []byte) ([]byte, int) {
	// Apply the discipline to each byte.
	tio := in.term.termios()
	var d disciplined
	for _, b := range data {
		if b == '\r' && tio.iflag()&icrnl != 0 {
			b = '\n'
		}
		if tio.lflag()&icanon == 0 {
			d.out = append(d.out, b)
			d.echo = append(d.echo, b)
			continue
		}
		in.editLine(&d, &tio, b)
	}

	// Echo the input.
	if tio.lflag()&echo != 0 && len(d.echo) != 0 {
		in.term.echo(d.echo)
	}
	return d.out, d.eofMarks
}

// disciplined is the result of the line discipline on a run of input.
type disciplined struct {
	// out holds the bytes readers receive.
	out []byte
	// echo holds the terminal's echo when ECHO is set.
	echo []byte
	// eofMarks counts the end-of-file marks.
	eofMarks int
}

// editLine applies one byte of canonical input to the edited line. ECHOE
// echoes an erase as backspace, space, backspace.
func (in *ttyInput) editLine(d *disciplined, tio *termios, b byte) {
	cc := tio.cc()
	eraseEcho := ""
	if tio.lflag()&echoe != 0 {
		eraseEcho = "\b \b"
	}
	switch {
	case b == cc[verase] || b == '\b':
		_, size := utf8.DecodeLastRune(in.line)
		if size != 0 {
			in.line = in.line[:len(in.line)-size]
			d.echo = append(d.echo, eraseEcho...)
		}
	case b == cc[vkill]:
		d.echo = append(d.echo, strings.Repeat(eraseEcho, utf8.RuneCount(in.line))...)
		in.line = in.line[:0]
	case b == cc[veof] && len(in.line) == 0:
		d.eofMarks++
	case b == cc[veof]:
		d.out = append(d.out, in.line...)
		in.line = in.line[:0]
	case b == '\n':
		d.out = append(append(d.out, in.line...), '\n')
		d.echo = append(d.echo, '\n')
		in.line = in.line[:0]
	default:
		in.line = append(in.line, b)
		d.echo = append(d.echo, b)
	}
}
