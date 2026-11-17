//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"golang.org/x/term"

	v86_wazero "github.com/s4wave/spacewave/forge/lib/v86/wazero"
)

// serialEscapePrefix is the qemu-style console escape lead byte (Ctrl-A); the
// next byte selects an action, so Ctrl-A x quits and Ctrl-A Ctrl-A sends a
// literal Ctrl-A to the guest.
const serialEscapePrefix = 0x01

// v86RunArgs selects the local image, root backing, and virtual machine options.
type v86RunArgs struct {
	// root selects v86fs or the host9p root device.
	root string
	// rootMode selects the v86fs root's backing and persistence.
	rootMode string
	// assetDir overrides the directory holding all boot artifacts.
	assetDir string
	// v86Dir overrides the emulator and BIOS artifact directory.
	v86Dir string
	// fsDir overrides the kernel and root filesystem artifact directory.
	fsDir string
	// imageKey selects the image used when hydrating artifacts from the CDN.
	imageKey string
	// bootArgs overrides the guest kernel command line.
	bootArgs string
	// memoryMb is the guest memory in MiB, or zero for the runtime default.
	memoryMb uint
	// enableJIT enables the emulator's JIT compiler.
	enableJIT bool
	// refresh forces cached assets to be hydrated again.
	refresh bool
	// net attaches the guest NIC to the host's usermode network stack.
	net bool
}

// newVmRunCommand builds the local VM command and its serial console action.
func newVmRunCommand() *cli.Command {
	args := &v86RunArgs{root: "v86fs", rootMode: "ram", memoryMb: 128}
	return &cli.Command{
		Name:  "run",
		Usage: "boot a v86 VM locally and attach an interactive serial console",
		Description: "Run boots a v86 image under the in-process wazero host runtime and " +
			"bridges COM1 to this terminal, like qemu -nographic. It needs local image " +
			"artifacts: point --asset-dir at a directory with v86.wasm, seabios.bin, " +
			"vgabios.bin, bzImage and rootfs.tar (or fs.json + flat/ for --root host9p), " +
			"or set --v86-dir and --fs-dir (env V86_DIR, V86FS_DIR, V86_WAZERO_ASSET_DIR " +
			"are honored as defaults). Press Ctrl-A x to quit.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "root", Usage: "guest root device: v86fs or host9p", Value: "v86fs", Destination: &args.root},
			&cli.StringFlag{Name: "root-mode", Usage: "guest root backing: readonly|ram|disk=<path>|volume=<file>|daemon=<space>", Value: "ram", Destination: &args.rootMode},
			&cli.StringFlag{Name: "asset-dir", Usage: "directory holding v86.wasm, seabios.bin, vgabios.bin, bzImage, rootfs.tar/fs.json", Destination: &args.assetDir},
			&cli.StringFlag{Name: "v86-dir", Usage: "v86 source tree (build/v86.wasm, bios/)", Destination: &args.v86Dir},
			&cli.StringFlag{Name: "fs-dir", Usage: "v86fs artifact dir (bzImage, rootfs.tar, fs.json, flat/)", Destination: &args.fsDir},
			&cli.StringFlag{Name: "image-key", Usage: "V86Image object key for CDN hydration fallback", Destination: &args.imageKey},
			&cli.StringFlag{Name: "boot-args", Usage: "override the kernel command line", Destination: &args.bootArgs},
			&cli.UintFlag{Name: "memory-mb", Usage: "guest memory in MiB (0 uses the runtime default)", Value: 128, Destination: &args.memoryMb},
			&cli.BoolFlag{Name: "jit", Usage: "enable the v86 JIT", Destination: &args.enableJIT},
			&cli.BoolFlag{Name: "refresh", Usage: "force CDN re-hydration of cached assets", Destination: &args.refresh},
			&cli.BoolFlag{Name: "net", Usage: "attach an NE2000 NIC backed by the usermode network stack (guest egress via the host)", Destination: &args.net},
		},
		Action: func(c *cli.Context) error {
			return runV86Interactive(c, args)
		},
	}
}

// runV86Interactive boots the selected image and attaches the terminal until quit.
func runV86Interactive(c *cli.Context, args *v86RunArgs) error {
	// Stop the VM when the process is interrupted.
	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt)
	defer stop()

	// Build the boot options and release the root handle when the command returns.
	boot, release, assets, err := buildV86RunBoot(ctx, args)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}

	// Read the emulator.
	wasm, err := os.ReadFile(assets.Wasm)
	if err != nil {
		return errors.Wrap(err, "read v86 wasm")
	}

	// Instantiate the host runtime and initialize the CPU.
	instance, err := v86_wazero.InstantiateHostRuntime(ctx, wasm, v86_wazero.HostRuntimeOptions{})
	if err != nil {
		return errors.Wrap(err, "instantiate v86 runtime")
	}
	defer instance.Close(context.Background())
	if err := instance.InitCPU(ctx, boot); err != nil {
		return errors.Wrap(err, "initialize v86 CPU")
	}

	// Describe the root and announce the boot.
	rootDesc := args.root
	if rootDesc == "" {
		rootDesc = "v86fs"
	}
	if rootDesc == "v86fs" {
		rootDesc += " root (" + args.rootMode + ")"
	} else {
		rootDesc += " root"
	}
	os.Stderr.WriteString("v86: booting " + rootDesc + " from " + assets.Dir + "; press Ctrl-A x to quit\n")

	// Put the terminal in raw mode when stdin is a terminal.
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return errors.Wrap(err, "set terminal raw mode")
		}
		defer term.Restore(fd, state)
	}

	// Run the serial console until Ctrl-A x or interrupt.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	in := &serialEscapeReader{src: os.Stdin, quit: cancel}
	runErr := instance.RunSerialConsole(runCtx, in, os.Stdout)
	os.Stderr.WriteString("\r\n")
	if runErr != nil && !stderrors.Is(runErr, context.Canceled) {
		return runErr
	}
	return nil
}

