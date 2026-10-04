//go:build !js

package termui

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/term"
)

// KeyHandler handles a raw key byte from the terminal.
type KeyHandler[T any] func(T, byte)

// Run renders a live terminal screen until the context or update channel stops it.
func Run[T any](
	ctx context.Context,
	input *os.File,
	output io.Writer,
	initial T,
	updates <-chan T,
	render func(T) string,
) error {
	return RunWithKeys(ctx, input, output, initial, updates, render, nil)
}

// RunWithKeys renders a live terminal screen and forwards keypresses.
func RunWithKeys[T any](
	ctx context.Context,
	input *os.File,
	output io.Writer,
	initial T,
	updates <-chan T,
	render func(T) string,
	keyHandler KeyHandler[T],
) error {
	// Reject canceled runs and hide the cursor until deferred restoration.
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := io.WriteString(output, "\x1b[?25l"); err != nil {
		return errors.Wrap(err, "hide terminal cursor")
	}
	defer func() {
		_, _ = io.WriteString(output, "\x1b[?25h")
	}()

	// Render the initial snapshot before listening for keys or updates.
	if err := Write(output, render(initial)); err != nil {
		return err
	}

	// Track the displayed snapshot and start the optional raw-key reader.
	current := initial
	keyCh, restore, err := startKeyReader(input, keyHandler != nil)
	if err != nil {
		return err
	}
	defer restore()
	for {
		select {
		case <-ctx.Done():
			return nil
		case key, ok := <-keyCh:
			if !ok {
				keyCh = nil
				continue
			}
			if keyHandler != nil {
				keyHandler(current, key)
			}
		case snapshot, ok := <-updates:
			if !ok {
				return nil
			}
			current = snapshot
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
	text = normalizeNewlines(text)
	if _, err := io.WriteString(output, text); err != nil {
		return errors.Wrap(err, "write terminal screen")
	}
	return nil
}

func startKeyReader(input *os.File, enabled bool) (<-chan byte, func(), error) {
	// Create the key channel and return it closed when input is disabled.
	keyCh := make(chan byte, 16)
	if !enabled || input == nil {
		close(keyCh)
		return keyCh, func() {}, nil
	}

	// Configure raw terminal mode and retain its restoration callback.
	restore := func() {}
	if term.IsTerminal(int(input.Fd())) {
		oldState, err := term.MakeRaw(int(input.Fd()))
		if err != nil {
			return nil, nil, errors.Wrap(err, "set terminal raw mode")
		}
		restore = func() {
			_ = term.Restore(int(input.Fd()), oldState)
		}
	}

	// Read terminal bytes asynchronously and publish them to the key channel.
	go func() {
		defer close(keyCh)
		var buf [1]byte
		for {
			n, err := input.Read(buf[:])
			if err != nil {
				return
			}
			if n == 0 {
				continue
			}
			select {
			case keyCh <- buf[0]:
			default:
			}
		}
	}()
	return keyCh, restore, nil
}

func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n")
}
