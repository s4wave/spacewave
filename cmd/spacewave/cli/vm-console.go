//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"golang.org/x/term"

	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

func newVmConsoleCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "console",
		Usage:     "attach a terminal to the console of a running WebAssembly VM",
		ArgsUsage: "<vm-key>",
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			uri := fsURI{
				sessionIdx: sessionIndex32(*sessionIdx),
				spaceID:    *spaceID,
				objectKey:  key,
			}
			return attachWasiVmConsole(c, *statePath, uri)
		},
	}
}

// attachWasiVmConsole streams the console of the WasiVM at uri to the
// terminal until the VM exits or the user quits with Ctrl-A x. The console
// replays the recent output first, so a late attach shows the prompt.
func attachWasiVmConsole(c *cli.Context, statePath string, uri fsURI) error {
	// Refuse a stopped VM, whose console would wait for a start.
	if err := requireVMStartRequested(c, statePath, uri.sessionIdx, uri.spaceID, uri.objectKey); err != nil {
		return err
	}

	// Open the console of the VM's typed resource.
	mount, err := mountObjectChain(c, statePath, uri, nil)
	if err != nil {
		return err
	}
	defer mount.release()
	ctx, cancel := context.WithCancel(c.Context)
	defer cancel()
	strm, err := s4wave_vm.NewSRPCWasiVmConsoleServiceClient(mount.TypedClient).Console(ctx)
	if err != nil {
		return errors.Wrap(err, "open VM console")
	}
	defer strm.Close()

	// Put the terminal in raw mode and send its size.
	fd := int(os.Stdin.Fd())
	open := &s4wave_terminal.TerminalFrame{Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN}
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return errors.Wrap(err, "set terminal raw mode")
		}
		defer term.Restore(fd, state)
		cols, rows, err := term.GetSize(fd)
		if err == nil && cols > 0 && rows > 0 && cols <= math.MaxUint16 && rows <= math.MaxUint16 {
			open.Cols, open.Rows = uint32(cols), uint32(rows)
		}
		os.Stderr.WriteString("console: attached to " + uri.objectKey + "; press Ctrl-A x to detach\r\n")
	}
	if err := strm.Send(open); err != nil {
		return errors.Wrap(err, "open VM console")
	}

	// Forward input until stdin ends. The VM keeps running after stdin
	// ends.
	go func() {
		in := &serialEscapeReader{src: os.Stdin, quit: cancel}
		buf := make([]byte, 4096)
		for {
			n, err := in.Read(buf)
			if n != 0 {
				sendErr := strm.Send(&s4wave_terminal.TerminalFrame{
					Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT,
					Data: buf[:n],
				})
				if sendErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Print the output until the VM exits.
	for {
		frame, err := strm.Recv()
		switch {
		case ctx.Err() != nil:
			os.Stderr.WriteString("\r\n")
			return nil
		case stderrors.Is(err, io.EOF):
			return errors.New("console closed before the VM exited")
		case err != nil:
			return errors.Wrap(err, "read VM console")
		}
		switch frame.GetKind() {
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT:
			if _, err := os.Stdout.Write(frame.GetData()); err != nil {
				return err
			}
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT:
			if msg := frame.GetError(); msg != "" {
				return errors.New("VM exited: " + msg)
			}
			if code := frame.GetExitCode(); code != 0 {
				return errors.New("VM exited with status " + strconv.Itoa(int(code)))
			}
			return nil
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR:
			return errors.New("VM console: " + frame.GetError())
		}
	}
}
