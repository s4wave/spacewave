//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"io/fs"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"

	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_cdn "github.com/s4wave/spacewave/sdk/cdn"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// newVmCommand builds the vm command group.
func newVmCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	// Declare the shared VM flags and return the command group.
	var statePath string
	var sessionIdx uint
	var spaceID string
	flags := append(clientFlags(&statePath, &sessionIdx), vmSpaceFlag(&spaceID))
	return &cli.Command{
		Name:    "vm",
		Aliases: []string{"vms"},
		Usage:   "manage virtual machines",
		Flags:   flags,
		Subcommands: []*cli.Command{
			newVmListCommand(&statePath, &sessionIdx, &spaceID),
			newVmInfoCommand(&statePath, &sessionIdx, &spaceID),
			newVmCreateCommand(&statePath, &sessionIdx, &spaceID),
			newVmStartCommand(&statePath, &sessionIdx, &spaceID),
			newVmStopCommand(&statePath, &sessionIdx, &spaceID),
			newVmWatchCommand(&statePath, &sessionIdx, &spaceID),
			newVmConsoleCommand(&statePath, &sessionIdx, &spaceID),
			newVmImageCommand(&statePath, &sessionIdx, &spaceID),
			newVmRunCommand(),
		},
	}
}

func newVmStartCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var wait bool
	return &cli.Command{
		Name:      "start",
		Usage:     "request a VM start",
		ArgsUsage: "<vm-key>",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "wait", Usage: "wait for running, stopped, or error state", Destination: &wait},
		},
		Action: func(c *cli.Context) error {
			// Require a VM key, start it, and watch unless waiting was disabled.
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			if err := setVMState(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key, s4wave_vm.VmState_VmState_STARTING); err != nil {
				return err
			}
			if !wait {
				os.Stdout.WriteString("starting\n")
				return nil
			}
			return watchVM(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key)
		},
	}
}

func newVmWatchCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "watch",
		Usage:     "watch VM runtime state",
		ArgsUsage: "<vm-key>",
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			return watchVM(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key)
		},
	}
}

func newVmStopCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var wait bool
	return &cli.Command{
		Name:      "stop",
		Usage:     "request a VM stop",
		ArgsUsage: "<vm-key>",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "wait", Usage: "wait for stopped or error state", Destination: &wait},
		},
		Action: func(c *cli.Context) error {
			// Require a VM key, stop it, and watch when waiting was requested.
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			if err := setVMState(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key, s4wave_vm.VmState_VmState_STOPPED); err != nil {
				return err
			}
			if !wait {
				os.Stdout.WriteString("stopping\n")
				return nil
			}
			return watchVM(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key)
		},
	}
}

func newVmCreateCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create a VM",
		Subcommands: []*cli.Command{
			newVmCreateV86Command(statePath, sessionIdx, spaceID),
			newVmCreateWasiVmCommand(statePath, sessionIdx, spaceID),
		},
	}
}

func newVmCreateV86Command(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	args := &v86VMCreateArgs{memoryMb: 256, vgaMemoryMb: 8}
	return &cli.Command{
		Name:      "v86",
		Usage:     "create a v86 VM",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "as", Usage: "destination VM object key", Destination: &args.objectKey},
			&cli.StringFlag{Name: "image", Usage: "V86Image object key", Required: true, Destination: &args.imageObjectKey},
			&cli.UintFlag{Name: "memory-mb", Usage: "memory size in MiB", Value: 256, Destination: &args.memoryMb},
			&cli.UintFlag{Name: "vga-memory-mb", Usage: "VGA memory size in MiB", Value: 8, Destination: &args.vgaMemoryMb},
			&cli.BoolFlag{Name: "networking", Usage: "enable networking", Destination: &args.networking},
			&cli.BoolFlag{Name: "serial", Usage: "enable serial console", Destination: &args.serialEnabled},
			&cli.StringFlag{Name: "boot-args", Usage: "kernel boot arguments", Destination: &args.bootArgs},
			&cli.StringFlag{Name: "runtime-plugin-id", Usage: "runtime plugin id", Destination: &args.runtimePluginID},
			&cli.StringSliceFlag{Name: "mount", Usage: "guest mount /path=objectKey[:rw|:ro]", Destination: &args.mounts},
		},
		Action: func(c *cli.Context) error {
			// Require a name, create the VM, and print its key.
			name := c.Args().First()
			if name == "" {
				return errors.New("VM name required")
			}
			key, err := createV86VM(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, name, args)
			if err != nil {
				return err
			}
			os.Stdout.WriteString(key + "\n")
			return nil
		},
	}
}

func newVmListCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list VMs in a space",
		Action: func(c *cli.Context) error {
			vms, err := readVMs(c, *statePath, sessionIndex32(*sessionIdx), *spaceID)
			if err != nil {
				return err
			}
			return writeVMList(vms, c.String("output"))
		},
	}
}

func newVmInfoCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "show VM metadata and runtime configuration",
		ArgsUsage: "<vm-key>",
		Action: func(c *cli.Context) error {
			// Require a VM key, read it, and write its info.
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			vm, err := readVM(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key)
			if err != nil {
				return err
			}
			return writeVMInfo(vm, c.String("output"))
		},
	}
}

func newVmCreateWasiVmCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	args := &wasiVmCreateArgs{}
	return &cli.Command{
		Name:      "wasivm",
		Usage:     "create a WasiVM on a root filesystem",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "as", Usage: "destination VM object key", Destination: &args.objectKey},
			&cli.StringFlag{Name: "rootfs", Usage: "root filesystem UnixFS object key", Destination: &args.rootfsObjectKey},
			&cli.StringFlag{Name: "rootfs-tar", Usage: "import the root filesystem from a tar", Destination: &args.rootfsTarPath},
			&cli.StringFlag{Name: "image", Usage: "WasiVM image object key supplying the root filesystem", Destination: &args.imageObjectKey},
			&cli.StringFlag{Name: "init", Usage: "init program path in the root filesystem", Destination: &args.initPath},
			&cli.StringSliceFlag{Name: "init-arg", Usage: "init program argument", Destination: &args.initArgs},
			&cli.StringSliceFlag{Name: "env", Usage: "init environment KEY=VALUE", Destination: &args.env},
		},
		Action: func(c *cli.Context) error {
			// Require a name.
			name := c.Args().First()
			if name == "" {
				return errors.New("VM name required")
			}

			// Require exactly one root filesystem source.
			sources := 0
			for _, source := range []string{args.rootfsObjectKey, args.rootfsTarPath, args.imageObjectKey} {
				if source != "" {
					sources++
				}
			}
			if sources != 1 {
				return errors.New("exactly one of --rootfs, --rootfs-tar or --image is required")
			}

			// Create the VM and print its key.
			key, err := createWasiVm(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, name, args)
			if err != nil {
				return err
			}
			os.Stdout.WriteString(key + "\n")
			return nil
		},
	}
}

func newVmImageCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:    "image",
		Aliases: []string{"images"},
		Usage:   "manage VM images",
		Subcommands: []*cli.Command{
			newVmImageV86Command(statePath, sessionIdx, spaceID),
		},
	}
}

func newVmImageV86Command(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "v86",
		Usage: "manage v86 VM images",
		Subcommands: []*cli.Command{
			newVmImageV86ListCommand(statePath, sessionIdx, spaceID),
			newVmImageV86InfoCommand(statePath, sessionIdx, spaceID),
			newVmImageV86CopyFromCdnCommand(statePath, sessionIdx, spaceID),
			newVmImageV86ImportCommand(statePath, sessionIdx, spaceID),
		},
	}
}

func newVmImageV86ListCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list v86 images in a space",
		Action: func(c *cli.Context) error {
			images, err := readV86Images(c, *statePath, sessionIndex32(*sessionIdx), *spaceID)
			if err != nil {
				return err
			}
			return writeV86ImageList(images, c.String("output"))
		},
	}
}

func newVmImageV86ImportCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "import local v86 image artifacts into a space",
		Subcommands: []*cli.Command{
			newVmImageV86ImportTarCommand(statePath, sessionIdx, spaceID),
		},
	}
}

func newVmImageV86ImportTarCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	args := &v86ImageImportTarArgs{}
	return &cli.Command{
		Name:  "tar",
		Usage: "import local v86 artifacts and a rootfs tar into a space",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Usage: "image display name", Required: true, Destination: &args.name},
			&cli.StringFlag{Name: "wasm", Usage: "path to v86.wasm", Required: true, Destination: &args.wasmPath},
			&cli.StringFlag{Name: "seabios", Usage: "path to seabios.bin", Required: true, Destination: &args.seabiosPath},
			&cli.StringFlag{Name: "vgabios", Usage: "path to vgabios.bin", Required: true, Destination: &args.vgabiosPath},
			&cli.StringFlag{Name: "kernel", Usage: "path to bzImage", Required: true, Destination: &args.kernelPath},
			&cli.StringFlag{Name: "rootfs-tar", Usage: "path to rootfs tar", Required: true, Destination: &args.rootfsTarPath},
			&cli.StringFlag{Name: "as", Usage: "destination image object key", Destination: &args.objectKey},
			&cli.StringFlag{Name: "version", Usage: "image version", Destination: &args.version},
			&cli.StringFlag{Name: "distro", Usage: "image distro", Destination: &args.distro},
			&cli.StringSliceFlag{Name: "tag", Usage: "image discovery tag", Destination: &args.tags},
			&cli.StringFlag{Name: "description", Usage: "image description", Destination: &args.description},
			&cli.StringFlag{Name: "kernel-version", Usage: "kernel version", Destination: &args.kernelVersion},
		},
		Action: func(c *cli.Context) error {
			key, err := importV86ImageTar(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, args)
			if err != nil {
				return err
			}
			os.Stdout.WriteString(key + "\n")
			return nil
		},
	}
}

