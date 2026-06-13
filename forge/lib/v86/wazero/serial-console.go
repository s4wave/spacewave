package v86_wazero

import (
	"context"
	"errors"
	"io"
	"time"
)

// serialConsoleInputBuffer sizes the per-read chunk the input goroutine hands
// to the console goroutine.
const serialConsoleInputBuffer = 256

// RunSerialConsole drives the v86 CPU and bridges COM1 to in and out until the
// context is canceled or input ends. It runs every wasm call on this single
// goroutine: MainLoop and WriteSerialInput (which raises the UART IRQ) must
// never run concurrently, so a helper goroutine only reads in and forwards
// bytes over a channel that this goroutine drains. The next tick is scheduled
// after the delay MainLoop requests, so an idle guest does not busy-spin; this
// honors the emulator's own clock rather than polling guest state. Returns nil
// when in reaches EOF or ctx is canceled with no other error.
func (h *HostRuntime) RunSerialConsole(ctx context.Context, in io.Reader, out io.Writer) error {
	h.SetSerialSink(out)
	defer h.SetSerialSink(nil)

	inputCh := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, serialConsoleInputBuffer)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				select {
				case inputCh <- chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				select {
				case readErr <- err:
				case <-ctx.Done():
				}
				return
			}
		}
	}()

	// Start stopped and drained so every Reset below acts on a clean timer.
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		delay, err := h.MainLoop(ctx)
		if err != nil {
			return err
		}
		wait := max(time.Duration(delay*float64(time.Millisecond)), 0)
		timer.Reset(wait)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case data := <-inputCh:
			if !timer.Stop() {
				<-timer.C
			}
			if err := h.WriteSerialInput(ctx, data); err != nil {
				return err
			}
		case <-timer.C:
		}
	}
}
