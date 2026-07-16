//go:build darwin && !js

package bldr_buildbudget

import "golang.org/x/sys/unix"

func availableHostMemoryBytes() (uint64, error) {
	// Darwin does not expose a stable available-memory syscall. Physical host
	// capacity is the safe upper bound for deriving the process budget.
	return unix.SysctlUint64("hw.memsize")
}
