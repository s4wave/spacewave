//go:build unix && !js

package spacewave_cli

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// startKeyListener puts the stdin terminal in raw mode and calls onKey for
// each byte typed until stop is called. stop restores the terminal and
// returns only after the reader has exited, so later reads see every byte.
// listening is false when stdin is not a terminal.
func startKeyListener(onKey func(byte)) (stop func(), listening bool) {
	// Read a nonblocking duplicate of stdin, which the runtime poller can
	// interrupt with a deadline.
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return func() {}, false
	}
	dup, err := unix.Dup(fd)
	if err != nil {
		return func() {}, false
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		_ = unix.Close(dup)
		return func() {}, false
	}
	_ = unix.SetNonblock(dup, true)
	f := os.NewFile(uintptr(dup), "stdin")

	// Deliver keys until the read fails.
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1)
		for {
			if _, err := f.Read(buf); err != nil {
				return
			}
			onKey(buf[0])
		}
	}()

	// Hand back the stop that ends the listener.
	return func() {
		// Interrupt the read, wait for the reader, then restore stdin.
		_ = f.SetReadDeadline(time.Unix(1, 0))
		<-done
		_ = f.Close()
		_ = unix.SetNonblock(fd, false)
		_ = term.Restore(fd, state)
	}, true
}
