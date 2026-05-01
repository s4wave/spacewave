//go:build !js

package spacewave_cli

import (
	"context"
	"io/fs"
	"os"
	"sort"
	"strconv"
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
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// newVmCommand builds the vm command group.
func newVmCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	var spaceID string
	flags := append(clientFlags(&statePath, &sessionIdx), vmSpaceFlag(&spaceID))
	return &cli.Command{
		Name:  "vm",
		Usage: "manage virtual machines",
		Flags: flags,
		Subcommands: []*cli.Command{
			newVmListCommand(&statePath, &sessionIdx, &spaceID),
			newVmInfoCommand(&statePath, &sessionIdx, &spaceID),
			newVmCreateCommand(&statePath, &sessionIdx, &spaceID),
			newVmStartCommand(&statePath, &sessionIdx, &spaceID),
			newVmStopCommand(&statePath, &sessionIdx, &spaceID),
			newVmWatchCommand(&statePath, &sessionIdx, &spaceID),
			newVmImageCommand(&statePath, &sessionIdx, &spaceID),
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
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			if err := setV86VMState(c, *statePath, uint32(*sessionIdx), *spaceID, key, s4wave_vm.VmState_VmState_STARTING); err != nil {
				return err
			}
			if !wait {
				os.Stdout.WriteString("starting\n")
				return nil
			}
			return watchV86VM(c, *statePath, uint32(*sessionIdx), *spaceID, key)
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
			return watchV86VM(c, *statePath, uint32(*sessionIdx), *spaceID, key)
		},
	}
}

func isTerminalExecutionState(state s4wave_process.ExecutionState) bool {
	switch state {
	case s4wave_process.ExecutionState_ExecutionState_RUNNING,
		s4wave_process.ExecutionState_ExecutionState_STOPPED,
		s4wave_process.ExecutionState_ExecutionState_ERROR:
		return true
	default:
		return false
	}
}

func newVmStopCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "stop",
		Usage:     "request a VM stop",
		ArgsUsage: "<vm-key>",
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			if err := setV86VMState(c, *statePath, uint32(*sessionIdx), *spaceID, key, s4wave_vm.VmState_VmState_STOPPED); err != nil {
				return err
			}
			os.Stdout.WriteString("stopped\n")
			return nil
		},
	}
}

func newVmCreateCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create a VM",
		Subcommands: []*cli.Command{
			newVmCreateV86Command(statePath, sessionIdx, spaceID),
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
			name := c.Args().First()
			if name == "" {
				return errors.New("VM name required")
			}
			key, err := createV86VM(c, *statePath, uint32(*sessionIdx), *spaceID, name, args)
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
			vms, err := readV86VMs(c, *statePath, uint32(*sessionIdx), *spaceID)
			if err != nil {
				return err
			}
			return writeV86VMList(vms, c.String("output"))
		},
	}
}

func newVmInfoCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "show VM metadata and runtime configuration",
		ArgsUsage: "<vm-key>",
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("VM key required")
			}
			vm, err := readV86VM(c, *statePath, uint32(*sessionIdx), *spaceID, key)
			if err != nil {
				return err
			}
			return writeV86VMInfo(vm, c.String("output"))
		},
	}
}

func newVmImageCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	return &cli.Command{
		Name:  "image",
		Usage: "manage VM images",
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
			images, err := readV86Images(c, *statePath, uint32(*sessionIdx), *spaceID)
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
			key, err := importV86ImageTar(c, *statePath, uint32(*sessionIdx), *spaceID, args)
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
			srcKey := c.Args().First()
			if srcKey == "" {
				return errors.New("CDN image key required")
			}
			if dstKey == "" {
				dstKey = srcKey
			}
			if err := copyV86ImageFromCdn(c, *statePath, uint32(*sessionIdx), *spaceID, cdnID, srcKey, dstKey); err != nil {
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
			key := c.Args().First()
			if key == "" {
				return errors.New("image key required")
			}
			image, err := readV86Image(c, *statePath, uint32(*sessionIdx), *spaceID, key)
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

type v86VMCLIEntry struct {
	objectKey string
	vm        *s4wave_vm.VmV86
	edges     map[string]string
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

func readV86Images(c *cli.Context, statePath string, sessionIdx uint32, spaceID string) ([]*v86ImageCLIEntry, error) {
	var out []*v86ImageCLIEntry
	err := withVmWorldReadTx(c, statePath, sessionIdx, spaceID, func(tx world.WorldState) error {
		keys, err := world_types.ListObjectsWithType(c.Context, tx, s4wave_vm.V86ImageTypeID)
		if err != nil {
			return errors.Wrap(err, "list v86 images")
		}
		sort.Strings(keys)
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

func readV86VMs(c *cli.Context, statePath string, sessionIdx uint32, spaceID string) ([]*v86VMCLIEntry, error) {
	var out []*v86VMCLIEntry
	err := withVmWorldReadTx(c, statePath, sessionIdx, spaceID, func(tx world.WorldState) error {
		keys, err := world_types.ListObjectsWithType(c.Context, tx, s4wave_vm.VmV86TypeID)
		if err != nil {
			return errors.Wrap(err, "list VMs")
		}
		sort.Strings(keys)
		out = make([]*v86VMCLIEntry, 0, len(keys))
		for _, key := range keys {
			vm, err := readV86VMFromTx(c, tx, key)
			if err != nil {
				return err
			}
			out = append(out, vm)
		}
		return nil
	})
	return out, err
}

func readV86VM(c *cli.Context, statePath string, sessionIdx uint32, spaceID, key string) (*v86VMCLIEntry, error) {
	var out *v86VMCLIEntry
	err := withVmWorldReadTx(c, statePath, sessionIdx, spaceID, func(tx world.WorldState) error {
		vm, err := readV86VMFromTx(c, tx, key)
		if err != nil {
			return err
		}
		out = vm
		return nil
	})
	return out, err
}

func readV86VMFromTx(c *cli.Context, tx world.WorldState, key string) (*v86VMCLIEntry, error) {
	ctx := c.Context
	obj, found, err := tx.GetObject(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "get VM")
	}
	if !found {
		return nil, errors.Errorf("VM %q not found", key)
	}
	var vm *s4wave_vm.VmV86
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		current, err := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if err != nil {
			return err
		}
		vm = current.CloneVT()
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "read VM block")
	}
	edges, err := readV86VMEdges(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	return &v86VMCLIEntry{objectKey: key, vm: vm, edges: edges}, nil
}

func readV86VMEdges(ctx context.Context, tx world.WorldState, key string) (map[string]string, error) {
	preds := map[string]quad.IRI{
		"image":          s4wave_vm.PredV86Image,
		"kernelOverride": s4wave_vm.PredV86KernelOverride,
		"rootfsOverride": s4wave_vm.PredV86RootfsOverride,
		"biosOverride":   s4wave_vm.PredV86BiosOverride,
		"wasmOverride":   s4wave_vm.PredV86WasmOverride,
	}
	out := make(map[string]string, len(preds))
	for name, pred := range preds {
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

func copyV86ImageFromCdn(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	cdnID string,
	srcKey string,
	dstKey string,
) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}

	resp, err := client.root.GetCdn(ctx, cdnID)
	if err != nil {
		return errors.Wrap(err, "get CDN")
	}
	cdnRef := client.resClient.CreateResourceReference(resp.GetResourceId())
	defer cdnRef.Release()
	cdnClient, err := cdnRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "CDN client")
	}
	cdnSvc := s4wave_cdn.NewSRPCCdnResourceServiceClient(cdnClient)
	_, err = cdnSvc.CopyV86ImageToSpace(ctx, &s4wave_cdn.CopyV86ImageToSpaceRequest{
		SessionIdx:   sessionIdx,
		DstSpaceId:   sid,
		SrcObjectKey: srcKey,
		DstObjectKey: dstKey,
	})
	if err != nil {
		return errors.Wrap(err, "copy v86 image from CDN")
	}
	return nil
}

func createV86VM(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	name string,
	args *v86VMCreateArgs,
) (string, error) {
	mounts, err := parseV86MountFlags(args.mounts.Value())
	if err != nil {
		return "", err
	}
	vmKey := args.objectKey
	if vmKey == "" {
		vmKey = "vm/v86/" + ulid.NewULID()
	}
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return "", err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return "", err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return "", err
	}
	defer spaceCleanup()
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return "", err
	}
	defer engineCleanup()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return "", errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	if _, found, err := tx.GetObject(ctx, vmKey); err != nil {
		return "", errors.Wrap(err, "probe destination VM")
	} else if found {
		return "", errors.Errorf("destination VM %q already exists", vmKey)
	}
	op := s4wave_vm.NewCreateVmV86Op(vmKey, name, args.imageObjectKey, time.Now())
	op.Config = &s4wave_vm.V86Config{
		MemoryMb:        uint32(args.memoryMb),
		VgaMemoryMb:     uint32(args.vgaMemoryMb),
		Networking:      args.networking,
		SerialEnabled:   args.serialEnabled,
		BootArgs:        args.bootArgs,
		RuntimePluginId: args.runtimePluginID,
		Mounts:          mounts,
	}
	if _, _, err := tx.ApplyWorldOp(ctx, op, ""); err != nil {
		return "", errors.Wrap(err, "create v86 VM")
	}
	if err := tx.Commit(ctx); err != nil {
		return "", errors.Wrap(err, "commit create VM")
	}
	return vmKey, nil
}

