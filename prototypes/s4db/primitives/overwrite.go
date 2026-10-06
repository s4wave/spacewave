//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"time"
)

// overwrite measures 4 KiB positional writes into an already allocated file
// against writes that extend it, to separate allocation cost from syscall cost.
func overwrite(path string) {
	// Create the scratch file.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Extend the file with 4 KiB writes, then flush the allocation.
	buf := make([]byte, 4096)
	total := 64 << 20
	start := time.Now()
	for off := 0; off < total; off += len(buf) {
		if _, err := f.WriteAt(buf, int64(off)); err != nil {
			panic(err)
		}
	}
	ext := time.Since(start)
	if err := f.Sync(); err != nil {
		panic(err)
	}

	// Overwrite the same allocated range with 4 KiB writes.
	start = time.Now()
	for off := 0; off < total; off += len(buf) {
		if _, err := f.WriteAt(buf, int64(off)); err != nil {
			panic(err)
		}
	}
	over := time.Since(start)
	mib := float64(total) / (1 << 20)
	fmt.Printf("%-40s %8.0f MiB/s\n", "pwrite 4 KiB extending", mib/ext.Seconds())
	fmt.Printf("%-40s %8.0f MiB/s\n", "pwrite 4 KiB overwriting", mib/over.Seconds())
}
