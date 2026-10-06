//go:build darwin || linux

// Command mproc checks the cross-process primitives a shared single-file
// engine needs: a byte-range lock per reader slot that the kernel releases
// when the process dies, and a file watch that wakes readers in other
// processes after the writer appends a commit.
//
// The parent is the writer. It starts reader children, appends commit records
// carrying their write time, and the children report how long each took to
// observe the record.
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// headerSize is the space before the first record. Byte 0 holds the commit
// sequence; bytes [slotBase, slotBase+slots) are reader slot locks.
const headerSize = 4096

// slotBase is the first reader slot byte.
const slotBase = 1024

// recordSize is the length of one commit record.
const recordSize = 4096

// main runs the writer, which starts the reader processes, or one reader.
func main() {
	// Parse the role and the run's size.
	role := flag.String("role", "writer", "writer or reader")
	path := flag.String("path", "/tmp/mproc.s4wave", "shared file")
	readers := flag.Int("readers", 3, "reader processes")
	commits := flag.Int("commits", 2000, "commit records")
	slot := flag.Int("slot", 0, "reader slot")
	flag.Parse()

	// Run the requested role.
	if *role == "reader" {
		read(*path, *slot, *commits)
		return
	}
	write(*path, *readers, *commits)
}

// write creates the file, starts readers, and appends commits.
func write(path string, readers, commits int) {
	// Create the file and take the writer lock on byte 0.
	_ = os.Remove(path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := f.Truncate(headerSize); err != nil {
		panic(err)
	}
	if err := lock(f, 0, true); err != nil {
		panic(err)
	}

	// Start the readers and wait until each reports it holds its slot,
	// then forward the rest of its output.
	var procs []*exec.Cmd
	var output sync.WaitGroup
	for i := range readers {
		cmd := exec.Command(os.Args[0], "-role", "reader", "-path", path, "-slot", strconv.Itoa(i), "-commits", strconv.Itoa(commits))
		cmd.Stderr = os.Stderr
		out, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		rd := bufio.NewReader(out)
		if _, err := rd.ReadString('\n'); err != nil {
			panic(err)
		}
		output.Go(func() { _, _ = io.Copy(os.Stdout, rd) })
		procs = append(procs, cmd)
	}
	fmt.Printf("writer: %d reader slots held\n", readers)

	// Append each commit record, then publish its sequence in the header.
	rec := make([]byte, recordSize)
	seq := make([]byte, 8)
	for i := 1; i <= commits; i++ {
		binary.LittleEndian.PutUint64(rec, uint64(time.Now().UnixNano()))
		if _, err := f.WriteAt(rec, int64(headerSize+(i-1)*recordSize)); err != nil {
			panic(err)
		}
		binary.LittleEndian.PutUint64(seq, uint64(i))
		if _, err := f.WriteAt(seq, 0); err != nil {
			panic(err)
		}
		time.Sleep(500 * time.Microsecond)
	}

	// Kill the first reader and confirm the kernel released its slot.
	if err := procs[0].Process.Kill(); err != nil {
		panic(err)
	}
	output.Wait()
	for _, p := range procs {
		_ = p.Wait()
	}
	fmt.Printf("writer: slot 0 held after kill: %v\n", held(f, slotBase))
}

// read holds a slot, watches the file, and reports observation latency.
func read(path string, slot, commits int) {
	// Take the slot, start watching, and tell the writer.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		panic(err)
	}
	if err := lock(f, int64(slotBase+slot), false); err != nil {
		panic(err)
	}
	w, err := newWatch(path)
	if err != nil {
		panic(err)
	}
	fmt.Println("ready")

	// Each wakeup reads the published sequence and the newest record time.
	var lat []time.Duration
	var last uint64
	buf := make([]byte, 8)
	wakes := 0
	for last < uint64(commits) {
		if err := w.wait(); err != nil {
			panic(err)
		}
		wakes++
		if _, err := f.ReadAt(buf, 0); err != nil {
			panic(err)
		}
		seq := binary.LittleEndian.Uint64(buf)
		if seq == last {
			continue
		}
		if _, err := f.ReadAt(buf, int64(headerSize+(int(seq)-1)*recordSize)); err != nil {
			panic(err)
		}
		lat = append(lat, time.Since(time.Unix(0, int64(binary.LittleEndian.Uint64(buf)))))
		last = seq
	}

	// Report the latency quantiles.
	slices.Sort(lat)
	p := func(q float64) time.Duration { return lat[int(q*float64(len(lat)-1))].Round(time.Microsecond) }
	fmt.Printf("reader %d: %d commits seen in %d observations, %d wakeups; latency p50 %s p99 %s max %s\n",
		slot, commits, len(lat), wakes, p(0.5), p(0.99), p(1))
	if slot == 0 {
		// Stay alive holding the slot until the writer kills this process.
		os.Stdout.Close()
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, unix.SIGTERM)
		<-stop
	}
}

// lock takes a write lock on one byte, or a read lock when shared.
func lock(f *os.File, off int64, exclusive bool) error {
	typ := int16(unix.F_RDLCK)
	if exclusive {
		typ = unix.F_WRLCK
	}
	lk := unix.Flock_t{Type: typ, Whence: 0, Start: off, Len: 1}
	return unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lk)
}

// held reports whether another process holds a lock on byte off.
func held(f *os.File, off int64) bool {
	lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: off, Len: 1}
	if err := unix.FcntlFlock(f.Fd(), unix.F_GETLK, &lk); err != nil {
		panic(err)
	}
	return lk.Type != unix.F_UNLCK
}
