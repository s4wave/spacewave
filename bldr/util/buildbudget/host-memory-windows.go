//go:build windows && !js

package bldr_buildbudget

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

type memoryStatusEx struct {
	length                  uint32
	memoryLoad              uint32
	totalPhysicalMemory     uint64
	availablePhysicalMemory uint64
	totalPageFile           uint64
	availablePageFile       uint64
	totalVirtualMemory      uint64
	availableVirtualMemory  uint64
	availableExtended       uint64
}

func availableHostMemoryBytes() (uint64, error) {
	status := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 {
		if err != nil {
			return 0, err
		}
		return 0, errors.New("GlobalMemoryStatusEx returned failure")
	}
	return status.availablePhysicalMemory, nil
}