// buildV86RunBoot resolves image artifacts and assembles the boot options plus
// the chosen root device. The returned release func drops the v86fs root handle
// and is nil for the host9p path.
func buildV86RunBoot(ctx context.Context, args *v86RunArgs) (v86_wazero.HostBootOptions, func(), *v86_wazero.AssetSet, error) {
	// Apply asset flags and resolve the v86 asset set.
	assetOpts := v86_wazero.OptionsFromEnv()
	if args.assetDir != "" {
		assetOpts.AssetDir = args.assetDir
	}
	if args.v86Dir != "" {
		assetOpts.V86Dir = args.v86Dir
	}
	if args.fsDir != "" {
		assetOpts.V86FSDir = args.fsDir
	}
	if args.imageKey != "" {
		assetOpts.ImageKey = args.imageKey
	}
	if args.refresh {
		assetOpts.Refresh = true
	}
	assets, err := v86_wazero.ResolveAssets(ctx, assetOpts)
	if err != nil {
		return v86_wazero.HostBootOptions{}, nil, nil, errors.Wrap(err, "resolve v86 assets")
	}

	// Read SeaBIOS, VGABIOS, and the kernel.
	bios, err := os.ReadFile(assets.SeaBIOS)
	if err != nil {
		return v86_wazero.HostBootOptions{}, nil, nil, errors.Wrap(err, "read SeaBIOS")
	}
	vgaBIOS, err := os.ReadFile(assets.VGABIOS)
	if err != nil {
		return v86_wazero.HostBootOptions{}, nil, nil, errors.Wrap(err, "read VGABIOS")
	}
	kernel, err := os.ReadFile(assets.Kernel)
	if err != nil {
		return v86_wazero.HostBootOptions{}, nil, nil, errors.Wrap(err, "read kernel")
	}

	// Assemble the boot options, memory, network, and root device.
	boot := v86_wazero.HostBootOptions{
		EnableJIT: args.enableJIT,
		BIOS:      bios,
		VGABIOS:   vgaBIOS,
		Kernel:    kernel,
		Cmdline:   args.bootArgs,
	}
	if args.memoryMb > 0 {
		if args.memoryMb > math.MaxUint32/(1024*1024) {
			return v86_wazero.HostBootOptions{}, nil, nil, errors.New("memory-mb exceeds the runtime addressable size")
		}
		boot.MemorySize = uint32(args.memoryMb * 1024 * 1024) //nolint:gosec // the MaxUint32 division check bounds the byte size.
	}
	if args.net {
		boot.Networking = &v86_wazero.NetworkConfig{}
	}
	switch args.root {
	case "v86fs", "":
		rootMode, err := v86_wazero.ParseRootMode(args.rootMode)
		if err != nil {
			return v86_wazero.HostBootOptions{}, nil, nil, err
		}
		server, release, err := v86_wazero.OpenV86Root(ctx, rootMode, assets)
		if err != nil {
			return v86_wazero.HostBootOptions{}, nil, nil, err
		}
		boot.V86FSServer = server
		return boot, release, assets, nil
	case "host9p":
		host9p, err := v86_wazero.OpenHost9PFS(filepath.Dir(assets.RootfsJSON))
		if err != nil {
			return v86_wazero.HostBootOptions{}, nil, nil, errors.Wrap(err, "open host9p root")
		}
		boot.Host9PFS = host9p
		return boot, nil, assets, nil
	default:
		return v86_wazero.HostBootOptions{}, nil, nil, errors.Errorf("unknown root device %q, want v86fs or host9p", args.root)
	}
}

// serialEscapeReader forwards stdin to the guest while watching for the Ctrl-A
// console escape. Ctrl-A x calls quit to stop the console; Ctrl-A Ctrl-A emits
// one literal Ctrl-A. Filtered output that overflows the caller buffer is held
// in pending and served on the next read.
type serialEscapeReader struct {
	// src supplies the terminal's unfiltered input.
	src io.Reader
	// quit cancels the console when the quit escape is read.
	quit context.CancelFunc
	// armed records a Ctrl-A prefix whose following byte has not arrived.
	armed bool
	// pending holds filtered bytes that did not fit in the previous read buffer.
	pending []byte
}

// Read filters console escapes and retains output that does not fit in p.
// Ctrl-A x cancels the console and returns EOF after any preceding output.
func (e *serialEscapeReader) Read(p []byte) (int, error) {
	// Read stdin, treating Ctrl-A x as quit.
	if len(e.pending) != 0 {
		n := copy(p, e.pending)
		e.pending = e.pending[n:]
		return n, nil
	}
	n, err := e.src.Read(p)
	if n == 0 {
		return 0, err
	}
	out := make([]byte, 0, n+1)
	for _, b := range p[:n] {
		if e.armed {
			e.armed = false
			switch b {
			case 'x':
				e.quit()
				return e.flush(p, out), io.EOF
			case serialEscapePrefix:
				out = append(out, serialEscapePrefix)
			default:
				out = append(out, serialEscapePrefix, b)
			}
			continue
		}
		if b == serialEscapePrefix {
			e.armed = true
			continue
		}
		out = append(out, b)
	}
	return e.flush(p, out), err
}

// flush copies filtered input to p and retains the remainder for the next read.
func (e *serialEscapeReader) flush(p, out []byte) int {
	n := copy(p, out)
	if n < len(out) {
		e.pending = append(e.pending, out[n:]...)
	}
	return n
}
