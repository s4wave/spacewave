//go:build linux && !js

package bldr_buildbudget

import (
	"bufio"
	"bytes"
	"os"
	"strconv"

	"github.com/pkg/errors"
)

// availableHostMemoryBytes returns the kernel's MemAvailable estimate: free
// memory plus the page cache and slab it can reclaim without swapping. Free
// memory alone excludes the page cache and collapses the budget to its minimum
// on any host that has been reading files.
func availableHostMemoryBytes() (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := bytes.Fields(scanner.Bytes())
		if len(fields) != 3 || string(fields[0]) != "MemAvailable:" {
			continue
		}
		kib, err := strconv.ParseUint(string(fields[1]), 10, 64)
		if err != nil {
			return 0, errors.Wrap(err, "parse MemAvailable")
		}
		return kib << 10, nil
	}
	return 0, errors.New("MemAvailable not found in /proc/meminfo")
}
