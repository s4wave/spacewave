//go:build linux

package bldr_project_starlark

import (
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// limitMemory limits the address space of this process to memoryBytes more than
// it uses now. The Go runtime has already reserved some, so the limit is
// relative to the current size.
func limitMemory(memoryBytes uint64) error {
	// Read the size of the address space in pages from the first field.
	statm, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return err
	}
	size, _, _ := strings.Cut(string(statm), " ")
	pages, err := strconv.ParseUint(size, 10, 64)
	if err != nil {
		return errors.Wrap(err, "parse /proc/self/statm")
	}

	// Set the limit above the current size.
	limit := pages*uint64(os.Getpagesize()) + memoryBytes
	return unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: limit, Max: limit})
}
