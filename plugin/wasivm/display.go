package wasivm_plugin

import (
	"context"
	"image"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/plugin/wasivm/display"
	"github.com/s4wave/spacewave/plugin/wasivm/wayland"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// vmDisplay serves the display of one VM on each of its resources. A viewer
// that attaches before the VM runs, or before the guest brings up a display,
// waits for both.
type vmDisplay struct {
	consoles  *consoleTable
	objectKey string
}

// screen is a display a viewer attaches to: X display 0 or the Wayland
// compositor.
type screen interface {
	// Input injects a pointer or key event.
	Input(in *s4wave_vm.WasiVmDisplayInput)
	// Run streams the whole scene, then each change, until the display or
	// ctx ends. Changes made while send blocks go in the next update.
	Run(ctx context.Context, send func(*s4wave_vm.WasiVmScene) error) error
}

// attached is a screen with the function that detaches from it, or the error
// that kept the viewer from attaching.
type attached struct {
	screen  screen
	release func()
	err     error
}

// Display attaches to the display of the running VM, streams its screen to
// the viewer and injects the viewer's input, until the display ends or the
// viewer leaves.
func (v *vmDisplay) Display(strm s4wave_vm.SRPCWasiVmDisplayService_DisplayStream) error {
	// Wait for the VM to run and bring up a display.
	ctx, cancel := context.WithCancel(strm.Context())
	defer cancel()
	c, err := v.consoles.wait(ctx, v.objectKey)
	if err != nil {
		return err
	}
	a := c.attachScreen(ctx)
	if a.err != nil {
		return a.err
	}
	defer a.release()

	return serveScreen(ctx, strm.Send, strm.Recv, a.screen)
}

// WatchWindows streams the windows of the running VM's Wayland display, then
// the list again each time it changes, until the display ends or the viewer
// leaves. A VM without a Wayland display sends an empty list.
func (v *vmDisplay) WatchWindows(_ *s4wave_vm.WatchWasiVmWindowsRequest, strm s4wave_vm.SRPCWasiVmDisplayService_WatchWindowsStream) error {
	// Wait for the VM to run.
	ctx := strm.Context()
	c, err := v.consoles.wait(ctx, v.objectKey)
	if err != nil {
		return err
	}

	// Stream its compositor's windows, or none.
	if c.wayland == nil {
		if err := strm.Send(&s4wave_vm.WasiVmWindowList{}); err != nil {
			return err
		}
		<-ctx.Done()
		return context.Cause(ctx)
	}
	return c.wayland.WatchWindows(ctx, strm.Send)
}

// Window pops one window of the running VM's Wayland display out to the
// viewer at the viewer's size, streams it and injects the viewer's input,
// until the window closes or the viewer leaves. The first input opens the
// window. Subsequent inputs resize the window or carry input. The stream
// ends without an error when the window closes, and the window returns to
// the desktop when the viewer leaves.
func (v *vmDisplay) Window(strm s4wave_vm.SRPCWasiVmDisplayService_WindowStream) error {
	// Read which window to open.
	ctx := strm.Context()
	first, err := strm.Recv()
	if err != nil {
		return err
	}
	open := first.GetOpen()
	if open == nil {
		return errors.New("the first window input must open a window")
	}

	// Open it on the running VM's compositor.
	c, err := v.consoles.wait(ctx, v.objectKey)
	if err != nil {
		return err
	}
	if c.wayland == nil {
		return nil
	}
	win, err := c.wayland.OpenWindow(open.GetId(), windowSize(open.GetSize()))
	if errors.Is(err, wayland.ErrWindowClosed) {
		return nil
	}
	if err != nil {
		return err
	}
	defer win.Close()

	// Stream it, applying each resize as it comes.
	recv := func() (*s4wave_vm.WasiVmDisplayInput, error) {
		for {
			in, err := strm.Recv()
			if err != nil {
				return nil, err
			}
			if size := in.GetResize(); size != nil {
				win.Resize(windowSize(size))
			} else if in := in.GetInput(); in != nil {
				return in, nil
			}
		}
	}
	err = serveScreen(ctx, strm.Send, recv, win)
	if errors.Is(err, wayland.ErrWindowClosed) {
		return nil
	}
	return err
}

// windowSize returns a viewer's window size as a point.
func windowSize(s *s4wave_vm.WasiVmWindowSize) image.Point {
	return image.Pt(int(s.GetWidth()), int(s.GetHeight()))
}

// framesAhead is how many scene updates the viewer may have been sent but
// not yet drawn. Two let the screen build an update while the viewer draws
// the last update.
const framesAhead = 2

// serveScreen streams the screen to the viewer through send and injects the
// input recv returns, until either ends. It holds an update while
// framesAhead are undrawn, so the screen folds the changes made meanwhile into
// the next update.
func serveScreen(
	ctx context.Context,
	send func(*s4wave_vm.WasiVmScene) error,
	recv func() (*s4wave_vm.WasiVmDisplayInput, error),
	scr screen,
) error {
	// End the screen when the viewer leaves, and the input loop with it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Inject the viewer's input until it leaves, and pass on its reports of
	// drawn updates.
	drawn := make(chan struct{}, framesAhead)
	go func() {
		defer cancel()
		for {
			in, err := recv()
			if err != nil {
				return
			}
			if in.GetDrawn() != nil {
				select {
				case drawn <- struct{}{}:
				default:
				}
				continue
			}
			scr.Input(in)
		}
	}()

	// Stream the scene, holding the next update while too many are undrawn.
	undrawn := 0
	return scr.Run(ctx, func(update *s4wave_vm.WasiVmScene) error {
		if err := send(update); err != nil {
			return err
		}
		if undrawn++; undrawn < framesAhead {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-drawn:
			undrawn--
			return nil
		}
	})
}

// attachScreen waits for the first display the guest brings up: X display 0
// once its server listens, or the Wayland compositor once a client connects.
// It detaches from the other.
func (c *console) attachScreen(ctx context.Context) attached {
	// Wait for each display.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan attached, 2)
	waits := 1
	go func() { results <- c.attachX(waitCtx) }()
	if comp := c.wayland; comp != nil {
		waits++
		go func() {
			results <- attached{screen: comp, release: func() {}, err: comp.WaitClient(waitCtx)}
		}()
	}

	// Keep the first, then detach from the other once its wait ends.
	first := <-results
	cancel()
	for range waits - 1 {
		if late := <-results; late.err == nil {
			late.release()
		}
	}
	return first
}

// attachX connects to X display 0 once its server listens.
func (c *console) attachX(ctx context.Context) attached {
	// Connect, then greet the server as an X client.
	conn, err := c.kernel.DialUnix(ctx, display.SocketPath)
	if err != nil {
		return attached{err: err}
	}
	client, err := display.New(conn)
	if err != nil {
		_ = conn.Close()
		return attached{err: err}
	}
	return attached{screen: client, release: client.Close}
}

// _ is a type assertion.
var _ s4wave_vm.SRPCWasiVmDisplayServiceServer = (*vmDisplay)(nil)
