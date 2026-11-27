package v86_wazero

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_overlay "github.com/s4wave/spacewave/db/unixfs/overlay"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/s4wave/spacewave/db/world"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
	"github.com/sirupsen/logrus"
)

const (
	// runInitPath is the script the guest's shell runs as init.
	runInitPath = "/.spacewave-run"
	// runExitPath is the file the script writes the program's exit code to.
	runExitPath = "/.spacewave-run-exit"
	// runCmdline boots the v86fs root with the shell running the script as init.
	runCmdline = "rw quiet init=/bin/sh root=v86fs rootfstype=v86fs rootflags= console=ttyS0 -- " + runInitPath
)

// RunConfig configures one program run by RunImage.
type RunConfig struct {
	// Path is the program to run.
	Path string
	// Args are the program's arguments.
	Args []string
	// Env holds KEY=VALUE pairs exported to the program.
	Env []string
	// Console receives the guest's serial console: the program's stdout and
	// stderr, and the kernel's messages. Nil discards it.
	Console io.Writer
	// MemorySize is the guest memory in bytes, or 0 for the default size.
	MemorySize uint32
}

// RunImage boots the V86Image imageKey in ws and runs conf's program until it
// exits, then halts the guest and returns the program's exit code. An
// in-memory upper layer over the image's rootfs holds the run's writes, so
// each run starts from the unchanged image. A published image's CDN Space
// stays mounted until the run ends. Canceling ctx stops the guest.
func RunImage(ctx context.Context, le *logrus.Entry, b bus.Bus, ws world.WorldState, imageKey string, conf *RunConfig) (int, error) {
	// Open the image's assets and read its boot files.
	assets := s4wave_vm_world.NewV86ImageAssets(ctx, le, b, ws, imageKey)
	defer assets.Release()
	boot, wasm, err := readImageBoot(ctx, assets)
	if err != nil {
		return 0, err
	}

	// Serve the rootfs under an upper layer holding the init script.
	upper := memfs.New()
	if err := billy_util.WriteFile(upper, runInitPath, runInitScript(conf), 0o755); err != nil {
		return 0, errors.Wrap(err, "write the init script")
	}
	server, releaseRoot, err := openRunRoot(ctx, assets, upper)
	if err != nil {
		return 0, err
	}
	defer releaseRoot()

	// Boot the guest.
	instance, err := InstantiateHostRuntime(ctx, wasm, HostRuntimeOptions{})
	if err != nil {
		return 0, errors.Wrap(err, "instantiate v86 runtime")
	}
	defer instance.Close(context.WithoutCancel(ctx))
	boot.Cmdline, boot.V86FSServer, boot.MemorySize = runCmdline, server, conf.MemorySize
	if err := instance.InitCPU(ctx, boot); err != nil {
		return 0, errors.Wrap(err, "initialize v86 CPU")
	}

	// Drive the guest until it halts, with console input that never ends.
	input, closeInput := io.Pipe()
	defer closeInput.Close()
	if err := instance.RunSerialConsole(ctx, input, conf.Console); err != nil {
		return 0, err
	}
	return readRunExit(upper)
}

// readImageBoot reads the image's BIOSes and kernel into boot options, and its
// emulator.
func readImageBoot(ctx context.Context, assets *s4wave_vm_world.V86ImageAssets) (HostBootOptions, []byte, error) {
	// Read each file from its asset.
	read := func(name, fileName string) ([]byte, error) {
		// Open the asset, then read the file from it.
		fsh, err := assets.Open(ctx, name)
		if err != nil {
			return nil, errors.Wrapf(err, "open the image's %s", name)
		}
		defer fsh.Release()
		data, err := readFSHandleAsset(ctx, fsh, fileName)
		return data, errors.Wrapf(err, "read the image's %s", name)
	}

	// Read the four files the boot needs.
	var boot HostBootOptions
	var wasm []byte
	var err error
	for _, f := range []struct {
		name, fileName string
		dst            *[]byte
	}{
		{"wasm", "v86.wasm", &wasm},
		{"seabios", "seabios.bin", &boot.BIOS},
		{"vgabios", "vgabios.bin", &boot.VGABIOS},
		{"kernel", "bzImage", &boot.Kernel},
	} {
		if *f.dst, err = read(f.name, f.fileName); err != nil {
			return boot, nil, err
		}
	}
	return boot, wasm, nil
}

// openRunRoot serves the image's rootfs with upper over it as the guest root.
// The returned function releases both layers.
func openRunRoot(ctx context.Context, assets *s4wave_vm_world.V86ImageAssets, upper billy.Filesystem) (*unixfs_v86fs.Server, func(), error) {
	lower, err := assets.Open(ctx, "rootfs")
	if err != nil {
		return nil, nil, errors.Wrap(err, "open the image's rootfs")
	}
	root := unixfs_overlay.NewOverlayFSCursor(
		unixfs.NewFSHandleCursor(lower, true, nil),
		unixfs_billy.NewBillyFSCursor(upper, ""),
	)
	return openV86RootServer(root)
}

// runInitScript returns the script that runs conf's program and records its
// exit code before halting the guest.
func runInitScript(conf *RunConfig) []byte {
	// Mount the kernel filesystems and export the environment.
	var s strings.Builder
	s.WriteString("mount -t proc proc /proc 2>/dev/null\nmount -t sysfs sysfs /sys 2>/dev/null\n")
	for _, kv := range conf.Env {
		s.WriteString("export " + shellQuote(kv) + "\n")
	}

	// Run the program, then record its exit code and halt.
	s.WriteString(shellQuote(conf.Path))
	for _, arg := range conf.Args {
		s.WriteString(" " + shellQuote(arg))
	}
	s.WriteString("\necho $? > " + runExitPath + "\nhalt -f\n")
	return []byte(s.String())
}

// shellQuote quotes s as one word of a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// readRunExit reads the exit code the init script wrote to upper.
func readRunExit(upper billy.Filesystem) (int, error) {
	data, err := billy_util.ReadFile(upper, runExitPath)
	if err != nil {
		return 0, errors.Wrap(err, "the guest halted without the program's exit code")
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return code, errors.Wrap(err, "parse the program's exit code")
}
