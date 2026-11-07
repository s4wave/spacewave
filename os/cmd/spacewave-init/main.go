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
	if len(os.Args) == 3 && os.Args[1] == "enter-console" {
		finish(boot.EnterConsole(os.Args[2]))
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "workload" || os.Args[1] == "workload-fail" || os.Args[1] == "workload-wait") {
		finish(boot.RunWorkload(os.Args[1]))
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "seed" {
		if len(os.Args) != 4 {
			finish(errors.New("usage: spacewave-init seed ABSENT-DIRECTORY STATIC-BUSYBOX"))
			return
		}
		finish(seedImage(os.Args[2], os.Args[3]))
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "seed-data" {
		if len(os.Args) != 3 {
			finish(errors.New("usage: spacewave-init seed-data ABSENT-DIRECTORY"))
			return
		}
		finish(seedData(os.Args[2]))
		return
	}

	// A real PID1 prepares only the independent bootstrap's kernel interfaces.
	pid1 := os.Getpid() == 1
	console := os.Getenv("SPACEWAVE_OS_MODE") == "console" || (len(os.Args) > 1 && os.Args[1] == "console")
	if console && !pid1 {
		finish(errors.New("console requires actual guest PID1; use probe for hosted checks"))
		return
	}
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

	// BusyBox poweroff sends SIGUSR2 to init. Console shutdown is a request,
	// separate from cancellation that disconnects failed filesystem service.
	signals := []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	if console {
		signals = append(signals, syscall.SIGUSR2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), signals...)
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

	// Interactive mode is guest-only; hosted probes retain their bounded lifecycle.
	var err error
	if console {
		terminal := os.Getenv("SPACEWAVE_OS_TTY")
		if terminal == "" {
			terminal = "/dev/ttyS0"
		}
		err = boot.RunConsole(context.Background(), ctx.Done(), store, os.Getenv("SPACEWAVE_OS_DATA"), terminal,
			os.Getenv("SPACEWAVE_OS_NATIVE_BENCH") == "1")
	} else {
		err = boot.Run(ctx, workload, store)
	}
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
		if console {
			finish(err)
		}
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
	// Keep all guest mounts private, including kernel mounts nested under UnixFS.
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}

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

// seedData initializes only an absent disposable data fixture through CreateData.
func seedData(dir string) error {
	// Complete the normal Data close fence before reporting fixture creation.
	d, err := boot.CreateData(context.Background(), logrus.NewEntry(logrus.New()), dir)
	if err != nil {
		return err
	}
	if err := d.Close(); err != nil {
		return err
	}
	println("SPACEWAVE_OS_DATA_SEEDED")
	return nil
}

// seedImage imports the bootstrap probe and static tools, then verifies a reopen.
func seedImage(dir, busyboxPath string) (retErr error) {
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

	// Stream the caller's independent tool file without executing or collecting it.
	busybox, err := os.Open(busyboxPath) //nolint:gosec // The operator selects the tool input, outside any served filesystem.
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, busybox.Close()) }()
	le := logrus.NewEntry(logrus.New())
	if err := boot.SeedRoot(context.Background(), le, dir, file, busybox); err != nil {
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
			println("Recovery: stores remain in place; this bootstrap will not reset or reseed them.")
			println("Exit QEMU with Ctrl-a x. Check the error and fixture inputs, then rerun the same run-console.sh command to retry.")
			for {
				unix.Pause()
			}
		}
		os.Exit(1)
	}
}
