//go:build !js

// Command wasivm runs a WebAssembly program under the WasiVM kernel, for
// debugging the kernel outside Spacewave.
//
// Run one BusyBox binary, which also provides the common applets. Flags
// anywhere on the line belong to wasivm, so "--" passes the rest to the
// program:
//
//	wasivm busybox.wasm sh
//	wasivm -- busybox.wasm sh -c 'echo hello'
//
// Boot a root filesystem directory:
//
//	wasivm --root rootfs/out [--init /bin/sh] [-- args...]
package main

import (
	"context"
	"os"

	"github.com/pkg/errors"

	"github.com/aperturerobotics/cli"
	"github.com/go-git/go-billy/v6/osfs"
	"golang.org/x/term"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine/native"
)

// runArgs are the arguments of the wasivm command.
type runArgs struct {
	// root is a host directory to boot as the root filesystem.
	root string
	// init is the program to boot from the root filesystem.
	init string
	// debug logs every system call to stderr.
	debug bool
}

// BuildFlags returns the wasivm flags.
func (a *runArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "root",
			Usage:       "boot this host directory as the root filesystem",
			Destination: &a.root,
		},
		&cli.StringFlag{
			Name:        "init",
			Usage:       "program to boot from the root filesystem",
			Value:       "/bin/sh",
			Destination: &a.init,
		},
		&cli.BoolFlag{
			Name:        "debug",
			Usage:       "log every system call to stderr",
			Destination: &a.debug,
		},
	}
}

// Run boots the root filesystem, or runs the program named by the first
// argument.
func (a *runArgs) Run(c *cli.Context) error {
	// Build the kernel configuration for the chosen mode.
	cfg := &kernel.Config{
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		Stdin:         os.Stdin,
		TerminalFD:    -1,
		DebugSyscalls: a.debug,
	}
	if term.IsTerminal(0) {
		cfg.TerminalFD = 0
	}
	var wasm []byte
	args := c.Args().Slice()
	if a.root != "" {
		root, err := os.OpenRoot(a.root)
		if err != nil {
			return err
		}
		defer root.Close()
		if cfg.RootFS, err = osfs.FromRoot(root); err != nil {
			return err
		}
		cfg.InitPath = a.init
		cfg.InitArgs = args
	} else {
		if len(args) == 0 {
			return errors.New("name a WebAssembly program, or boot a directory with --root")
		}
		var err error
		if wasm, err = os.ReadFile(args[0]); err != nil {
			return err
		}
		cfg.Files = kernel.BusyboxApplets(wasm)
	}

	// Boot init, booting a new kernel each time the system restarts.
	ctx := c.Context
	if a.root != "" {
		for {
			err := runKernel(ctx, cfg, func(k *kernel.Kernel) error { return k.Boot(ctx) })
			if !errors.Is(err, kernel.ErrReboot) {
				return err
			}
		}
	}

	// Run the program until it exits.
	return runKernel(ctx, cfg, func(k *kernel.Kernel) error {
		return k.RunInteractive(ctx, wasm, args...)
	})
}

// runKernel starts a kernel on a new runtime, calls run and closes both.
func runKernel(ctx context.Context, cfg *kernel.Config, run func(k *kernel.Kernel) error) error {
	// Start the kernel. It serves the runtime alone.
	rt := native.New(ctx)
	defer rt.Close(ctx)
	k, err := kernel.New(ctx, rt, cfg)
	if err != nil {
		return err
	}
	defer k.Close(ctx)

	// Run until the program exits or the system halts or restarts.
	return run(k)
}

func main() {
	// Build the command.
	var args runArgs
	app := &cli.App{
		Name:            "wasivm",
		Usage:           "run a WebAssembly program under the WasiVM kernel",
		ArgsUsage:       "[--] [program.wasm] [args...]",
		HideVersion:     true,
		HideHelpCommand: true,
		Flags:           args.BuildFlags(),
		Action:          args.Run,
	}

	// Exit with the program's exit status.
	err := app.Run(os.Args)
	var exitErr *kernel.ProcessExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