func newVmImageV86CopyFromCdnCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var dstKey string
	var cdnID string
	return &cli.Command{
		Name:      "copy-from-cdn",
		Usage:     "copy a published v86 image from the CDN into a space",
		ArgsUsage: "<cdn-image-key>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "as",
				Usage:       "destination image object key (defaults to source key)",
				Destination: &dstKey,
			},
			&cli.StringFlag{
				Name:        "cdn-id",
				Usage:       "configured CDN id (empty uses the default CDN)",
				Destination: &cdnID,
			},
		},
		Action: func(c *cli.Context) error {
			// Require source and destination keys, copy the CDN image, and print the destination.
			srcKey := c.Args().First()
			if srcKey == "" {
				return errors.New("CDN image key required")
			}
			if dstKey == "" {
				dstKey = srcKey
			}
			if err := copyV86ImageFromCdn(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, cdnID, srcKey, dstKey); err != nil {
				return err
			}
			os.Stdout.WriteString(dstKey + "\n")
			return nil
		},
	}
}

func newVmImageV86InfoCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "show v86 image metadata and asset edges",
		ArgsUsage: "<image-key>",
		Action: func(c *cli.Context) error {
			// Require an image key, read it, and write its info.
			key := c.Args().First()
			if key == "" {
				return errors.New("image key required")
			}
			image, err := readV86Image(c, *statePath, sessionIndex32(*sessionIdx), *spaceID, key)
			if err != nil {
				return err
			}
			return writeV86ImageInfo(image, c.String("output"))
		},
	}
}

func vmSpaceFlag(dest *string) cli.Flag {
	return &cli.StringFlag{
		Name:        "space",
		Usage:       "space ID or name",
		EnvVars:     []string{"SPACEWAVE_SPACE"},
		Destination: dest,
	}
}

type v86ImageCLIEntry struct {
	objectKey string
	image     *s4wave_vm.V86Image
	assets    map[string]string
}

type v86ImageImportTarArgs struct {
	name          string
	wasmPath      string
	seabiosPath   string
	vgabiosPath   string
	kernelPath    string
	rootfsTarPath string
	objectKey     string
	version       string
	distro        string
	tags          cli.StringSlice
	description   string
	kernelVersion string
}

type v86VMCreateArgs struct {
	objectKey       string
	imageObjectKey  string
	memoryMb        uint
	vgaMemoryMb     uint
	networking      bool
	serialEnabled   bool
	bootArgs        string
	runtimePluginID string
	mounts          cli.StringSlice
}

type wasiVmCreateArgs struct {
	objectKey       string
	rootfsObjectKey string
	rootfsTarPath   string
	imageObjectKey  string
	initPath        string
	initArgs        cli.StringSlice
	env             cli.StringSlice
}

func vmMemoryMiB(value uint) (uint32, error) {
	if value > math.MaxUint32 {
		return 0, errors.Errorf("memory size exceeds uint32 range: %d MiB", value)
	}
	return uint32(value), nil //nolint:gosec // the MaxUint32 check bounds the VM configuration field.
}

func readV86Images(c *cli.Context, statePath string, sessionIdx uint32, spaceID string) ([]*v86ImageCLIEntry, error) {
	var out []*v86ImageCLIEntry
	err := withVmWorldTx(c, statePath, sessionIdx, spaceID, false, func(tx world.WorldState) error {
		// List image keys, sort them, and read each image.
		keys, err := world_types.ListObjectsWithType(c.Context, tx, s4wave_vm.V86ImageTypeID)
		if err != nil {
			return errors.Wrap(err, "list v86 images")
		}
		slices.Sort(keys)
		out = make([]*v86ImageCLIEntry, 0, len(keys))
		for _, key := range keys {
			img, err := readV86ImageFromTx(c, tx, key)
			if err != nil {
				return err
			}
			out = append(out, img)
		}
		return nil
	})
	return out, err
}

func copyV86ImageFromCdn(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	cdnID string,
	srcKey string,
	dstKey string,
) error {
	// Connect to the daemon.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the selected session.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Resolve the Space ID.
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}

	// Open the CDN resource.
	cdnSvc, releaseCdn, err := client.accessCdn(ctx, cdnID)
	if err != nil {
		return err
	}
	defer releaseCdn()

	// Copy the image into the Space and drain the progress stream.
	strm, err := cdnSvc.CopyV86ImageToSpace(ctx, &s4wave_cdn.CopyV86ImageToSpaceRequest{
		SessionIdx:   sessionIdx,
		DstSpaceId:   sid,
		SrcObjectKey: srcKey,
		DstObjectKey: dstKey,
	})
	if err != nil {
		return errors.Wrap(err, "copy v86 image from CDN")
	}
	defer strm.Close()
	for {
		if _, err := strm.Recv(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.Wrap(err, "receive v86 image copy progress")
		}
	}
}

