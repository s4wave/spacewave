//go:build linux

package boot

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// newPayload provides distinct pages so faults must fetch more than one block.
func newPayload() *bytes.Reader {
	// Mix page and byte offsets across a bounded deterministic input.
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i*31 + i/4096)
	}
	return bytes.NewReader(data)
}

// RunWorkload executes in a separate process chrooted to the FUSE root. Lazy
// page faults are safe here because this runtime does not serve the filesystem.
func RunWorkload(fail bool) error {
	// Require a genuine root-relative executable and the image's only data file.
	if _, err := os.Stat("/workload"); err != nil {
		return err
	}
	file, err := os.Open("/payload")
	if err != nil {
		return err
	}
	defer file.Close()

	// Hash lazy shared mappings whose faults reach the independent server.
	mapped, err := unix.Mmap(int(file.Fd()), 0, 3<<20, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return err
	}
	defer unix.Munmap(mapped)
	want := make([]byte, 3<<20)
	if _, err := newPayload().Read(want); err != nil {
		return err
	}
	if sha256.Sum256(mapped) != sha256.Sum256(want) {
		return errors.New("mmap hash mismatch")
	}
	println("SPACEWAVE_OS_MMAP_OK bytes=" + strconv.Itoa(len(mapped)))

	// Require immutable mount failures through independent kernel operations.
	for _, err := range []error{
		os.WriteFile("/payload", []byte("changed"), 0o644),
		os.WriteFile("/new", []byte("new"), 0o644),
		os.Mkdir("/dir", 0o755),
		os.Remove("/payload"),
		os.Rename("/payload", "/renamed"),
	} {
		if !errors.Is(err, unix.EROFS) {
			return errors.Join(errors.New("expected EROFS"), err)
		}
	}
	println("SPACEWAVE_OS_READONLY_OK operations=5")

	// Exercise the supervisor's workload-failure cleanup path on request.
	if fail {
		return errors.New("injected workload failure")
	}
	println("SPACEWAVE_OS_WORKLOAD_OK pid=" + strconv.Itoa(os.Getpid()))
	return nil
}