func setV86VMState(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	vmKey string,
	state s4wave_vm.VmState,
) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return err
	}
	defer spaceCleanup()
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()
	op := s4wave_vm.NewSetV86StateOp(vmKey, state, "")
	return applyWorldOp(c, engine, op)
}

func watchV86VM(c *cli.Context, statePath string, sessionIdx uint32, spaceID, vmKey string) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return err
	}
	defer spaceCleanup()
	_, engineRef, engineCleanup, err := client.accessWorldEngineWithRef(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()
	vmClient, _, _, vmCleanup, err := client.accessTypedObject(ctx, engineRef, vmKey)
	if err != nil {
		return err
	}
	defer vmCleanup()
	execSvc := s4wave_process.NewSRPCPersistentExecutionServiceClient(vmClient)
	stream, err := execSvc.Execute(ctx, &s4wave_process.ExecuteRequest{})
	if err != nil {
		return errors.Wrap(err, "execute VM")
	}
	defer stream.Close()
	for {
		status, err := stream.Recv()
		if err != nil {
			return errors.Wrap(err, "recv VM status")
		}
		state := status.GetState()
		os.Stdout.WriteString(state.String() + "\n")
		if isTerminalExecutionState(state) {
			if state == s4wave_process.ExecutionState_ExecutionState_ERROR {
				return errors.New("VM entered ERROR")
			}
			return nil
		}
	}
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
	ctx := c.Context
	if err := validateV86ImageImportTarArgs(args); err != nil {
		return "", err
	}
	dstKey := args.objectKey
	if dstKey == "" {
		dstKey = "v86image-" + ulid.NewULID()
	}
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
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return "", err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return "", err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return "", err
	}
	defer spaceCleanup()
	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return "", err
	}
	defer engineCleanup()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return "", errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	if _, found, err := tx.GetObject(ctx, dstKey); err != nil {
		return "", errors.Wrap(err, "probe destination")
	} else if found {
		return "", errors.Errorf("destination image %q already exists", dstKey)
	}
	ts := time.Now()
	edges := make(map[string]string, 5)
	for _, asset := range assetPaths {
		key, err := importV86SingleFile(ctx, tx, asset.name, asset.path, ts)
		if err != nil {
			return "", err
		}
		edges[string(asset.pred)] = key
	}
	rootfsKey, err := importV86RootfsTar(ctx, tx, args.rootfsTarPath, ts)
	if err != nil {
		return "", err
	}
	edges[string(s4wave_vm.PredV86ImageRootfs)] = rootfsKey
	tags := args.tags.Value()
	img := &s4wave_vm.V86Image{
		Name:          args.name,
		Version:       args.version,
		Platform:      "v86",
		Distro:        args.distro,
		KernelVersion: args.kernelVersion,
		Description:   args.description,
		Tags:          tags,
	}
	op := s4wave_vm.NewCreateV86ImageOp(dstKey, img, ts)
	if _, _, err := tx.ApplyWorldOp(ctx, op, ""); err != nil {
		return "", errors.Wrap(err, "create v86 image")
	}
	for pred, target := range edges {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(dstKey, pred, target, "")); err != nil {
			return "", errors.Wrapf(err, "set %s edge", pred)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", errors.Wrap(err, "commit import")
	}
	return dstKey, nil
}