func createV86VM(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	name string,
	args *v86VMCreateArgs,
) (string, error) {
	// Parse the mounts and memory sizes.
	mounts, err := parseV86MountFlags(args.mounts.Value())
	if err != nil {
		return "", err
	}
	memoryMb, err := vmMemoryMiB(args.memoryMb)
	if err != nil {
		return "", err
	}
	vgaMemoryMb, err := vmMemoryMiB(args.vgaMemoryMb)
	if err != nil {
		return "", err
	}

	// Default the VM object key and build the create operation.
	vmKey := args.objectKey
	if vmKey == "" {
		vmKey = "vm/v86/" + ulid.NewULID()
	}
	op := s4wave_vm.NewCreateVmV86Op(vmKey, name, args.imageObjectKey, time.Now())
	op.Config = &s4wave_vm.V86Config{
		MemoryMb:        memoryMb,
		VgaMemoryMb:     vgaMemoryMb,
		Networking:      args.networking,
		SerialEnabled:   args.serialEnabled,
		BootArgs:        args.bootArgs,
		RuntimePluginId: args.runtimePluginID,
		Mounts:          mounts,
	}

	// Create the VM under a new key.
	err = withVmWorldTx(c, statePath, sessionIdx, spaceID, true, func(tx world.WorldState) error {
		if err := requireNewObject(c.Context, tx, vmKey, "VM"); err != nil {
			return err
		}
		_, _, err := tx.ApplyWorldOp(c.Context, op, "")
		return errors.Wrap(err, "create v86 VM")
	})
	if err != nil {
		return "", err
	}
	return vmKey, nil
}

func createWasiVm(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	name string,
	args *wasiVmCreateArgs,
) (string, error) {
	// Default the VM key and the key of a root filesystem imported from a tar.
	vmKey := args.objectKey
	if vmKey == "" {
		vmKey = "vm/wasivm/" + ulid.NewULID()
	}
	rootfsKey := args.rootfsObjectKey
	if args.rootfsTarPath != "" {
		rootfsKey = vmKey + "/rootfs"
	}

	// Parse the environment and build the create operation.
	env := make(map[string]string, len(args.env.Value()))
	for _, kv := range args.env.Value() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return "", errors.Errorf("invalid --env %q, want KEY=VALUE", kv)
		}
		env[k] = v
	}
	ts := time.Now()
	op := s4wave_vm.NewCreateWasiVmOp(vmKey, name, rootfsKey, args.imageObjectKey, ts)
	op.Config = &s4wave_vm.WasiVmConfig{
		InitPath: args.initPath,
		InitArgs: args.initArgs.Value(),
		Env:      env,
	}

	// Import the root filesystem when it comes from a tar, then create the VM.
	ctx := c.Context
	err := withVmWorldTx(c, statePath, sessionIdx, spaceID, true, func(tx world.WorldState) error {
		if err := requireNewObject(ctx, tx, vmKey, "VM"); err != nil {
			return err
		}
		if args.rootfsTarPath != "" {
			if err := requireNewObject(ctx, tx, rootfsKey, "root filesystem"); err != nil {
				return err
			}
			if _, err := importRootfsTar(ctx, tx, rootfsKey, args.rootfsTarPath, ts); err != nil {
				return err
			}
		}
		_, _, err := tx.ApplyWorldOp(ctx, op, "")
		return errors.Wrap(err, "create WasiVM")
	})
	if err != nil {
		return "", err
	}
	return vmKey, nil
}

// requireNewObject rejects a destination key that already holds an object.
func requireNewObject(ctx context.Context, tx world.WorldState, key, what string) error {
	// Probe the destination key.
	objectState, found, err := tx.GetObject(ctx, key)
	world.ReleaseObjectState(objectState)
	if err != nil {
		return errors.Wrapf(err, "probe destination %s", what)
	}
	if found {
		return errors.Errorf("destination %s %q already exists", what, key)
	}
	return nil
}

// setVMState requests a run state for the VM vmKey with the state operation of
// its VM type. A start first approves the VM's process binding, which has the
// Space run the VM; the approval outlasts a stop, so the runner commits it.
func setVMState(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	vmKey string,
	state s4wave_vm.VmState,
) error {
	return withVmSpace(c, statePath, sessionIdx, spaceID, func(
		client *sdkClient,
		spaceSvc s4wave_space.SRPCSpaceResourceServiceClient,
		engine world.Engine,
	) error {
		// Choose the state operation of the VM type.
		ctx := c.Context
		typeID, err := vmTypeID(ctx, world.NewEngineWorldState(engine, false), vmKey)
		if err != nil {
			return err
		}
		var op world.Operation = s4wave_vm.NewSetWasiVmStateOp(vmKey, state)
		if typeID == s4wave_vm.VmV86TypeID {
			op = s4wave_vm.NewSetV86StateOp(vmKey, state, "")
		}

		// Approve the process binding before a start.
		if state == s4wave_vm.VmState_VmState_RUNNING || state == s4wave_vm.VmState_VmState_STARTING {
			contents, releaseContents, err := client.mountSpaceContents(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer releaseContents()
			if _, err := contents.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
				ObjectKey: vmKey,
				TypeId:    typeID,
				Approved:  true,
			}); err != nil {
				return errors.Wrap(err, "approve VM process binding")
			}
		}

		// Apply the state change.
		err = world.ExecTransaction(ctx, engine, true, func(ctx context.Context, tx world.WorldState) error {
			_, _, err := tx.ApplyWorldOp(ctx, op, "")
			return err
		})
		return errors.Wrap(err, "apply "+op.TypeID())
	})
}

