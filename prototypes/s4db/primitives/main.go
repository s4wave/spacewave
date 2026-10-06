//go:build darwin || linux

// Command primitives measures the file primitives a single-file storage engine
// builds on: flush variants, positional write and read throughput, hole
// punching, and byte-range locks. Run it with -dir on the file system under
// test; it writes about 300 MiB.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sys/unix"
)

// main measures the file system primitives the engine design relies on.
func main() {
	// Parse the directory under test.
	dir := flag.String("dir", os.TempDir(), "directory on the file system under test")
	flag.Parse()

	// Measure only extending against overwriting writes when asked.
	if os.Getenv("ONLY_OVERWRITE") != "" {
		overwrite(filepath.Join(*dir, "overwrite.s4wave"))
		return
	}

	// Open a fresh scratch file for every measurement.
	path := filepath.Join(*dir, "primitives.s4wave")
	defer os.Remove(path)
	open := func() *os.File {
		_ = os.Remove(path)
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			panic(err)
		}
		return f
	}

	// Measure each flush variant after a small appended record.
	for _, flush := range flushes() {
		f := open()
		lat := measure(200, func(i int) {
			buf := make([]byte, 4096)
			if _, err := f.WriteAt(buf, int64(i)*4096); err != nil {
				panic(err)
			}
			if err := flush.fn(f); err != nil {
				panic(err)
			}
		})
		report("append 4 KiB + "+flush.name, lat)
		f.Close()
	}

	// Measure flush cost as the unflushed batch grows.
	for _, size := range []int{64 << 10, 1 << 20, 8 << 20} {
		for _, flush := range flushes() {
			f := open()
			buf := make([]byte, size)
			lat := measure(20, func(i int) {
				if _, err := f.WriteAt(buf, int64(i)*int64(size)); err != nil {
					panic(err)
				}
				if err := flush.fn(f); err != nil {
					panic(err)
				}
			})
			report(fmt.Sprintf("append %d KiB + %s", size>>10, flush.name), lat)
			f.Close()
		}
	}

	// Measure buffered sequential write throughput without flushes.
	f := open()
	for _, size := range []int{4 << 10, 64 << 10, 1 << 20} {
		buf := make([]byte, size)
		total := 256 << 20
		start := time.Now()
		for off := 0; off < total; off += size {
			if _, err := f.WriteAt(buf, int64(off)); err != nil {
				panic(err)
			}
		}
		el := time.Since(start)
		fmt.Printf("%-40s %8.0f MiB/s\n", fmt.Sprintf("pwrite %d KiB unflushed", size>>10), float64(total)/(1<<20)/el.Seconds())
	}
	if err := f.Sync(); err != nil {
		panic(err)
	}

	// Measure hot positional reads at random 4 KiB offsets.
	for _, size := range []int{4 << 10, 64 << 10} {
		buf := make([]byte, size)
		lat := measure(20000, func(i int) {
			off := int64((i*7919)%((256<<20)/size)) * int64(size)
			if _, err := f.ReadAt(buf, off); err != nil {
				panic(err)
			}
		})
		report(fmt.Sprintf("pread %d KiB hot", size>>10), lat)
	}

	// Measure hole punching and confirm the allocation shrinks.
	before := allocated(f)
	lat := measure(1024, func(i int) {
		if err := punch(f, int64(i)*(128<<10), 64<<10); err != nil {
			panic(err)
		}
	})
	report("punch 64 KiB", lat)
	fmt.Printf("%-40s %d MiB -> %d MiB\n", "allocation after 64 MiB punched", before>>20, allocated(f)>>20)
	f.Close()

	// Measure byte-range lock acquire and release.
	f = open()
	lat = measure(20000, func(i int) {
		lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: int64(i % 64), Len: 1}
		if err := unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lk); err != nil {
			panic(err)
		}
		lk.Type = unix.F_UNLCK
		if err := unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lk); err != nil {
			panic(err)
		}
	})
	report("fcntl lock+unlock 1 byte", lat)
	lat = measure(20000, func(i int) {
		lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: int64(i % 64), Len: 1}
		if err := unix.FcntlFlock(f.Fd(), unix.F_GETLK, &lk); err != nil {
			panic(err)
		}
	})
	report("fcntl getlk 1 byte", lat)
	f.Close()
}

// flush is one way to make earlier writes durable or ordered.
type flush struct {
	// name labels the flush in the report.
	name string
	// fn flushes the file.
	fn func(f *os.File) error
}

// measure runs fn n times and returns each latency.
func measure(n int, fn func(i int)) []time.Duration {
	lat := make([]time.Duration, n)
	for i := range n {
		start := time.Now()
		fn(i)
		lat[i] = time.Since(start)
	}
	return lat
}

// report prints the median and 99th percentile latency.
func report(name string, lat []time.Duration) {
	slices.Sort(lat)
	p := func(q float64) time.Duration { return lat[int(q*float64(len(lat)-1))] }
	fmt.Printf("%-40s p50 %10s  p99 %10s\n", name, p(0.5).Round(time.Microsecond/10), p(0.99).Round(time.Microsecond/10))
}

// allocated returns the bytes the file system allocated for f.
func allocated(f *os.File) int64 {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		panic(err)
	}
	return st.Blocks * 512
}
