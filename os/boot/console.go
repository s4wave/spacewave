//go:build linux

package boot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// RunConsole composes a verified read-only system image, an existing writable
// Data filesystem and kernel mounts for one trusted serial shell in guest PID1.
// Backing stores and init's runtime remain outside the served roots. It never
// creates, resets or migrates a fixture. Hosted probes keep their separate Run.
// Native comparison access, when requested, exposes only /store/native-bench at
// /tmp/native-bench; that existing private folder must not hold database files.
func RunConsole(ctx context.Context, system, data, terminal string, nativeBench bool) (retErr error) {
	// Refuse hosted invocation before opening storage or changing any mount.
	if os.Getpid() != 1 {
		return errors.New("console requires actual guest PID1; use probe for hosted checks")
	}
	if system == "" || data == "" {
		return errors.New("console requires existing SPACEWAVE_OS_STORE and SPACEWAVE_OS_DATA directories")
	}
	println("Starting console: opening existing system and data files.")

	// Retain independent bootstrap paths; never remove a tree after failed unmount.
	base, err := os.MkdirTemp("/tmp", "spacewave-console-")
	if err != nil {
		return err
	}
	removeBase := true
	defer func() {
		if removeBase {
			retErr = errors.Join(retErr, os.RemoveAll(base))
		}
	}()

	// Read init from independent bootstrap storage, never its served image.
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	file, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()

	// Supply the privileged mount-helper handshake from the static bootstrap.
	if err := os.Symlink(executable, filepath.Join(base, "fusermount3")); err != nil {
		return err
	}
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", base); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, os.Setenv("PATH", oldPath)) }()

	// Verify every required image file/link before mounting or opening writable data.
	le := logrus.NewEntry(logrus.New())
	root, err := OpenRoot(ctx, le, system)
	if err != nil {
		return err
	}
	usersStopped := true
	defer func() {
		if usersStopped {
			retErr = errors.Join(retErr, root.Close())
		}
	}()

	// Complete the read-only image check before touching the writable filesystem.
	if err := root.VerifyImage(ctx, file); err != nil {
		return err
	}
	println("SPACEWAVE_OS_FROZEN")

	// Open only an existing supported data store and retain physical Close here.
	d, err := OpenData(ctx, le, data)
	if err != nil {
		return err
	}
	defer func() {
		if usersStopped {
			retErr = errors.Join(retErr, d.Close())
		}
	}()

	// Controller failure cancels requests and containment, but never closes storage.
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchCtx, stopWatch := context.WithCancel(context.Background())
	watched := make(chan struct{})
	go func() {
		err := d.Wait(watchCtx)
		if watchCtx.Err() == nil {
			cancel(err)
		}
		close(watched)
	}()
	defer func() { stopWatch(); <-watched }()

	// Reverse mount acquisition on every partial startup and after process reaping.
	var stopping atomic.Bool
	var mounts []*consoleMount
	var kernelMounts []string
	defer func() {
		// Keep stores and mounts live if containment could not prove users stopped.
		if !usersStopped {
			removeBase = false
			return
		}

		// Stop nested kernel users before joining either borrowed FUSE root.
		stopping.Store(true)
		for _, path := range slices.Backward(kernelMounts) {
			if err := unix.Unmount(path, 0); err != nil {
				removeBase = false
				retErr = errors.Join(retErr, err, unix.Unmount(path, unix.MNT_DETACH))
			}
		}
		for _, mount := range slices.Backward(mounts) {
			if err := mount.close(); err != nil {
				removeBase = false
				retErr = errors.Join(retErr, err)
			}
		}
		println("SPACEWAVE_OS_CONSOLE_SERVERS_JOINED")
		retErr = errors.Join(retErr, context.Cause(runCtx))
	}()

	// Serve the immutable image first so its existing /data mountpoint is available.
	rootPath := filepath.Join(base, "root")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		return err
	}
	for _, target := range []struct {
		// path selects the served image or its separate writable mountpoint.
		path string
		// readOnly selects the immutable system rather than writable data.
		readOnly bool
	}{{rootPath, true}, {filepath.Join(rootPath, "data"), false}} {
		handle := root.Handle()
		if !target.readOnly {
			handle = d.Handle()
		}
		mount, err := mountConsoleFS(runCtx, cancel, &stopping, le, target.path, handle, target.readOnly)
		if err != nil {
			return err
		}
		mounts = append(mounts, mount)
	}

	// Kernel interfaces and scratch storage overlay only the image's mountpoints.
	for _, target := range []struct {
		// path is relative to the served root.
		path string
		// fs selects the kernel filesystem.
		fs string
		// options supplies fixed kernel mount options.
		options string
	}{{"dev", "devtmpfs", "mode=0755"}, {"proc", "proc", ""}, {"tmp", "tmpfs", "mode=1777"}, {"dev/pts", "devpts", "newinstance,ptmxmode=0666,mode=0620"}} {
		path := filepath.Join(rootPath, target.path)
		if target.path == "dev/pts" {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		}
		if err := unix.Mount(target.fs, path, target.fs, unix.MS_NOSUID, target.options); err != nil {
			return err
		}
		kernelMounts = append(kernelMounts, path)
	}

	// Expose an explicitly prepared private comparison folder, never /store itself.
	if nativeBench {
		info, err := os.Lstat("/store/native-bench")
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			return errors.New("native comparison requires an existing private /store/native-bench directory (mode 0700)")
		}
		target := filepath.Join(rootPath, "tmp/native-bench")
		if err := os.Mkdir(target, 0o700); err != nil {
			return err
		}
		if err := unix.Mount("/store/native-bench", target, "", unix.MS_BIND, ""); err != nil {
			return err
		}
		kernelMounts = append(kernelMounts, target)
	}

	// Start only after helpers finish; from here one Wait4 authority owns children.
	if err := context.Cause(runCtx); err != nil {
		return err
	}
	usersStopped, err = runGuestShell(runCtx, executable, rootPath, terminal)
	return errors.Join(err, context.Cause(runCtx))
}