// vmTypeID returns the type of the VM object vmKey.
func vmTypeID(ctx context.Context, ws world.WorldState, vmKey string) (string, error) {
	typeID, err := world_types.GetObjectType(ctx, ws, vmKey)
	if err != nil {
		return "", errors.Wrap(err, "get VM type")
	}
	switch typeID {
	case s4wave_vm.VmV86TypeID, s4wave_vm.WasiVmTypeID:
		return typeID, nil
	case "":
		return "", errors.Errorf("VM %q not found", vmKey)
	default:
		return "", errors.Errorf("%q is a %s object, not a VM", vmKey, typeID)
	}
}

// vmRunState is the run state every VM block carries.
type vmRunState interface {
	// GetState returns the desired state.
	GetState() s4wave_vm.VmState
	// GetObservedState returns the state the runner last committed.
	GetObservedState() s4wave_vm.VmState
	// GetErrorMessage returns why the last run failed.
	GetErrorMessage() string
}

// readVMRunState reads the run state of a VM object of type typeID.
func readVMRunState(ctx context.Context, obj world.ObjectState, typeID string) (vmRunState, error) {
	var vm vmRunState
	_, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		if typeID == s4wave_vm.VmV86TypeID {
			v86, err := block.UnmarshalBlock[s4wave_vm.VmV86](ctx, bcs)
			if v86 != nil {
				vm = v86
			}
			return err
		}
		wasiVm, err := block.UnmarshalBlock[s4wave_vm.WasiVm](ctx, bcs)
		if wasiVm != nil {
			vm = wasiVm
		}
		return err
	})
	if err == nil && vm == nil {
		err = errors.New("VM block missing")
	}
	return vm, err
}

// openVM opens the VM object vmKey and returns it with its type. The caller
// releases the object.
func openVM(ctx context.Context, engine world.Engine, vmKey string) (world.ObjectState, string, error) {
	// Check the object's type, then open it.
	ws := world.NewEngineWorldState(engine, false)
	typeID, err := vmTypeID(ctx, ws, vmKey)
	if err != nil {
		return nil, "", err
	}
	obj, found, err := ws.GetObject(ctx, vmKey)
	if err == nil && !found {
		err = errors.Errorf("VM %q not found", vmKey)
	}
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, "", err
	}
	return obj, typeID, nil
}

// requireVMStartRequested fails unless the VM vmKey is running or starting.
// A console waits for its VM to start, so a stopped VM would wait silently.
func requireVMStartRequested(c *cli.Context, statePath string, sessionIdx uint32, spaceID, vmKey string) error {
	return withVmSpace(c, statePath, sessionIdx, spaceID, func(
		_ *sdkClient,
		_ s4wave_space.SRPCSpaceResourceServiceClient,
		engine world.Engine,
	) error {
		// Open the VM object.
		ctx := c.Context
		obj, typeID, err := openVM(ctx, engine, vmKey)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)

		// Require a desired running state.
		vm, err := readVMRunState(ctx, obj, typeID)
		if err != nil {
			return err
		}
		if vm.GetState() != s4wave_vm.VmState_VmState_RUNNING {
			return errors.Errorf("VM %q is %s; start it with: spacewave vm start %s", vmKey, vmStateLabel(vm.GetObservedState()), vmKey)
		}
		return nil
	})
}

// watchVM prints each observed state of the VM vmKey until it reaches the
// desired state or fails.
func watchVM(c *cli.Context, statePath string, sessionIdx uint32, spaceID, vmKey string) error {
	return withVmSpace(c, statePath, sessionIdx, spaceID, func(
		_ *sdkClient,
		_ s4wave_space.SRPCSpaceResourceServiceClient,
		engine world.Engine,
	) error {
		// Open the VM object.
		ctx := c.Context
		obj, typeID, err := openVM(ctx, engine, vmKey)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)

		// Print each new observed state until it settles.
		last := s4wave_vm.VmState(-1)
		for {
			_, rev, err := obj.GetRootRef(ctx)
			if err != nil {
				return err
			}
			vm, err := readVMRunState(ctx, obj, typeID)
			if err != nil {
				return err
			}
			observed := vm.GetObservedState()
			if observed != last {
				last = observed
				os.Stdout.WriteString(vmStateLabel(observed) + "\n")
			}
			switch {
			case observed == s4wave_vm.VmState_VmState_ERROR:
				return errors.Errorf("VM failed: %s", vm.GetErrorMessage())
			case observed == vm.GetState():
				return nil
			}
			if _, err := obj.WaitRev(ctx, rev+1, false); err != nil {
				return err
			}
		}
	})
}

