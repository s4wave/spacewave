package s4db

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// getCompressedFileSize reads the allocated size of a sparse file.
var getCompressedFileSize = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCompressedFileSizeW")

// usage returns the allocated bytes and length of the file at path.
func usage(t testing.TB, path string) (used, size int64) {
	// Read the length.
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Read the allocated size, which excludes punched ranges.
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var high uint32
	low, _, err := getCompressedFileSize.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&high)))
	if uint32(low) == 0xffffffff && err != windows.ERROR_SUCCESS {
		t.Fatal(err)
	}
	return int64(high)<<32 | int64(uint32(low)), info.Size()
}
