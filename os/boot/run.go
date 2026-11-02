//go:build linux

package boot

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	unixfs_fuse "github.com/s4wave/spacewave/db/unixfs/fuse"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Run builds, freezes and mounts the image, runs a bounded chroot workload,
// then unmounts and joins the server before releasing its pinned storage.
// It requires root in a private mount namespace, or an isolated guest.
func Run(ctx context.Context, fail bool) (retErr error) {
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

	// Import and freeze the ordinary World before exposing it to the kernel.
	le := logrus.NewEntry(logrus.New())
	root, err := NewRoot(ctx, le, file)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	println("SPACEWAVE_OS_FROZEN")

	// Start the existing kernel adapter only after the image is complete.
	mountPath := filepath.Join(base, "root")
	if err := os.Mkdir(mountPath, 0o755); err != nil {
		return err
	}
	rfs, err := unixfs_fuse.Mount(ctx, le, mountPath, root.Handle(), false, true, nil)
	if err != nil {
		return err
	}
	served := make(chan error, 1)
	go func() {
		// Disconnect on an unexpected server exit so kernel users cannot wait
		// indefinitely on a dead server's still-open FUSE channel.
		err := rfs.Serve()
		rfs.Close()
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
		retErr = errors.Join(retErr, <-served)
		if unmountErr == nil {
			println("SPACEWAVE_OS_UNMOUNT_OK")
		}
	}()

	// Exec from the FUSE tree in another process; this runtime never faults on
	// pages that need its own FUSE goroutines. Chroot is rooting, not a sandbox.
	workCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	arg := "workload"
	if fail {
		arg = "workload-fail"
	}

	// Launch with inherited serial descriptors but without bootstrap paths.
	cmd := exec.CommandContext(workCtx, "/workload", arg)
	cmd.Dir = "/"
	cmd.Env = []string{"GOMAXPROCS=4"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: mountPath}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	println("SPACEWAVE_OS_EXEC_OK")
	return nil
}