func parseV86MountFlags(vals []string) ([]*s4wave_vm.VmMount, error) {
	out := make([]*s4wave_vm.VmMount, 0, len(vals))
	for _, val := range vals {
		left, right, ok := strings.Cut(val, "=")
		if !ok || left == "" || right == "" {
			return nil, errors.Errorf("invalid mount %q, expected /guest/path=objectKey[:rw|:ro]", val)
		}
		objectKey := right
		writable := false
		if base, mode, ok := strings.Cut(right, ":"); ok {
			objectKey = base
			switch mode {
			case "rw":
				writable = true
			case "ro":
				writable = false
			default:
				return nil, errors.Errorf("invalid mount mode %q in %q", mode, val)
			}
		}
		if objectKey == "" {
			return nil, errors.Errorf("invalid mount %q, object key is empty", val)
		}
		out = append(out, &s4wave_vm.VmMount{
			Path:      left,
			ObjectKey: objectKey,
			Writable:  writable,
		})
	}
	return out, nil
}

func importV86ImageTar(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	args *v86ImageImportTarArgs,
) (string, error) {
	// Validate the import and default the image object key.
	if err := validateV86ImageImportTarArgs(args); err != nil {
		return "", err
	}
	dstKey := args.objectKey
	if dstKey == "" {
		dstKey = "v86image-" + ulid.NewULID()
	}

	// Import the assets and create the image under a new key.
	err := withVmWorldTx(c, statePath, sessionIdx, spaceID, true, func(tx world.WorldState) error {
		if err := requireNewObject(c.Context, tx, dstKey, "image"); err != nil {
			return err
		}
		return importV86ImageAssets(c.Context, tx, dstKey, args)
	})
	if err != nil {
		return "", err
	}
	return dstKey, nil
}

// importV86ImageAssets imports the image files and rootfs tar of args and
// creates the image dstKey linked to them.
func importV86ImageAssets(ctx context.Context, tx world.WorldState, dstKey string, args *v86ImageImportTarArgs) error {
	// Import the wasm, BIOS, and kernel files.
	assetPaths := []struct {
		name string
		path string
		pred quad.IRI
	}{
		{"v86.wasm", args.wasmPath, s4wave_vm.PredV86ImageWasm},
		{"seabios.bin", args.seabiosPath, s4wave_vm.PredV86ImageBiosSeabios},
		{"vgabios.bin", args.vgabiosPath, s4wave_vm.PredV86ImageBiosVgabios},
		{"bzImage", args.kernelPath, s4wave_vm.PredV86ImageKernel},
	}
	ts := time.Now()
	edges := make(map[string]string, len(assetPaths)+1)
	for _, asset := range assetPaths {
		key, err := v86ImageImportAssetObjectKey(dstKey, string(asset.pred))
		if err != nil {
			return err
		}
		key, err = importV86SingleFile(ctx, tx, key, asset.name, asset.path, ts)
		if err != nil {
			return err
		}
		edges[string(asset.pred)] = key
	}

	// Import the rootfs tar.
	rootfsKey, err := v86ImageImportAssetObjectKey(dstKey, string(s4wave_vm.PredV86ImageRootfs))
	if err != nil {
		return err
	}
	rootfsKey, err = importRootfsTar(ctx, tx, rootfsKey, args.rootfsTarPath, ts)
	if err != nil {
		return err
	}
	edges[string(s4wave_vm.PredV86ImageRootfs)] = rootfsKey

	// Create the image and set its asset edges.
	img := &s4wave_vm.V86Image{
		Name:          args.name,
		Version:       args.version,
		Platform:      "v86",
		Distro:        args.distro,
		KernelVersion: args.kernelVersion,
		Description:   args.description,
		Tags:          args.tags.Value(),
	}
	op := s4wave_vm.NewCreateV86ImageOp(dstKey, img, ts)
	if _, _, err := tx.ApplyWorldOp(ctx, op, ""); err != nil {
		return errors.Wrap(err, "create v86 image")
	}
	for pred, target := range edges {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(dstKey, pred, target, "")); err != nil {
			return errors.Wrapf(err, "set %s edge", pred)
		}
	}
	return nil
}

func v86ImageImportAssetObjectKey(imageKey, pred string) (string, error) {
	if imageKey == "" {
		return "", errors.New("image object key is required")
	}
	switch pred {
	case string(s4wave_vm.PredV86ImageWasm):
		return imageKey + "/wasm", nil
	case string(s4wave_vm.PredV86ImageBiosSeabios):
		return imageKey + "/bios/seabios", nil
	case string(s4wave_vm.PredV86ImageBiosVgabios):
		return imageKey + "/bios/vgabios", nil
	case string(s4wave_vm.PredV86ImageKernel):
		return imageKey + "/kernel", nil
	case string(s4wave_vm.PredV86ImageRootfs):
		return imageKey + "/rootfs", nil
	default:
		return "", errors.Errorf("unsupported v86 image asset predicate %q", pred)
	}
}

