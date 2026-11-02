//go:build linux

// Command spacewave-init boots the immutable UnixFS experiment or runs its probe.
package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/s4wave/spacewave/os/boot"
	"golang.org/x/sys/unix"
)

// main selects bootstrap, adapter helper or separately executed workload mode.
func main() {
	// Dispatch the helper before changing namespaces or bootstrap mounts.
	if filepath.Base(os.Args[0]) == "fusermount3" {
		finish(boot.RunMountHelper(os.Args[1:]))
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "workload" || os.Args[1] == "workload-fail") {
		finish(boot.RunWorkload(os.Args[1] == "workload-fail"))
		return
	}

	// A real PID1 prepares only the independent bootstrap's kernel interfaces.
	pid1 := os.Getpid() == 1
	if pid1 {
		finish(prepareBootstrap())
		println("SPACEWAVE_OS_PID1")
	} else {
		finish(unix.Unshare(unix.CLONE_NEWNS))
		finish(unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""))
	}

	// Join cancellation and run the bounded supervisor, retaining serial output
	// on failure. The initial experiment powers off instead of restarting work.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	fail := len(os.Args) == 2 && os.Args[1] == "probe-fail"
	err := boot.Run(ctx, fail)
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

// prepareBootstrap mounts kernel interfaces and loads an optional FUSE module.
func prepareBootstrap() error {
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

	// A built-in FUSE driver needs nothing else; modular kernels carry the
	// exact matching uncompressed module beside init, never inside UnixFS.
	if _, err := os.Stat("/dev/fuse"); err == nil {
		return nil
	}
	module, err := os.Open("/fuse.ko")
	if err != nil {
		return err
	}
	defer module.Close()
	return unix.FinitModule(int(module.Fd()), "", 0)
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
