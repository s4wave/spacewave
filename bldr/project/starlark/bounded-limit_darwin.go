//go:build darwin

package bldr_project_starlark

import (
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// limitSearchTop is a limit above the address space of any process, which the
// search for the current size starts from.
const limitSearchTop = 1 << 50

// limitSearchStep is the precision of the search for the current size.
const limitSearchStep = 1 << 20

// limitMemory limits the address space of this process to memoryBytes more than
// it uses now. Darwin refuses a limit below the current size, which is hundreds
// of gigabytes for a Go process and cannot be read without cgo, so the search
// for the smallest limit it accepts finds the size.
func limitMemory(memoryBytes uint64) error {
	// Search for the smallest accepted limit, putting the unlimited state back
	// after each probe so the process never runs near its own size.
	high := uint64(limitSearchTop)
	if !tryLimit(high) {
		return errors.New("the system refuses an address space limit")
	}
	for low := uint64(0); high-low > limitSearchStep; {
		mid := low + (high-low)/2
		if tryLimit(mid) {
			high = mid
		} else {
			low = mid
		}
	}

	// Set the limit above the current size.
	limit := high + memoryBytes
	return unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: limit, Max: limit})
}

// tryLimit reports whether the system accepts limit as the address space
// limit, and leaves the process without a limit.
func tryLimit(limit uint64) bool {
	unlimited := &unix.Rlimit{Cur: unix.RLIM_INFINITY, Max: unix.RLIM_INFINITY}
	if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: limit, Max: unix.RLIM_INFINITY}); err != nil {
		return false
	}
	return unix.Setrlimit(unix.RLIMIT_AS, unlimited) == nil
}