func validateV86ImageImportTarArgs(args *v86ImageImportTarArgs) error {
	// Require the wasm, BIOS, and kernel paths to be files.
	for _, path := range []string{args.wasmPath, args.seabiosPath, args.vgabiosPath, args.kernelPath} {
		st, err := os.Stat(path)
		if err != nil {
			return errors.Wrapf(err, "stat %s", path)
		}
		if st.IsDir() {
			return errors.Errorf("%s is a directory, expected file", path)
		}
	}

	// Open the rootfs tar and require it to be a file.
	f, err := os.Open(args.rootfsTarPath)
	if err != nil {
		return errors.Wrapf(err, "open rootfs tar %s", args.rootfsTarPath)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return errors.Wrapf(err, "stat rootfs tar %s", args.rootfsTarPath)
	}
	if st.IsDir() {
		return errors.Errorf("%s is a directory, expected tar file", args.rootfsTarPath)
	}

	// Parse the rootfs tar and release the cursor.
	tarCursor, err := unixfs_tar.NewTarFSCursor(f, st.Size())
	if err != nil {
		return errors.Wrapf(err, "parse rootfs tar %s", args.rootfsTarPath)
	}
	tarCursor.Release()
	return nil
}

func importV86SingleFile(ctx context.Context, tx world.WorldState, key, name, path string, ts time.Time) (string, error) {
	// Open the asset file.
	st, err := os.Stat(path)
	if err != nil {
		return "", errors.Wrapf(err, "stat %s", path)
	}
	if st.IsDir() {
		return "", errors.Errorf("%s is a directory, expected file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "open %s", path)
	}
	defer f.Close()

	// Initialize the UnixFS node and write the file contents.
	if _, _, err := unixfs_world.FsInit(ctx, tx, "", key, unixfs_world.FSType_FSType_FS_NODE, nil, false, ts); err != nil {
		return "", errors.Wrap(err, "fs-init "+name)
	}
	obj, err := world.MustGetObject(ctx, tx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return "", err
	}
	if _, _, err := unixfs_world.FsMknodWithContent(
		ctx,
		tx,
		obj,
		"",
		unixfs_world.FSType_FSType_FS_NODE,
		[]string{name},
		unixfs.NewFSCursorNodeType_File(),
		st.Size(),
		f,
		fs.FileMode(0o644),
		ts,
	); err != nil {
		return "", errors.Wrap(err, "write "+name)
	}
	return key, nil
}

func importRootfsTar(ctx context.Context, tx world.WorldState, key, path string, ts time.Time) (string, error) {
	// Open the rootfs tar and require it to be a file.
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "open rootfs tar %s", path)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", errors.Wrapf(err, "stat rootfs tar %s", path)
	}
	if st.IsDir() {
		return "", errors.Errorf("%s is a directory, expected tar file", path)
	}

	// Parse the tar and open it as a filesystem handle.
	tarCursor, err := unixfs_tar.NewTarFSCursor(f, st.Size())
	if err != nil {
		return "", errors.Wrapf(err, "parse rootfs tar %s", path)
	}
	defer tarCursor.Release()
	srcHandle, err := unixfs.NewFSHandle(tarCursor)
	if err != nil {
		return "", errors.Wrap(err, "build tar fs handle")
	}
	defer srcHandle.Release()

	// Initialize the rootfs node and sync the tar into it.
	if _, _, err := unixfs_world.FsInit(ctx, tx, "", key, unixfs_world.FSType_FSType_FS_NODE, nil, false, ts); err != nil {
		return "", errors.Wrap(err, "fs-init rootfs")
	}
	b := unixfs_world.NewBatchFSWriter(tx, key, unixfs_world.FSType_FSType_FS_NODE, "")
	if err := unixfs_sync.SyncToUnixfsBatch(ctx, b, srcHandle, nil); err != nil {
		b.Release()
		return "", errors.Wrap(err, "sync rootfs tar")
	}
	return key, nil
}

func readV86Image(c *cli.Context, statePath string, sessionIdx uint32, spaceID, key string) (*v86ImageCLIEntry, error) {
	var out *v86ImageCLIEntry
	err := withVmWorldTx(c, statePath, sessionIdx, spaceID, false, func(tx world.WorldState) error {
		img, err := readV86ImageFromTx(c, tx, key)
		if err != nil {
			return err
		}
		out = img
		return nil
	})
	return out, err
}

// withVmSpace runs cb with the selected Space and its World engine.
func withVmSpace(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	cb func(client *sdkClient, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient, engine world.Engine) error,
) error {
	// Connect to the daemon.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the selected session.
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	// Resolve the Space ID.
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}

	// Mount the Space service.
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return err
	}
	defer spaceCleanup()

	// Open the World engine and run the callback.
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()
	return cb(client, spaceSvc, engine)
}

// withVmWorldTx runs cb in a transaction on the World of the selected Space.
// A write transaction commits when cb succeeds.
func withVmWorldTx(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	write bool,
	cb func(world.WorldState) error,
) error {
	return withVmSpace(c, statePath, sessionIdx, spaceID, func(
		_ *sdkClient,
		_ s4wave_space.SRPCSpaceResourceServiceClient,
		engine world.Engine,
	) error {
		return world.ExecTransaction(c.Context, engine, write, func(_ context.Context, tx world.WorldState) error {
			return cb(tx)
		})
	})
}

