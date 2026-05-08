//go:build !js

package devtool

import (
	"os"

	"github.com/mattn/go-isatty"
)

func devtoolFileDescriptorIsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
