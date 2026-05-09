//go:build !js

package termui

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// Run renders a live terminal screen until the context or update channel stops it.
func Run[T any](
	ctx context.Context,
	_ *os.File,
	output io.Writer,
	initial T,
	updates <-chan T,
	render func(T) string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	text = normalizeNewlines(text)
	if _, err := io.WriteString(output, text); err != nil {
		return errors.Wrap(err, "write terminal screen")
	}
	return nil
}

func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\n", "\r\n")
}
