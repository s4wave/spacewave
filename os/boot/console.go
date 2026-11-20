//go:build linux

package boot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// RunConsole composes a verified read-only system image, a writable Data
// filesystem and kernel mounts for one trusted serial shell in guest PID1.
// Backing stores and init's runtime remain outside the served roots. First
// boot creates the data Volume, and so its key, once the image verifies; it
// never resets or migrates a store. Hosted probes keep their separate Run.
// Native comparison access, when requested, exposes only /store/native-bench at
// /tmp/native-bench; that existing private folder must not hold database files.
// Closing shutdown requests ordered cleanup while healthy FUSE remains live;
// cancelling ctx represents failure and disconnects FUSE before containment.
func RunConsole(ctx context.Context, shutdown <-chan struct{}, system, data, config, terminal string, nativeBench bool) (retErr error) {
	// Refuse hosted invocation before opening storage or changing any mount.
	if os.Getpid() != 1 {
		return errors.New("console requires actual guest PID1; use probe for hosted checks")
	}
	if system == "" || data == "" {
		return errors.New("console requires SPACEWAVE_OS_STORE and SPACEWAVE_OS_DATA directories")
	}
	println("Starting console: opening system and data files.")

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

	// Verify the card's authorization against the pinned signers before
	// opening the image, and serve only the root it names.
	authorized, err := AuthorizedRoot(system, data)
	if err != nil {
		return err
	}

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
	if !root.ImageRoot().EqualsRef(authorized) {
		return errors.New("image authorization: selected image root " + root.ImageRoot().MarshalString() + " is not the authorized root " + authorized.MarshalString())
	}

	// Complete the read-only image check before touching the writable filesystem.
	if err := root.VerifyImage(ctx, file); err != nil {
		return err
	}
	println("SPACEWAVE_OS_FROZEN")

	// Open the data store, creating it on first boot, and retain physical Close here.
	d, err := EnsureData(ctx, le, data)
	if err != nil {
		return err
	}
	defer func() {
		if usersStopped {
			retErr = errors.Join(retErr, d.Close())
		}
	}()

	// Configuration has a separate persistent binding from repository files.
	var configuration *Data
	if config != "" {
		configuration, err = OpenData(ctx, le, config)
		if err != nil {
			return err
		}
		defer func() {
			if usersStopped {
				retErr = errors.Join(retErr, configuration.Close())
			}
		}()
		if err := d.Handle().MkdirAllPath(ctx, "config", 0o755, time.Now()); err != nil {
			return err
		}
	}
	println("SPACEWAVE_OS_BINDING role=files volume=" + d.VolumeID() + " object=" + dataRootKey)
	if configuration != nil {
		println("SPACEWAVE_OS_BINDING role=config volume=" + configuration.VolumeID() + " object=" + dataRootKey)
	}

	// Watch both storage owners; neither may close below filesystem users.
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchCtx, stopWatch := context.WithCancel(context.Background())
	var watchers sync.WaitGroup
	for _, store := range []*Data{d, configuration} {
		if store != nil {
			watchers.Go(func() {
				err := store.Wait(watchCtx)
				if watchCtx.Err() == nil {
					cancel(err)
				}
			})
		}
	}
	defer func() { stopWatch(); watchers.Wait() }()

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

	// Each mount carries its handle explicitly; writable roles cannot alias by mode.
	type mountTarget struct {
		// path is the explicit guest mountpoint for this binding.
		path string
		// readOnly distinguishes the frozen image from writable bindings.
		readOnly bool
		// handle borrows this binding's root until unmount and Serve join.
		handle *unixfs.FSHandle
	}
	targets := []mountTarget{{rootPath, true, root.Handle()}, {filepath.Join(rootPath, "data"), false, d.Handle()}}
	if configuration != nil {
		targets = append(targets, mountTarget{filepath.Join(rootPath, "data/config"), false, configuration.Handle()})
	}
	for _, target := range targets {
		mount, err := mountConsoleFS(runCtx, cancel, &stopping, le, target.path, target.handle, target.readOnly)
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
	usersStopped, err = runGuestShell(runCtx, shutdown, executable, rootPath, terminal)
	return errors.Join(err, context.Cause(runCtx))
}
