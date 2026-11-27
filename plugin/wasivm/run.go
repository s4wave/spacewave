package wasivm_plugin

import (
	"cmp"
	"context"
	"io"
	"maps"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// RunConfig configures one program run by Run.
type RunConfig struct {
	// Path is the program to run, or the default init when empty.
	Path string
	// Args are the program's arguments.
	Args []string
	// Env holds variables set over the kernel's default environment.
	Env map[string]string
	// Stdout receives the program's output. Nil discards it.
	Stdout io.Writer
	// Stderr receives the program's error output. Nil discards it.
	Stderr io.Writer
}

// Run boots the WasiVM image imageKey in ws with conf's program as init and
// returns its exit code once it exits. The program reads an empty input. An
// in-memory upper layer over the image's root filesystem holds the run's
// writes, so each run starts from the unchanged image. A published image's CDN
// Space stays mounted until the run ends. Canceling ctx stops the run.
func Run(ctx context.Context, le *logrus.Entry, b bus.Bus, ws world.WorldState, imageKey string, conf *RunConfig) (int, error) {
	// Open the image's root filesystem and the WebAssembly runtime.
	src, err := resolveImageRootfs(ctx, le, b, ws, imageKey)
	if err != nil {
		return 0, err
	}
	rootFS, releaseRootfs, err := openRootfs(ctx, le, src)
	if err != nil {
		return 0, err
	}
	defer releaseRootfs()
	rt := newRuntime(ctx)
	defer rt.Close(context.WithoutCancel(ctx))

	// Create the kernel with the program as init on a plain console.
	path := cmp.Or(conf.Path, s4wave_vm.DefaultWasiVmInitPath)
	env := kernel.DefaultEnv()
	maps.Copy(env, conf.Env)
	k, err := kernel.New(ctx, rt, &kernel.Config{
		Stdout:     writerOrDiscard(conf.Stdout),
		Stderr:     writerOrDiscard(conf.Stderr),
		Stdin:      strings.NewReader(""),
		TerminalFD: -1,
		RootFS:     rootFS,
		InitPath:   path,
		InitArgs:   conf.Args,
		Env:        env,
	})
	if err != nil {
		return 0, errors.Wrap(err, "create the kernel")
	}
	defer k.Close(context.WithoutCancel(ctx))

	// Run init and report how it ended.
	bootErr := k.Boot(ctx)
	var exitErr *kernel.ProcessExitError
	switch {
	case bootErr == nil:
		return 0, nil
	case errors.As(bootErr, &exitErr):
		return exitErr.Code, nil
	case errors.Is(bootErr, kernel.ErrNoInit):
		return 0, errors.Errorf("%s is not in image %s", path, imageKey)
	case errors.Is(bootErr, kernel.ErrReboot):
		return 0, errors.New("the program restarted the system")
	default:
		return 0, bootErr
	}
}

// writerOrDiscard returns w, or io.Discard when w is nil.
func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
