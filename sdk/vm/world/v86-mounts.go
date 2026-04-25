package s4wave_vm_world

import (
	"context"
	"strings"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// homeMountPath is the guest filesystem path at which the auto-provisioned
// home mount is attached. Derived name used in the v86fs mount table follows
// deriveV86MountName (first non-empty path segment -> "home").
const homeMountPath = "/home"

// registerV86ConfigMounts reads V86Config.Mounts on the VmV86 at objectKey
// and registers each mount on the v86fs server. The guest is notified of the
// mount set via MOUNT_NOTIFY frames on session join (seeded by the server).
//
// Returns a cleanup closure that releases every opened FSHandle. Callers must
// invoke cleanup during factory shutdown so handle refcounts drop cleanly.
// A missing object or missing config is not an error; the returned cleanup is
// a no-op in that case.
func registerV86ConfigMounts(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	srv *unixfs_v86fs.Server,
) (func(), error) {
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "get vm object")
	}
	if !found {
		return func() {}, nil
	}

	var cfg *s4wave_vm.V86Config
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm != nil {
			cfg = vm.GetConfig().CloneVT()
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "read v86 config")
	}
	if cfg == nil || len(cfg.GetMounts()) == 0 {
		return func() {}, nil
	}

	var opened []*unixfs.FSHandle
	registered := make([]string, 0, len(cfg.GetMounts()))
	cleanup := func() {
		for _, name := range registered {
			srv.RemoveMount(name)
		}
		for _, h := range opened {
			h.Release()
		}
	}

	for _, mnt := range cfg.GetMounts() {
		path := mnt.GetPath()
		objKey := mnt.GetObjectKey()
		if path == "" || objKey == "" {
			continue
		}
		name := deriveV86MountName(path)
		if name == "" {
			continue
		}
		handle, err := openFSHandleForObject(ctx, ws, objKey)
		if err != nil {
			cleanup()
			return nil, errors.Wrapf(err, "open v86 mount %q -> %s", path, objKey)
		}
		opened = append(opened, handle)
		srv.AddMount(name, path, handle)
		registered = append(registered, name)
	}
	return cleanup, nil
}

// deriveV86MountName converts a guest mount path into the v86fs MOUNT name
// used to look the mount up in the server. The first non-empty path segment
// is used verbatim, so "/workspace" -> "workspace" and "/home/user" ->
// "home".
func deriveV86MountName(path string) string {
	trimmed := strings.TrimLeft(path, "/")
	if trimmed == "" {
		return ""
	}
	if before, _, ok := strings.Cut(trimmed, "/"); ok {
		return before
	}
	return trimmed
}

// ensureHomeMount guarantees the VmV86 at =vmObjectKey= carries a writable
// home mount. If the stored V86Config.Mounts list already contains an entry
// for =/home=, nothing happens. Otherwise a fresh empty UnixFS FS-node world
// object is created at a deterministic key (=<vm>-home=), registered as
// =unixfs/fs-node=, and appended to Mounts via SetV86ConfigOp so the host
// factory picks it up on the next boot and subsequent boots reuse the same
// object so writes persist across VM restarts.
//
// Intended to run once per VM start, before the plugin backend is loaded.
// Returns nil on success (including the already-provisioned case) and
// propagates any error from the backing world state.
func ensureHomeMount(
	ctx context.Context,
	ws world.WorldState,
	vmObjectKey string,
) error {
	if vmObjectKey == "" {
		return errors.New("vm object key is required")
	}

	vmObjState, found, err := ws.GetObject(ctx, vmObjectKey)
	if err != nil {
		return errors.Wrap(err, "get vm object")
	}
	if !found {
		return errors.Errorf("vm-v86 object %q not found", vmObjectKey)
	}

	var cfg *s4wave_vm.V86Config
	_, _, err = world.AccessObjectState(ctx, vmObjState, false, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm != nil {
			cfg = vm.GetConfig().CloneVT()
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "read v86 config")
	}
	if cfg == nil {
		cfg = &s4wave_vm.V86Config{}
	}
	for _, mnt := range cfg.GetMounts() {
		if mnt.GetPath() == homeMountPath {
			return nil
		}
	}

	homeObjectKey := vmObjectKey + "-home"
	if err := ensureEmptyFSNodeObject(ctx, ws, homeObjectKey); err != nil {
		return errors.Wrap(err, "provision home unixfs object")
	}

	cfg.Mounts = append(cfg.GetMounts(), &s4wave_vm.VmMount{
		Path:      homeMountPath,
		ObjectKey: homeObjectKey,
		Writable:  true,
	})

	op := s4wave_vm.NewSetV86ConfigOp(vmObjectKey, cfg)
	if _, _, err := ws.ApplyWorldOp(ctx, op, ""); err != nil {
		return errors.Wrap(err, "apply set-config op with home mount")
	}
	return nil
}

// ensureEmptyFSNodeObject creates an empty UnixFS FS-node world object at
// =objKey= if one does not already exist. Existing objects are left
// untouched so repeated calls are idempotent across VM restarts.
func ensureEmptyFSNodeObject(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) error {
	if _, found, err := ws.GetObject(ctx, objKey); err != nil {
		return errors.Wrap(err, "probe fs-node object")
	} else if found {
		return nil
	}

	ts := timestamppb.New(time.Now())
	root := unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, ts)
	if _, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(root, true)
		return nil
	}); err != nil {
		return errors.Wrap(err, "create fs-node object")
	}
	if err := world_types.SetObjectType(ctx, ws, objKey, unixfs_world.FSNodeTypeID); err != nil {
		return errors.Wrap(err, "set fs-node object type")
	}
	return nil
}