func validateV86ImageImportTarArgs(args *v86ImageImportTarArgs) error {
	for _, path := range []string{args.wasmPath, args.seabiosPath, args.vgabiosPath, args.kernelPath} {
		st, err := os.Stat(path)
		if err != nil {
			return errors.Wrapf(err, "stat %s", path)
		}
		if st.IsDir() {
			return errors.Errorf("%s is a directory, expected file", path)
		}
	}
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
	tarCursor, err := unixfs_tar.NewTarFSCursor(f, st.Size())
	if err != nil {
		return errors.Wrapf(err, "parse rootfs tar %s", args.rootfsTarPath)
	}
	tarCursor.Release()
	return nil
}

func importV86SingleFile(ctx context.Context, tx world.WorldState, name, path string, ts time.Time) (string, error) {
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
	key := "v86image-asset-" + ulid.NewULID()
	if _, _, err := unixfs_world.FsInit(ctx, tx, "", key, unixfs_world.FSType_FSType_FS_NODE, nil, false, ts); err != nil {
		return "", errors.Wrap(err, "fs-init "+name)
	}
	obj, err := world.MustGetObject(ctx, tx, key)
	if err != nil {
		return "", err
	}
	if _, _, err := unixfs_world.FsMknodWithContent(
		ctx,
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

func importV86RootfsTar(ctx context.Context, tx world.WorldState, path string, ts time.Time) (string, error) {
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
	key := "v86image-asset-" + ulid.NewULID()
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
	err := withVmWorldReadTx(c, statePath, sessionIdx, spaceID, func(tx world.WorldState) error {
		img, err := readV86ImageFromTx(c, tx, key)
		if err != nil {
			return err
		}
		out = img
		return nil
	})
	return out, err
}

func withVmWorldReadTx(
	c *cli.Context,
	statePath string,
	sessionIdx uint32,
	spaceID string,
	cb func(world.WorldState) error,
) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	sess, err := client.mountSession(ctx, sessionIdx)
	if err != nil {
		return err
	}
	defer sess.Release()

	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		return err
	}

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		return err
	}
	defer spaceCleanup()

	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		return err
	}
	defer engineCleanup()

	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	return cb(tx)
}

func readV86ImageFromTx(c *cli.Context, tx world.WorldState, key string) (*v86ImageCLIEntry, error) {
	ctx := c.Context
	obj, found, err := tx.GetObject(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "get v86 image")
	}
	if !found {
		return nil, errors.Errorf("v86 image %q not found", key)
	}
	var img *s4wave_vm.V86Image
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		current, err := block.UnmarshalBlock[*s4wave_vm.V86Image](ctx, bcs, func() block.Block {
			return &s4wave_vm.V86Image{}
		})
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