func readV86ImageFromTx(c *cli.Context, tx world.WorldState, key string) (*v86ImageCLIEntry, error) {
	// Load the image object and require that it exists.
	ctx := c.Context
	obj, found, err := tx.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, errors.Wrap(err, "get v86 image")
	}
	if !found {
		return nil, errors.Errorf("v86 image %q not found", key)
	}

	// Decode the image block and read its asset edges.
	var img *s4wave_vm.V86Image
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		current, err := block.UnmarshalBlock[s4wave_vm.V86Image](ctx, bcs)
		if err != nil {
			return err
		}
		img = current.CloneVT()
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "read v86 image block")
	}
	assets, err := readV86ImageAssets(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	return &v86ImageCLIEntry{objectKey: key, image: img, assets: assets}, nil
}

func readV86ImageAssets(ctx context.Context, tx world.WorldState, key string) (map[string]string, error) {
	out := make(map[string]string, len(v86ImageAssetPreds))
	for name, pred := range v86ImageAssetPreds {
		quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, string(pred), "", ""), 1)
		if err != nil {
			return nil, errors.Wrapf(err, "lookup %s edge", name)
		}
		if len(quads) == 0 {
			out[name] = ""
			continue
		}
		target, err := world.GraphValueToKey(quads[0].GetObj())
		if err != nil {
			return nil, errors.Wrapf(err, "parse %s edge", name)
		}
		out[name] = target
	}
	return out, nil
}

var v86ImageAssetPreds = map[string]quad.IRI{
	"wasm":    s4wave_vm.PredV86ImageWasm,
	"seabios": s4wave_vm.PredV86ImageBiosSeabios,
	"vgabios": s4wave_vm.PredV86ImageBiosVgabios,
	"kernel":  s4wave_vm.PredV86ImageKernel,
	"rootfs":  s4wave_vm.PredV86ImageRootfs,
}

func writeV86ImageList(images []*v86ImageCLIEntry, outputFormat string) error {
	// Emit the image list as JSON or YAML, or write the table.
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("images")
		ms.WriteArrayStart()
		var imf bool
		for _, img := range images {
			ms.WriteMoreIf(&imf)
			writeV86ImageJSON(ms, img)
		}
		ms.WriteArrayEnd()
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	rows := [][]string{{"KEY", "NAME", "VERSION", "DISTRO", "KERNEL", "TAGS"}}
	for _, img := range images {
		rows = append(rows, []string{
			img.objectKey,
			img.image.GetName(),
			img.image.GetVersion(),
			img.image.GetDistro(),
			img.image.GetKernelVersion(),
			strings.Join(img.image.GetTags(), ","),
		})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

func writeV86ImageInfo(img *v86ImageCLIEntry, outputFormat string) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		writeV86ImageJSON(ms, img)
		return formatOutput(buf.Bytes(), outputFormat)
	}
	fields := [][2]string{
		{"Key", img.objectKey},
		{"Name", img.image.GetName()},
		{"Version", img.image.GetVersion()},
		{"Platform", img.image.GetPlatform()},
		{"Distro", img.image.GetDistro()},
		{"Kernel", img.image.GetKernelVersion()},
		{"Tags", strings.Join(img.image.GetTags(), ",")},
		{"WASM", img.assets["wasm"]},
		{"SeaBIOS", img.assets["seabios"]},
		{"VGABIOS", img.assets["vgabios"]},
		{"Kernel Asset", img.assets["kernel"]},
		{"Rootfs", img.assets["rootfs"]},
	}
	writeFields(os.Stdout, fields)
	return nil
}

func writeV86ImageJSON(ms *protojson.MarshalState, img *v86ImageCLIEntry) {
	// Write the image identity fields.
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "objectKey", img.objectKey)
	writeJSONStringField(ms, &f, "name", img.image.GetName())
	writeJSONStringField(ms, &f, "version", img.image.GetVersion())
	writeJSONStringField(ms, &f, "platform", img.image.GetPlatform())
	writeJSONStringField(ms, &f, "distro", img.image.GetDistro())
	writeJSONStringField(ms, &f, "kernelVersion", img.image.GetKernelVersion())

	// Write the image tags.
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("tags")
	ms.WriteArrayStart()
	var tf bool
	for _, tag := range img.image.GetTags() {
		ms.WriteMoreIf(&tf)
		ms.WriteString(tag)
	}
	ms.WriteArrayEnd()

	// Write the image assets and close the object.
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("assets")
	ms.WriteObjectStart()
	var af bool
	for _, name := range []string{"wasm", "seabios", "vgabios", "kernel", "rootfs"} {
		writeJSONStringField(ms, &af, name, img.assets[name])
	}
	ms.WriteObjectEnd()
	ms.WriteObjectEnd()
}
