//go:build linux

package boot

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	unixfs_fuse "github.com/s4wave/spacewave/db/unixfs/fuse"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Run opens a retained image, or builds the in-memory baseline when store is
// empty, then mounts the image and runs a bounded chroot workload. It unmounts
// and joins the server before releasing its pinned storage.
// It requires root in a private mount namespace, or an isolated guest.
// The filesystem server must never enter a blocking exec or mmap through its
// own served tree: its runtime and every bootstrap exec use independent pages.
func Run(ctx context.Context, workload, store string) (retErr error) {
	// Only the experiment's independent workload modes may enter the root.
	if workload != "workload" && workload != "workload-fail" && workload != "workload-wait" {
		return errors.New("unknown workload mode")
	}

	// Keep every bootstrap path outside the mounted tree.
	base, err := os.MkdirTemp("", "spacewave-os-")
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, os.RemoveAll(base)) }()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	file, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer file.Close()

	// Supply bazil's helper from this same static executable via a private PATH.
	if err := os.Symlink(executable, filepath.Join(base, "fusermount3")); err != nil {
		return err
	}
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", base); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, os.Setenv("PATH", oldPath)) }()

	// Open never regenerates retained content; the empty store keeps the baseline.
	le := logrus.NewEntry(logrus.New())
	var root *Root
	if store == "" {
		root, err = NewRoot(ctx, le, file)
	} else {
		root, err = OpenRoot(ctx, le, store)
	}
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()

	// Read all required blocks before reporting ready, including on reopen.
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := root.VerifyImage(ctx, file); err != nil {
		return err
	}
	println("SPACEWAVE_OS_FROZEN")

	// Start the existing kernel adapter only after the image is complete.
	mountPath := filepath.Join(base, "root")
	if err := os.Mkdir(mountPath, 0o755); err != nil {
		return err
	}
	rfs, err := unixfs_fuse.Mount(context.WithoutCancel(ctx), le, mountPath, root.Handle(), false, true, nil)
	if err != nil {
		return err
	}

	// Contain workload on cancellation or server exit while keeping storage open.
	workCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	served := make(chan error, 1)
	println("SPACEWAVE_OS_MOUNTED")
	go func() {
		// Disconnect on an unexpected server exit so kernel users cannot wait
		// indefinitely on a dead server's still-open FUSE channel. Cancel the
		// workload too; storage remains alive until ordered Root cleanup.
		err := rfs.Serve()
		rfs.Close()
		cancel()
		served <- err
	}()

	// Always unmount after the child has exited, then join the server. A failed
	// ordinary unmount is reported even if detached cleanup succeeds.
	defer func() {
		// Stop kernel users before releasing the snapshot, including on failure.
		unmountErr := unix.Unmount(mountPath, 0)
		if unmountErr != nil {
			retErr = errors.Join(retErr, unmountErr, unix.Unmount(mountPath, unix.MNT_DETACH))
		}

		// Abort outstanding requests if ordinary unmount could not drain them.
		if unmountErr != nil {
			rfs.Close()
		}
		if unmountErr == nil {
			println("SPACEWAVE_OS_UNMOUNT_OK")
		}
		retErr = errors.Join(retErr, <-served)
		println("SPACEWAVE_OS_SERVER_JOINED")
	}()

	// First exec bootstrap pages in a separate address space. Direct vfork exec
	// from our own FUSE mount can retain the sole Go processor while the kernel
	// waits for that processor's server. The bootstrap child enters UnixFS only
	// after the parent's vfork has finished. Chroot is rooting, not a sandbox.
	// Launch with inherited serial descriptors but without bootstrap paths.
	cmd := exec.CommandContext(workCtx, executable, "enter", mountPath, workload)
	cmd.Dir = "/"
	cmd.Env = []string{"GOMAXPROCS=1"}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

	// Join the child before deferred unmount and server drain.
	err = cmd.Run()
	println("SPACEWAVE_OS_CHILD_JOINED")
	if err != nil || workCtx.Err() != nil {
		return errors.Join(err, workCtx.Err())
	}
	println("SPACEWAVE_OS_EXEC_OK")
	return nil
}
