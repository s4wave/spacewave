//go:build !js

package termui

import (
	"context"
	"io"
	"os"

	"github.com/pkg/errors"
	"golang.org/x/term"
)

// Run renders a live terminal screen until the context, quit key, or update channel stops it.
func Run[T any](
	ctx context.Context,
	input *os.File,
	output io.Writer,
	initial T,
	updates <-chan T,
	render func(T) string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	quitCh := make(chan struct{}, 1)
	restore, err := prepareInput(input, quitCh)
	if err != nil {
		return err
	}
	defer restore()
	if _, err := io.WriteString(output, "\x1b[?25l"); err != nil {
		return errors.Wrap(err, "hide terminal cursor")
	}
	defer func() {
		_, _ = io.WriteString(output, "\x1b[?25h")
	}()
	if err := Write(output, render(initial)); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-quitCh:
			return nil
		case snapshot, ok := <-updates:
			if !ok {
				return nil
			}
			if err := Write(output, render(snapshot)); err != nil {
				return err
			}
		}
	}
}

// Write replaces the current terminal screen with text.
func Write(output io.Writer, text string) error {
	if _, err := io.WriteString(output, "\x1b[H\x1b[2J"); err != nil {
		return errors.Wrap(err, "clear terminal screen")
	}
	if _, err := io.WriteString(output, text); err != nil {
		return errors.Wrap(err, "write terminal screen")
	}
	return nil
}

func prepareInput(input *os.File, quitCh chan<- struct{}) (func(), error) {
	if input == nil || !term.IsTerminal(int(input.Fd())) {
		return func() {}, nil
	}
	prev, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return nil, errors.Wrap(err, "enable raw terminal input")
	}
	go readQuitKeys(input, quitCh)
	return func() {
		_ = term.Restore(int(input.Fd()), prev)
	}, nil
}

func readQuitKeys(input *os.File, quitCh chan<- struct{}) {
	var buf [16]byte
	for {
		n, err := input.Read(buf[:])
		if err != nil {
			return
		}
		for _, b := range buf[:n] {
			if b == 'q' || b == 3 || b == 27 {
				select {
				case quitCh <- struct{}{}:
				default:
				}
				return
			}
		}
	}
}
