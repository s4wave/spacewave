//go:build linux && !js

package bldr_buildbudget

import "golang.org/x/sys/unix"

func availableHostMemoryBytes() (uint64, error) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0, err
	}
	return (info.Freeram + info.Bufferram) * uint64(info.Unit), nil
}
