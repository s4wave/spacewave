//go:build windows && !js

package hostmem

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// globalMemoryStatusEx is the kernel32 call that fills a memoryStatusEx.
var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryStatusEx is the MEMORYSTATUSEX structure.
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

// Total returns the physical memory of the host in bytes.
func Total() (uint64, error) {
	status, err := memoryStatus()
	return status.totalPhysicalMemory, err
}

// Available returns the physical memory not in use, in bytes.
func Available() (uint64, error) {
	status, err := memoryStatus()
	return status.availablePhysicalMemory, err
}

// memoryStatus reads the host's memory status.
func memoryStatus() (memoryStatusEx, error) {
	// Fill the status, sized for this structure version.
	status := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if result != 0 {
		return status, nil
	}

	// A zero result indicates failure. Report the call's error when the call
	// sets one.
	if err != nil {
		return status, err
	}
	return status, errors.New("GlobalMemoryStatusEx returned failure")
}
