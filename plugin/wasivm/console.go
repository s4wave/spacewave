package wasivm_plugin

import (
	"context"
	"io"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
	"github.com/s4wave/spacewave/plugin/wasivm/wayland"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// scrollback is the output a console keeps at least for viewers that attach
// after it was written.
const scrollback = 64 * 1024

// console is the terminal of one run. It keeps the recent output for viewers
// that attach later and fans new output and the exit out to every attached
// viewer.
type console struct {
	// kernel is the run's kernel, which the display dials.
	kernel *kernel.Kernel
	// wayland is the run's Wayland compositor, or nil when it could not
	// listen.
	wayland *wayland.Compositor
	// input feeds the kernel's terminal.
	input io.Writer
	// resize sets the size of the kernel's terminal.
	resize func(cols, rows uint32)
	// downloads watches the CDN reads of the run's root filesystem.
	downloads *downloads

	// bcast guards the fields below and wakes viewers when they change.
	bcast broadcast.Broadcast
	// out holds the most recent output, which ends at offset end of the
	// output stream. Bytes within len(out) are never rewritten, so a viewer
	// may send a subslice after releasing the lock.
	out []byte
	// end is the number of bytes written since the run began.
	end uint64
	// exited is set once the kernel has exited.
	exited bool
	// exitCode is the exit code of init.
	exitCode int32
	// exitError describes a run that failed instead of exiting.
	exitError string
}

// Write appends kernel output to the console.
func (c *console) Write(p []byte) (int, error) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.out = append(c.out, p...)
		c.end += uint64(len(p))
		if len(c.out) > 2*scrollback {
			c.out = append([]byte(nil), c.out[len(c.out)-scrollback:]...)
		}
		broadcast()
	})
	return len(p), nil
}

// exit records how the run ended and tells the attached viewers.
func (c *console) exit(code int32, runErr error) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.exited = true
		c.exitCode = code
		if runErr != nil {
			c.exitError = runErr.Error()
		}
		broadcast()
	})
}

// Console attaches a viewer. It replays the retained output, then streams new
// output until the VM exits or the viewer closes. Input and size changes from
// the viewer go to the kernel's terminal.
func (c *console) Console(strm s4wave_vm.SRPCWasiVmConsoleService_ConsoleStream) error {
	// Tell the viewer the console is ready.
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	if err := strm.Send(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY,
	}); err != nil {
		return err
	}

	// Forward viewer input and size until the viewer closes.
	go func() {
		defer cancel()
		for {
			frame, err := strm.Recv()
			if err != nil {
				return
			}
			switch frame.GetKind() {
			case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT:
				if _, err := c.input.Write(frame.GetData()); err != nil {
					return
				}
			case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
				s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE:
				if frame.GetCols() != 0 && frame.GetRows() != 0 {
					c.resize(frame.GetCols(), frame.GetRows())
				}
			case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
				return
			}
		}
	}()

	// Send output from the oldest retained byte until the VM exits.
	var next uint64
	for {
		// Take the output the viewer has not seen, and the exit once the
		// VM has exited.
		var data []byte
		var exit *s4wave_terminal.TerminalFrame
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			// Skip output that left the scrollback before the viewer saw it.
			start := c.end - uint64(len(c.out))
			next = max(next, start)
			data = c.out[next-start:]
			next = c.end
			if c.exited {
				exit = &s4wave_terminal.TerminalFrame{
					Kind:     s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT,
					ExitCode: c.exitCode,
					Error:    c.exitError,
				}
			}
			waitCh = getWaitCh()
		})

		// Send the new output. Once the program has exited, send the exit
		// frame and return; otherwise wait for more output.
		if len(data) != 0 {
			if err := strm.Send(&s4wave_terminal.TerminalFrame{
				Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT,
				Data: data,
			}); err != nil {
				return err
			}
		}
		if exit != nil {
			return strm.Send(exit)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// consoleTable holds the console of each running VM by object key. Each
// Space loads its own plugin instance, so an object key identifies one VM.
type consoleTable struct {
	// bcast guards consoles and wakes callers waiting for a console.
	bcast broadcast.Broadcast
	// consoles maps the object key of each running VM to its console.
	consoles map[string]*console
}

// newConsoleTable constructs an empty consoleTable.
func newConsoleTable() *consoleTable {
	return &consoleTable{consoles: make(map[string]*console)}
}

// publish makes c the console of the VM objectKey until the returned func
// is called.
func (t *consoleTable) publish(objectKey string, c *console) func() {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.consoles[objectKey] = c
		broadcast()
	})
	return func() {
		t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if t.consoles[objectKey] == c {
				delete(t.consoles, objectKey)
				broadcast()
			}
		})
	}
}

// wait returns the console of the VM objectKey, waiting for the VM to run.
func (t *consoleTable) wait(ctx context.Context, objectKey string) (*console, error) {
	for {
		var c *console
		var waitCh <-chan struct{}
		t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			c = t.consoles[objectKey]
			waitCh = getWaitCh()
		})
		if c != nil {
			return c, nil
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// vmConsole serves the console of one VM on each of its resources. A viewer
// that attaches while the VM is not running waits for it to start.
type vmConsole struct {
	consoles  *consoleTable
	objectKey string
}

// Console attaches the viewer to the console of the running VM.
func (v *vmConsole) Console(strm s4wave_vm.SRPCWasiVmConsoleService_ConsoleStream) error {
	c, err := v.consoles.wait(strm.Context(), v.objectKey)
	if err != nil {
		return err
	}
	return c.Console(strm)
}

// _ is a type assertion.
var (
	_ s4wave_vm.SRPCWasiVmConsoleServiceServer = (*console)(nil)
	_ s4wave_vm.SRPCWasiVmConsoleServiceServer = (*vmConsole)(nil)
)