func writeV86VMList(vms []*v86VMCLIEntry, outputFormat string) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("vms")
		ms.WriteArrayStart()
		var vf bool
		for _, vm := range vms {
			ms.WriteMoreIf(&vf)
			writeV86VMJSON(ms, vm)
		}
		ms.WriteArrayEnd()
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	rows := [][]string{{"KEY", "NAME", "STATE", "IMAGE", "MEMORY", "VGA"}}
	for _, vm := range vms {
		cfg := vm.vm.GetConfig()
		rows = append(rows, []string{
			vm.objectKey,
			vm.vm.GetName(),
			vm.vm.GetState().String(),
			vm.edges["image"],
			strconv.Itoa(int(cfg.GetMemoryMb())),
			strconv.Itoa(int(cfg.GetVgaMemoryMb())),
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

func writeV86VMInfo(vm *v86VMCLIEntry, outputFormat string) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		writeV86VMJSON(ms, vm)
		return formatOutput(buf.Bytes(), outputFormat)
	}
	cfg := vm.vm.GetConfig()
	fields := [][2]string{
		{"Key", vm.objectKey},
		{"Name", vm.vm.GetName()},
		{"State", vm.vm.GetState().String()},
		{"Image", vm.edges["image"]},
		{"Memory MB", strconv.Itoa(int(cfg.GetMemoryMb()))},
		{"VGA Memory MB", strconv.Itoa(int(cfg.GetVgaMemoryMb()))},
		{"Networking", strconv.FormatBool(cfg.GetNetworking())},
		{"Serial", strconv.FormatBool(cfg.GetSerialEnabled())},
		{"Boot Args", cfg.GetBootArgs()},
		{"Runtime Plugin", cfg.GetRuntimePluginId()},
		{"Kernel Override", vm.edges["kernelOverride"]},
		{"Rootfs Override", vm.edges["rootfsOverride"]},
		{"BIOS Override", vm.edges["biosOverride"]},
		{"WASM Override", vm.edges["wasmOverride"]},
		{"Error", vm.vm.GetErrorMessage()},
	}
	writeFields(os.Stdout, fields)
	return nil
}

func writeV86ImageJSON(ms *protojson.MarshalState, img *v86ImageCLIEntry) {
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "objectKey", img.objectKey)
	writeJSONStringField(ms, &f, "name", img.image.GetName())
	writeJSONStringField(ms, &f, "version", img.image.GetVersion())
	writeJSONStringField(ms, &f, "platform", img.image.GetPlatform())
	writeJSONStringField(ms, &f, "distro", img.image.GetDistro())
	writeJSONStringField(ms, &f, "kernelVersion", img.image.GetKernelVersion())
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("tags")
	ms.WriteArrayStart()
	var tf bool
	for _, tag := range img.image.GetTags() {
		ms.WriteMoreIf(&tf)
		ms.WriteString(tag)
	}
	ms.WriteArrayEnd()
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

func writeV86VMJSON(ms *protojson.MarshalState, vm *v86VMCLIEntry) {
	cfg := vm.vm.GetConfig()
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "objectKey", vm.objectKey)
	writeJSONStringField(ms, &f, "name", vm.vm.GetName())
	writeJSONStringField(ms, &f, "state", vm.vm.GetState().String())
	writeJSONStringField(ms, &f, "imageObjectKey", vm.edges["image"])
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("config")
	ms.WriteObjectStart()
	var cf bool
	writeJSONUint64Field(ms, &cf, "memoryMb", uint64(cfg.GetMemoryMb()))
	writeJSONUint64Field(ms, &cf, "vgaMemoryMb", uint64(cfg.GetVgaMemoryMb()))
	writeJSONBoolField(ms, &cf, "networking", cfg.GetNetworking())
	writeJSONBoolField(ms, &cf, "serialEnabled", cfg.GetSerialEnabled())
	writeJSONStringField(ms, &cf, "bootArgs", cfg.GetBootArgs())
	writeJSONStringField(ms, &cf, "runtimePluginId", cfg.GetRuntimePluginId())
	ms.WriteObjectEnd()
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("overrides")
	ms.WriteObjectStart()
	var of bool
	writeJSONStringField(ms, &of, "kernel", vm.edges["kernelOverride"])
	writeJSONStringField(ms, &of, "rootfs", vm.edges["rootfsOverride"])
	writeJSONStringField(ms, &of, "bios", vm.edges["biosOverride"])
	writeJSONStringField(ms, &of, "wasm", vm.edges["wasmOverride"])
	ms.WriteObjectEnd()
	writeJSONStringField(ms, &f, "errorMessage", vm.vm.GetErrorMessage())
	ms.WriteObjectEnd()
}
