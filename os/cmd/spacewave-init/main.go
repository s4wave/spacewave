//go:build linux

// Command spacewave-init boots the immutable UnixFS experiment or runs its probe.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/s4wave/spacewave/os/boot"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// main selects bootstrap, adapter helper or separately executed workload mode.
func main() {
	// Dispatch the helper before changing namespaces or bootstrap mounts.
	if filepath.Base(os.Args[0]) == "fusermount3" {
		finish(boot.RunMountHelper(os.Args[1:]))
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "enter" {
		finish(boot.EnterRoot(os.Args[2], os.Args[3]))
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "workload" || os.Args[1] == "workload-fail" || os.Args[1] == "workload-wait") {
		finish(boot.RunWorkload(os.Args[1]))
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "seed" {
		finish(seedImage(os.Args[2]))
		return
	}

	// A real PID1 prepares only the independent bootstrap's kernel interfaces.
	pid1 := os.Getpid() == 1
	store := os.Getenv("SPACEWAVE_OS_STORE")
	if pid1 {
		finish(prepareBootstrap(store != ""))
		println("SPACEWAVE_OS_PID1 gomaxprocs=" + strconv.Itoa(runtime.GOMAXPROCS(0)))
	} else {
		// Mount namespaces are per-thread. Keep this goroutine on its private
		// namespace through helper exec, workload wait and all deferred unmounts.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		finish(unix.Unshare(unix.CLONE_NEWNS))
		finish(unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""))
		if len(os.Args) == 3 {
			store = os.Args[2]
		}
	}

	// Join cancellation and run the bounded supervisor, retaining serial output
	// on failure. The initial experiment powers off instead of restarting work.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Select the bounded workload behavior, then run its ordinary supervisor.
	workload := "workload"
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "probe-fail":
			workload = "workload-fail"
		case "probe-wait":
			workload = "workload-wait"
		}
	}
	err := boot.Run(ctx, workload, store)
	if pid1 && store != "" {
		// Run has joined the workload/server and closed storage before this.
		unmountErr := unix.Unmount("/store", 0)
		err = errors.Join(err, unmountErr)
		if unmountErr == nil {
			println("SPACEWAVE_OS_BACKING_UNMOUNT_OK")
		}
	}
	if err != nil {
		println("SPACEWAVE_OS_FAILED: " + err.Error())
	} else {
		println("SPACEWAVE_OS_PASS")
	}
	if pid1 {
		unix.Sync()
		println("SPACEWAVE_OS_POWEROFF")
		finish(unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF))
		select {}
	}
	finish(err)
}

// prepareBootstrap mounts independent kernel interfaces and optional backing data.
func prepareBootstrap(persistent bool) error {
	// Create kernel mountpoints on the independent initramfs.
	for _, dir := range []string{"/proc", "/dev", "/tmp"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return err
	}
	if err := unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""); err != nil {
		return err
	}

	// Load only kernel content packaged for this matching kernel.
	return boot.PrepareBackingStore(persistent)
}

// seedImage imports this bootstrap ELF once and verifies its committed reopen.
func seedImage(dir string) (retErr error) {
	// Keep the seed input outside the image it will later execute.
	path, err := os.Executable()
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	le := logrus.NewEntry(logrus.New())
	if err := boot.SeedRoot(context.Background(), le, dir, file); err != nil {
		return err
	}

	// Reopen rather than verify the seed's in-memory snapshot.
	root, err := boot.OpenRoot(context.Background(), le, dir)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	return root.VerifyImage(context.Background(), file)
}

// finish reports a command failure and prevents a failed bootstrap continuing.
func finish(err error) {
	if err != nil {
		println("SPACEWAVE_OS_ERROR: " + err.Error())
		if os.Getpid() == 1 {
			// Preserve the stopped bootstrap and serial evidence for recovery.
			select {}
		}
		os.Exit(1)
	}
}
