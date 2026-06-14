//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
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

type v86RunArgs struct {
	root      string
	rootMode  string
	assetDir  string
	v86Dir    string
	fsDir     string
	imageKey  string
	bootArgs  string
	memoryMb  uint
	enableJIT bool
	refresh   bool
}

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
		},
		Action: func(c *cli.Context) error {
			return runV86Interactive(c, args)
		},
	}
}

func runV86Interactive(c *cli.Context, args *v86RunArgs) error {
	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt)
	defer stop()

	boot, release, assets, err := buildV86RunBoot(ctx, args)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}

	instance, err := v86_wazero.InstantiateHostRuntime(ctx, assets.Wasm, v86_wazero.HostRuntimeOptions{})
	if err != nil {
		return errors.Wrap(err, "instantiate v86 runtime")
	}
	defer instance.Close(context.Background())
	if err := instance.InitCPU(ctx, boot); err != nil {
		return errors.Wrap(err, "initialize v86 CPU")
	}

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

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return errors.Wrap(err, "set terminal raw mode")
		}
		defer term.Restore(fd, state)
	}

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
	boot := v86_wazero.HostBootOptions{
		EnableJIT: args.enableJIT,
		BIOS:      bios,
		VGABIOS:   vgaBIOS,
		Kernel:    kernel,
		Cmdline:   args.bootArgs,
	}
	if args.memoryMb > 0 {
		boot.MemorySize = uint32(args.memoryMb) * 1024 * 1024
	}
	switch args.root {
	case "v86fs", "":
		rootMode, err := v86_wazero.ParseRootMode(args.rootMode)
		if err != nil {
			return v86_wazero.HostBootOptions{}, nil, nil, err
		}
		server, release, err := v86_wazero.OpenV86Root(rootMode, assets.RootfsTar)
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
	src     io.Reader
	quit    context.CancelFunc
	armed   bool
	pending []byte
}

func (e *serialEscapeReader) Read(p []byte) (int, error) {
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

func (e *serialEscapeReader) flush(p, out []byte) int {
	n := copy(p, out)
	if n < len(out) {
		e.pending = append(e.pending, out[n:]...)
	}
	return n
}
