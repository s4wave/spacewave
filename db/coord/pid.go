//go:build !js

package coord

import (
	"os"
	"strconv"
)

func currentPID() uint32 {
	pid, err := strconv.ParseUint(strconv.Itoa(os.Getpid()), 10, 32)
	if err != nil {
		panic("coord: pid out of range")
	}
	return uint32(pid) //nolint:gosec
}
