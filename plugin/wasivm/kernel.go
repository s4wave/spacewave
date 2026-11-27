package wasivm_plugin

import (
	"context"
	"io"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_overlay "github.com/s4wave/spacewave/db/unixfs/overlay"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
	"github.com/s4wave/spacewave/plugin/wasivm/wayland"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// screenWidth and screenHeight define the Wayland compositor's screen size in
// pixels.
const screenWidth, screenHeight = 1024, 768

// startKernel boots the kernel for one run on the root filesystem rootfs,
// taking ownership of it. An in-memory upper layer holds the run's writes, so
// each run starts from the unchanged root filesystem. startKernel returns the
// run's console once the kernel is booting. The kernel runs until ctx is
// canceled, init exits or the system halts or restarts, then releases its
// resources, ends the console, and calls exited with whether the system
// restarted and the error that ended the run, or nil when it ended normally.
func startKernel(
	ctx context.Context,
	le *logrus.Entry,
	rootfs *rootfsSource,
	config *s4wave_vm.WasiVmConfig,
	exited func(reboot bool, runErr error),
) (*console, error) {
	// Open the root filesystem and the WebAssembly runtime.
	rootFS, releaseRootfs, err := openRootfs(ctx, le, rootfs)
	if err != nil {
		return nil, err
	}
	rt := newRuntime(ctx)

	// Create the kernel with the console as its terminal.
	stdin, input := io.Pipe()
	c := &console{input: input, downloads: newDownloads(rootfs.store)}
	initPath := config.GetInitPath()
	if initPath == "" {
		initPath = s4wave_vm.DefaultWasiVmInitPath
	}
	k, err := kernel.New(ctx, rt, &kernel.Config{
		Stdout:          c,
		Stderr:          c,
		Stdin:           stdin,
		VirtualTerminal: true,
		RootFS:          rootFS,
		InitPath:        initPath,
		InitArgs:        config.GetInitArgs(),
		Env:             config.GetEnv(),
	})
	if err != nil {
		_ = rt.Close(ctx)
		releaseRootfs()
		return nil, errors.Wrap(err, "create the kernel")
	}
	c.kernel = k
	c.resize = k.Resize

	// Serve the guest's Wayland clients, listening before init runs.
	if c.wayland, err = wayland.Listen(k, screenWidth, screenHeight); err != nil {
		le.WithError(err).Warn("wayland compositor did not start")
	} else {
		go func() {
			if err := c.wayland.Serve(ctx); err != nil && ctx.Err() == nil {
				le.WithError(err).Warn("wayland compositor ended")
			}
		}()
	}

	// Run init, then release the run and report how it ended.
	go func() {
		// Run init until it exits or the run ends, then release the run.
		bootErr := k.Boot(ctx)
		_ = input.Close()
		if c.wayland != nil {
			c.wayland.Close()
		}
		cleanupCtx := context.WithoutCancel(ctx)
		_ = k.Close(cleanupCtx)
		_ = rt.Close(cleanupCtx)
		releaseRootfs()

		// Report how the run ended.
		var code int32
		var runErr error
		var exitErr *kernel.ProcessExitError
		reboot := errors.Is(bootErr, kernel.ErrReboot)
		switch {
		case bootErr == nil, reboot, ctx.Err() != nil:
		case errors.As(bootErr, &exitErr):
			code = int32(exitErr.Code) //nolint:gosec // an exit code fits in a byte.
		case errors.Is(bootErr, kernel.ErrNoInit):
			runErr = errors.Errorf("init %s is not in the root filesystem", initPath)
		default:
			runErr = bootErr
		}
		c.exit(code, runErr)
		exited(reboot, runErr)
	}()
	return c, nil
}

// openRootfs opens src as the root filesystem of a run, with an in-memory
// upper layer over it. It takes ownership of src: the returned function
// releases both, and src is released on error.
func openRootfs(
	ctx context.Context,
	le *logrus.Entry,
	src *rootfsSource,
) (billy.Filesystem, func(), error) {
	// Open the object as the lower layer.
	fsType, _, err := unixfs_world.LookupFsType(ctx, src.ws, src.key)
	if err != nil {
		src.release()
		return nil, nil, errors.Wrapf(err, "look up the root filesystem %q", src.key)
	}
	lower := unixfs_world.NewFSCursorWithContext(ctx, le, src.ws, src.key, fsType, nil, false)

	// Overlay an in-memory upper layer and open the result as a filesystem.
	upper := unixfs_billy.NewBillyFSCursor(memfs.New(), "")
	handle, err := unixfs.NewFSHandle(unixfs_overlay.NewOverlayFSCursor(lower, upper))
	if err != nil {
		lower.Release()
		upper.Release()
		src.release()
		return nil, nil, errors.Wrap(err, "open the root filesystem")
	}
	release := func() {
		handle.Release()
		src.release()
	}
	return unixfs_billy.NewBillyFilesystem(ctx, handle, "/", time.Time{}), release, nil
}
