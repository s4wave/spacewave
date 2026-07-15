//go:build !js

package resource_world_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
)

// TestVmV86TypedObject tests the VmV86 ObjectType factory, v86fs service
// registration, and graph edge mount resolution.
func TestVmV86TypedObject(t *testing.T) {
	ctx := t.Context()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	t.Run("AccessTypedObject", func(t *testing.T) {
		engine, cleanup := setupVmV86WorldEngine(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test/vm"
		rootfsKey := "vm-v86-test/rootfs"

		// Create VmV86 + rootfs objects and wire graph edges.
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		// Access the VmV86 as a typed object.
		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer readTx.Release()

		srpcClient, err := readTx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		resp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: vmKey,
		})
		if err != nil {
			t.Fatalf("AccessTypedObject failed: %v", err)
		}
		if resp.TypeId != s4wave_vm.VmV86TypeID {
			t.Fatalf("expected type %q, got %q", s4wave_vm.VmV86TypeID, resp.TypeId)
		}
		if resp.ResourceId == 0 {
			t.Fatal("expected non-zero resource ID")
		}
		t.Logf("AccessTypedObject: type=%s resourceId=%d", resp.TypeId, resp.ResourceId)
	})

	t.Run("V86fsServiceOnMux", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()
		_ = resClient

		vmKey := "vm-v86-test-v86fs/vm"
		rootfsKey := "vm-v86-test-v86fs/rootfs"

		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		// Access typed object to get resource mux.
		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer readTx.Release()

		srpcClient, err := readTx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		resp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: vmKey,
		})
		if err != nil {
			t.Fatalf("AccessTypedObject failed: %v", err)
		}

		// Create a client to the typed resource mux.
		vmRef := resClient.CreateResourceReference(resp.ResourceId)
		defer vmRef.Release()

		vmClient, err := vmRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient for vm resource failed: %v", err)
		}

		// Verify V86fsService is accessible by opening a RelayV86fs stream.
		v86fsSvc := unixfs_v86fs.NewSRPCV86FsServiceClient(vmClient)

		// Send mount requests for every image-backed asset name. This exercises
		// the full path: v86fs server -> mount resolver -> graph edge -> FSHandle.
		streamCtx, streamCancel := context.WithTimeout(ctx, 5*time.Second)
		defer streamCancel()

		stream, err := v86fsSvc.RelayV86Fs(streamCtx)
		if err != nil {
			t.Fatalf("RelayV86Fs failed: %v", err)
		}

		for tag, name := range []string{"", "rootfs", "kernel", "seabios", "vgabios", "wasm"} {
			err = stream.Send(&unixfs_v86fs.V86FsMessage{
				Tag: uint32(tag + 1),
				Body: &unixfs_v86fs.V86FsMessage_MountRequest{
					MountRequest: &unixfs_v86fs.V86FsMountRequest{
						Name: name,
					},
				},
			})
			if err != nil {
				t.Fatalf("Send MOUNT %q failed: %v", name, err)
			}

			reply, err := stream.Recv()
			if err != nil {
				t.Fatalf("Recv MOUNT %q reply failed: %v", name, err)
			}
			if reply.GetTag() != uint32(tag+1) {
				t.Fatalf("expected tag %d, got %d", tag+1, reply.GetTag())
			}

			mountReply := reply.GetMountReply()
			if mountReply == nil {
				t.Fatalf("expected MountReply for %q, got %T", name, reply.GetBody())
			}
			if mountReply.GetStatus() != 0 {
				t.Fatalf("mount %q failed with status %d", name, mountReply.GetStatus())
			}
			if mountReply.GetRootInodeId() == 0 {
				t.Fatalf("expected non-zero root inode ID for %q", name)
			}

			t.Logf("V86fs MOUNT %q succeeded: rootInodeId=%d mode=%o",
				name, mountReply.GetRootInodeId(), mountReply.GetMode())
		}

		streamCancel()
	})

	t.Run("V86ConfigMountsBecomeGuestMounts", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-config-mount/vm"
		rootfsKey := "vm-v86-test-config-mount/rootfs"

		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)
		setV86Config(ctx, t, engine, vmKey, &s4wave_vm.V86Config{
			Mounts: []*s4wave_vm.VmMount{
				{Path: "/workspace", ObjectKey: rootfsKey, Writable: true},
			},
		})

		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer readTx.Release()

		srpcClient, err := readTx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		resp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: vmKey,
		})
		if err != nil {
			t.Fatalf("AccessTypedObject failed: %v", err)
		}

		vmRef := resClient.CreateResourceReference(resp.ResourceId)
		defer vmRef.Release()

		vmClient, err := vmRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient for vm resource failed: %v", err)
		}
		v86fsSvc := unixfs_v86fs.NewSRPCV86FsServiceClient(vmClient)

		streamCtx, streamCancel := context.WithTimeout(ctx, 5*time.Second)
		defer streamCancel()

		stream, err := v86fsSvc.RelayV86Fs(streamCtx)
		if err != nil {
			t.Fatalf("RelayV86Fs failed: %v", err)
		}

		notify, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv seeded MOUNT_NOTIFY failed: %v", err)
		}
		mountNotify := notify.GetMountNotify()
		if mountNotify == nil {
			t.Fatalf("expected seeded MOUNT_NOTIFY, got %T", notify.GetBody())
		}
		if mountNotify.GetName() != "workspace" || mountNotify.GetMountPath() != "/workspace" {
			t.Fatalf("seeded MOUNT_NOTIFY = name %q path %q, want workspace /workspace",
				mountNotify.GetName(), mountNotify.GetMountPath())
		}

		err = stream.Send(&unixfs_v86fs.V86FsMessage{
			Tag: 1,
			Body: &unixfs_v86fs.V86FsMessage_MountRequest{
				MountRequest: &unixfs_v86fs.V86FsMountRequest{Name: "workspace"},
			},
		})
		if err != nil {
			t.Fatalf("Send MOUNT workspace failed: %v", err)
		}
		reply, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv MOUNT workspace reply failed: %v", err)
		}
		mountReply := reply.GetMountReply()
		if mountReply == nil {
			t.Fatalf("expected MOUNT workspace reply, got %T", reply.GetBody())
		}
		if mountReply.GetStatus() != 0 {
			t.Fatalf("MOUNT workspace failed with status %d", mountReply.GetStatus())
		}
		if mountReply.GetRootInodeId() == 0 {
			t.Fatal("expected non-zero workspace root inode ID")
		}
	})

	t.Run("SetV86ConfigOpApplies", func(t *testing.T) {
		engine, cleanup := setupVmV86WorldEngine(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-setconfig/vm"
		rootfsKey := "vm-v86-test-setconfig/rootfs"

		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		// Apply SetV86ConfigOp with a populated Config; op type must be
		// registered and ApplyWorldOp must return no sys error.
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		newCfg := &s4wave_vm.V86Config{
			MemoryMb:      512,
			VgaMemoryMb:   16,
			Networking:    true,
			SerialEnabled: true,
			BootArgs:      "quiet console=ttyS0",
			Mounts: []*s4wave_vm.VmMount{
				{Path: "/workspace", ObjectKey: rootfsKey, Writable: true},
			},
		}
		setOp := s4wave_vm.NewSetV86ConfigOp(vmKey, newCfg)
		setOpData, err := setOp.MarshalVT()
		if err != nil {
			tx.Release()
			t.Fatalf("MarshalVT (setconfig) failed: %v", err)
		}
		_, sysErr, err := tx.ApplyWorldOp(ctx, s4wave_vm.SetV86ConfigOpId, setOpData, "")
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp (setconfig) failed: %v (sysErr=%v)", err, sysErr)
		}
		if sysErr {
			tx.Release()
			t.Fatalf("ApplyWorldOp (setconfig) returned sysErr=true")
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit failed: %v", err)
		}
		tx.Release()

		// Applying to a non-existent object should surface a validation error
		// without aborting the engine.
		tx2, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction (bad key) failed: %v", err)
		}
		missOp := s4wave_vm.NewSetV86ConfigOp("vm-v86-test-setconfig/missing", newCfg)
		missData, err := missOp.MarshalVT()
		if err != nil {
			tx2.Release()
			t.Fatalf("MarshalVT (missing) failed: %v", err)
		}
		_, _, err = tx2.ApplyWorldOp(ctx, s4wave_vm.SetV86ConfigOpId, missData, "")
		if err == nil {
			tx2.Release()
			t.Fatal("ApplyWorldOp on missing object should error")
		}
		tx2.Release()
	})

	t.Run("SetV86StateOpTransitions", func(t *testing.T) {
		engine, cleanup := setupVmV86WorldEngine(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-setstate/vm"
		rootfsKey := "vm-v86-test-setstate/rootfs"
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		apply := func(state s4wave_vm.VmState, expectOK bool) {
			t.Helper()
			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatalf("NewTransaction failed: %v", err)
			}
			op := s4wave_vm.NewSetV86StateOp(vmKey, state, "")
			data, err := op.MarshalVT()
			if err != nil {
				tx.Release()
				t.Fatalf("MarshalVT failed: %v", err)
			}
			_, _, applyErr := tx.ApplyWorldOp(ctx, s4wave_vm.SetV86StateOpId, data, "")
			if expectOK {
				if applyErr != nil {
					tx.Release()
					t.Fatalf("ApplyWorldOp -> %s: unexpected err: %v", state.String(), applyErr)
				}
				if err := tx.Commit(ctx); err != nil {
					tx.Release()
					t.Fatalf("Commit failed: %v", err)
				}
			}
			if !expectOK && applyErr == nil {
				tx.Release()
				t.Fatalf("ApplyWorldOp -> %s: expected error, got nil", state.String())
			}
			tx.Release()
		}

		// STARTING is a compatibility alias that stores desired RUNNING.
		apply(s4wave_vm.VmState_VmState_STARTING, true)
		// A repeated RUNNING request is rejected until the observed runtime
		// state changes or the caller requests STOPPED.
		apply(s4wave_vm.VmState_VmState_RUNNING, false)
		apply(s4wave_vm.VmState_VmState_STOPPING, true)
		// STOPPING is a compatibility alias that stores desired STOPPED;
		// STOPPED completes the observed stop.
		apply(s4wave_vm.VmState_VmState_STOPPED, true)
		// A new STARTING request creates a new run generation.
		apply(s4wave_vm.VmState_VmState_STARTING, true)
		apply(s4wave_vm.VmState_VmState_STOPPED, true)
		// ERROR is an observed-only state and is not a desired-state request.
		apply(s4wave_vm.VmState_VmState_ERROR, false)
	})

	t.Run("V86ImageCreateAndSetMetadata", func(t *testing.T) {
		engine, cleanup := setupVmV86WorldEngine(ctx, t, tb)
		defer cleanup()

		imgKey := "vm-image-test/default"

		// CreateV86ImageOp stores a fresh V86Image with the supplied metadata.
		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		createdAt := time.Now()
		createOp := s4wave_vm.NewCreateV86ImageOp(imgKey, &s4wave_vm.V86Image{
			Name:     "debian-default",
			Version:  "1.0.0",
			Platform: "v86",
			Distro:   "debian",
			Tags:     []string{"default"},
		}, createdAt)
		createData, err := createOp.MarshalVT()
		if err != nil {
			tx.Release()
			t.Fatalf("MarshalVT (create image) failed: %v", err)
		}
		if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.CreateV86ImageOpId, createData, ""); err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp (create image) failed: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit (create image) failed: %v", err)
		}
		tx.Release()

		// SetV86ImageMetadataOp replaces metadata while preserving the op type.
		tx2, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		setOp := s4wave_vm.NewSetV86ImageMetadataOp(imgKey, &s4wave_vm.V86Image{
			Name:          "debian-default",
			Version:       "1.1.0",
			Platform:      "v86",
			Distro:        "debian",
			KernelVersion: "6.1.0-13-686",
			Description:   "Debian minimal userspace",
			Tags:          []string{"default", "browser"},
		})
		setData, err := setOp.MarshalVT()
		if err != nil {
			tx2.Release()
			t.Fatalf("MarshalVT (set metadata) failed: %v", err)
		}
		if _, _, err := tx2.ApplyWorldOp(ctx, s4wave_vm.SetV86ImageMetadataOpId, setData, ""); err != nil {
			tx2.Release()
			t.Fatalf("ApplyWorldOp (set metadata) failed: %v", err)
		}
		if err := tx2.Commit(ctx); err != nil {
			tx2.Release()
			t.Fatalf("Commit (set metadata) failed: %v", err)
		}
		tx2.Release()

		// Applying SetV86ImageMetadataOp against a missing key must error.
		tx3, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction (missing) failed: %v", err)
		}
		missOp := s4wave_vm.NewSetV86ImageMetadataOp("vm-image-test/missing", &s4wave_vm.V86Image{
			Name: "missing", Platform: "v86",
		})
		missData, err := missOp.MarshalVT()
		if err != nil {
			tx3.Release()
			t.Fatalf("MarshalVT (missing) failed: %v", err)
		}
		if _, _, err := tx3.ApplyWorldOp(ctx, s4wave_vm.SetV86ImageMetadataOpId, missData, ""); err == nil {
			tx3.Release()
			t.Fatal("ApplyWorldOp on missing V86Image should error")
		}
		tx3.Release()
	})

	t.Run("ExecuteEmitsInitialStopped", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-exec-stopped/vm"
		rootfsKey := "vm-v86-test-exec-stopped/rootfs"
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		stream, execCancel := openExecuteStream(ctx, t, resClient, engine, vmKey)
		defer execCancel()

		status, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv status failed: %v", err)
		}
		if got := status.GetState(); got != s4wave_process.ExecutionState_ExecutionState_STOPPED {
			t.Fatalf("expected STOPPED, got %v", got)
		}
	})

	t.Run("ExecuteStartingReachesError", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-exec-start/vm"
		rootfsKey := "vm-v86-test-exec-start/rootfs"
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		// Request STARTING so the handler drives the plugin load path.
		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STARTING, "")

		stream, execCancel := openExecuteStream(ctx, t, resClient, engine, vmKey)
		defer execCancel()

		// Mount resolves but plugin load fails (no plugin host in test);
		// handler must surface STARTING before flipping to ERROR.
		expectStatusSequence(t, stream,
			s4wave_process.ExecutionState_ExecutionState_STARTING,
			s4wave_process.ExecutionState_ExecutionState_ERROR,
		)
	})

	t.Run("ExecuteMountResolveError", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-exec-mountfail/vm"
		createVmV86WithoutRootfs(ctx, t, engine, vmKey)

		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STARTING, "")

		stream, execCancel := openExecuteStream(ctx, t, resClient, engine, vmKey)
		defer execCancel()

		// V86Image has no v86image/rootfs edge -> resolveV86Mount fails.
		expectStatusSequence(t, stream,
			s4wave_process.ExecutionState_ExecutionState_STARTING,
			s4wave_process.ExecutionState_ExecutionState_ERROR,
		)
	})

	t.Run("ExecuteStartingLoadsInstancedSpacewaveV86Plugin", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		pluginCtrl := newTestV86PluginLoadController()
		releasePluginCtrl, err := tb.Bus.AddController(ctx, pluginCtrl, nil)
		if err != nil {
			t.Fatalf("AddController(plugin load): %v", err)
		}
		defer releasePluginCtrl()

		vmKey := "vm-v86-test-exec-plugin/vm"
		rootfsKey := "vm-v86-test-exec-plugin/rootfs"
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STARTING, "")

		stream, execCancel := openExecuteStream(ctx, t, resClient, engine, vmKey)
		defer execCancel()

		expectStatusSequence(t, stream, s4wave_process.ExecutionState_ExecutionState_STARTING)

		req := pluginCtrl.expectLoad(t, ctx)
		if req.pluginID != "spacewave-v86" {
			t.Fatalf("plugin id = %q, want spacewave-v86", req.pluginID)
		}
		if req.instanceKey != vmKey {
			t.Fatalf("instance key = %q, want %q", req.instanceKey, vmKey)
		}
		if req.inst.CloseIfUnreferenced(false) {
			t.Fatal("plugin directive closed while VM is still running")
		}

		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STOPPED, "")
		expectStatusSequence(t, stream, s4wave_process.ExecutionState_ExecutionState_STOPPED)
		if !req.inst.CloseIfUnreferenced(false) {
			t.Fatal("plugin directive still referenced after VM stopped")
		}
	})

	t.Run("ExecuteReleasesInstancedPluginWhenVmDisappears", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		pluginCtrl := newTestV86PluginLoadController()
		releasePluginCtrl, err := tb.Bus.AddController(ctx, pluginCtrl, nil)
		if err != nil {
			t.Fatalf("AddController(plugin load): %v", err)
		}
		defer releasePluginCtrl()

		vmKey := "vm-v86-test-exec-plugin-delete/vm"
		rootfsKey := "vm-v86-test-exec-plugin-delete/rootfs"
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STARTING, "")

		stream, execCancel := openExecuteStream(ctx, t, resClient, engine, vmKey)
		defer execCancel()

		expectStatusSequence(t, stream, s4wave_process.ExecutionState_ExecutionState_STARTING)

		req := pluginCtrl.expectLoad(t, ctx)
		if req.pluginID != "spacewave-v86" {
			t.Fatalf("plugin id = %q, want spacewave-v86", req.pluginID)
		}
		if req.instanceKey != vmKey {
			t.Fatalf("instance key = %q, want %q", req.instanceKey, vmKey)
		}
		if req.inst.CloseIfUnreferenced(false) {
			t.Fatal("plugin directive closed while VM is still running")
		}

		deleteObject(ctx, t, engine, vmKey)
		if _, err := stream.Recv(); err == nil {
			t.Fatal("expected Execute stream to close after VM deletion")
		}
		if !req.inst.CloseIfUnreferenced(false) {
			t.Fatal("plugin directive still referenced after VM deletion")
		}
	})

	t.Run("ExecuteReactsToSetStateStopped", func(t *testing.T) {
		resClient, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
		defer cleanup()

		vmKey := "vm-v86-test-exec-reactive/vm"
		rootfsKey := "vm-v86-test-exec-reactive/rootfs"
		createVmV86WithRootfs(ctx, t, engine, vmKey, rootfsKey)

		stream, execCancel := openExecuteStream(ctx, t, resClient, engine, vmKey)
		defer execCancel()

		// Initial stored state is STOPPED.
		if status, err := stream.Recv(); err != nil {
			t.Fatalf("Recv initial status failed: %v", err)
		} else if got := status.GetState(); got != s4wave_process.ExecutionState_ExecutionState_STOPPED {
			t.Fatalf("expected STOPPED, got %v", got)
		}

		// Flip to STARTING: handler must wake and emit STARTING (then ERROR).
		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STARTING, "")
		expectStatusSequence(t, stream,
			s4wave_process.ExecutionState_ExecutionState_STARTING,
			s4wave_process.ExecutionState_ExecutionState_ERROR,
		)

		// Clear the failed run by requesting STOPPED; the owner re-emits it.
		applySetV86State(ctx, t, engine, vmKey, s4wave_vm.VmState_VmState_STOPPED, "")
		expectStatusSequence(t, stream,
			s4wave_process.ExecutionState_ExecutionState_STOPPED,
		)
	})
}

// openExecuteStream opens an Execute stream against the VmV86 resource on the
// given engine. The returned cancel closes the stream context.
func openExecuteStream(
	ctx context.Context,
	t *testing.T,
	resClient *resource_client.Client,
	engine *s4wave_world.Engine,
	vmKey string,
) (s4wave_process.SRPCPersistentExecutionService_ExecuteClient, context.CancelFunc) {
	t.Helper()
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}
	t.Cleanup(readTx.Release)

	srpcClient, err := readTx.GetResourceRef().GetClient()
	if err != nil {
		t.Fatalf("GetClient failed: %v", err)
	}
	typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
	resp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: vmKey,
	})
	if err != nil {
		t.Fatalf("AccessTypedObject failed: %v", err)
	}

	vmRef := resClient.CreateResourceReference(resp.ResourceId)
	t.Cleanup(vmRef.Release)

	vmClient, err := vmRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient for vm resource failed: %v", err)
	}

	execSvc := s4wave_process.NewSRPCPersistentExecutionServiceClient(vmClient)
	execCtx, execCancel := context.WithTimeout(ctx, 10*time.Second)
	stream, err := execSvc.Execute(execCtx, &s4wave_process.ExecuteRequest{})
	if err != nil {
		execCancel()
		t.Fatalf("Execute failed: %v", err)
	}
	return stream, execCancel
}

// expectStatusSequence reads status messages from the stream and verifies they
// match the expected ordered list.
func expectStatusSequence(
	t *testing.T,
	stream s4wave_process.SRPCPersistentExecutionService_ExecuteClient,
	expected ...s4wave_process.ExecutionState,
) {
	t.Helper()
	for i, want := range expected {
		status, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv[%d] failed: %v", i, err)
		}
		if got := status.GetState(); got != want {
			t.Fatalf("status[%d]: want %v, got %v (error=%q)", i, want, got, status.GetError())
		}
	}
}

// applySetV86State applies SetV86StateOp and commits the transaction.
func applySetV86State(
	ctx context.Context,
	t *testing.T,
	engine *s4wave_world.Engine,
	vmKey string,
	state s4wave_vm.VmState,
	errorMessage string,
) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}
	op := s4wave_vm.NewSetV86StateOp(vmKey, state, errorMessage)
	data, err := op.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT (SetV86StateOp) failed: %v", err)
	}
	if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.SetV86StateOpId, data, ""); err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (SetV86StateOp %s) failed: %v", state.String(), err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Release()
		t.Fatalf("Commit (SetV86StateOp %s) failed: %v", state.String(), err)
	}
	tx.Release()
}

// deleteObject deletes an object and commits the write transaction.
func deleteObject(ctx context.Context, t *testing.T, engine *s4wave_world.Engine, objKey string) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}
	deleted, err := tx.DeleteObject(ctx, objKey)
	if err != nil {
		tx.Release()
		t.Fatalf("DeleteObject(%s) failed: %v", objKey, err)
	}
	if !deleted {
		tx.Release()
		t.Fatalf("DeleteObject(%s) did not delete an object", objKey)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Release()
		t.Fatalf("Commit (DeleteObject %s) failed: %v", objKey, err)
	}
	tx.Release()
}

// setupVmV86WorldEngine creates a world engine with VmV86 + UnixFS object types.
func setupVmV86WorldEngine(ctx context.Context, t *testing.T, tb *world_testbed.Testbed) (*s4wave_world.Engine, func()) {
	_, engine, cleanup := setupVmV86WorldEngineWithClient(ctx, t, tb)
	return engine, cleanup
}

// setupVmV86WorldEngineWithClient creates a world engine with VmV86 + UnixFS object types,
// also returns the resource client for creating resource references.
func setupVmV86WorldEngineWithClient(ctx context.Context, t *testing.T, tb *world_testbed.Testbed) (*resource_client.Client, *s4wave_world.Engine, func()) {
	objectTypes := map[string]objecttype.ObjectType{
		s4wave_vm.VmV86TypeID:            s4wave_vm_world.VmV86Type,
		s4wave_vm.V86ImageTypeID:         s4wave_vm_world.V86ImageType,
		s4wave_unixfs_world.UnixFSTypeID: s4wave_unixfs_world.UnixFSType,
	}
	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		return objectTypes[typeID], nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeCtrlRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatalf("Failed to add ObjectType controller: %v", err)
	}

	resClient, engine, clientCleanup := setupWorldResourceClient(ctx, t, tb)

	cleanup := func() {
		engine.Release()
		clientCleanup()
		objectTypeCtrlRelease()
	}

	return resClient, engine, cleanup
}

// createVmV86WithRootfs creates a VmV86 world object backed by a V86Image whose
// =v86image/rootfs= edge points at a UnixFS rootfs object. Mount resolution for
// the empty/rootfs name flows VM -> v86/image -> V86Image -> v86image/rootfs.
func createVmV86WithRootfs(ctx context.Context, t *testing.T, engine *s4wave_world.Engine, vmKey, rootfsKey string) {
	imageKey := vmKey + "-image"
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}

	// Create the UnixFS rootfs object via world op.
	fsInitOp := &unixfs_world.FsInitOp{
		ObjectKey: rootfsKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
		Timestamp: timestamppb.New(time.Now()),
	}
	opData, err := fsInitOp.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT failed: %v", err)
	}
	_, _, err = tx.ApplyWorldOp(ctx, "hydra/unixfs/init", opData, "")
	if err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (unixfs init) failed: %v", err)
	}

	// Create a V86Image that carries each runtime asset on its v86image/* edge.
	imageOp := s4wave_vm.NewCreateV86ImageOp(imageKey, &s4wave_vm.V86Image{
		Name:     "test-image",
		Platform: "v86",
		Tags:     []string{"default"},
	}, time.Now())
	imageOpData, err := imageOp.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT (create image) failed: %v", err)
	}
	if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.CreateV86ImageOpId, imageOpData, ""); err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (create image) failed: %v", err)
	}
	for _, pred := range []string{
		string(s4wave_vm.PredV86ImageRootfs),
		string(s4wave_vm.PredV86ImageKernel),
		string(s4wave_vm.PredV86ImageBiosSeabios),
		string(s4wave_vm.PredV86ImageBiosVgabios),
		string(s4wave_vm.PredV86ImageWasm),
	} {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(imageKey, pred, rootfsKey, "")); err != nil {
			tx.Release()
			t.Fatalf("SetGraphQuad (%s) failed: %v", pred, err)
		}
	}

	// Create the VmV86 object via CreateVmV86Op; the op wires the v86/image
	// edge to the V86Image created above.
	createOp := s4wave_vm.NewCreateVmV86Op(vmKey, "test-vm", imageKey, time.Now())
	createOpData, err := createOp.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT failed: %v", err)
	}
	if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.CreateVmV86OpId, createOpData, ""); err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (create vm) failed: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		tx.Release()
		t.Fatalf("Commit failed: %v", err)
	}
	tx.Release()

	t.Logf("Created VmV86 %s via image %s with rootfs %s", vmKey, imageKey, rootfsKey)
}

// createVmV86WithoutRootfs creates a VmV86 backed by a V86Image that has no
// =v86image/rootfs= edge, so the rootfs mount fails to resolve. Exercises the
// mount-resolution ERROR path in the handler.
func createVmV86WithoutRootfs(ctx context.Context, t *testing.T, engine *s4wave_world.Engine, vmKey string) {
	imageKey := vmKey + "-image"
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}

	imageOp := s4wave_vm.NewCreateV86ImageOp(imageKey, &s4wave_vm.V86Image{
		Name:     "empty-image",
		Platform: "v86",
	}, time.Now())
	imageOpData, err := imageOp.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT (create image) failed: %v", err)
	}
	if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.CreateV86ImageOpId, imageOpData, ""); err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (create image) failed: %v", err)
	}

	createOp := s4wave_vm.NewCreateVmV86Op(vmKey, "test-vm-noroot", imageKey, time.Now())
	createOpData, err := createOp.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT failed: %v", err)
	}
	if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.CreateVmV86OpId, createOpData, ""); err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (create vm) failed: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		tx.Release()
		t.Fatalf("Commit failed: %v", err)
	}
	tx.Release()

	t.Logf("Created VmV86 %s via image %s without rootfs edge", vmKey, imageKey)
}

func setV86Config(ctx context.Context, t *testing.T, engine *s4wave_world.Engine, vmKey string, cfg *s4wave_vm.V86Config) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction failed: %v", err)
	}
	op := s4wave_vm.NewSetV86ConfigOp(vmKey, cfg)
	data, err := op.MarshalVT()
	if err != nil {
		tx.Release()
		t.Fatalf("MarshalVT (setconfig) failed: %v", err)
	}
	if _, _, err := tx.ApplyWorldOp(ctx, s4wave_vm.SetV86ConfigOpId, data, ""); err != nil {
		tx.Release()
		t.Fatalf("ApplyWorldOp (setconfig) failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Release()
		t.Fatalf("Commit failed: %v", err)
	}
	tx.Release()
}

type testV86PluginLoadRequest struct {
	pluginID    string
	instanceKey string
	inst        directive.Instance
}

type testV86PluginLoadController struct {
	requests chan testV86PluginLoadRequest
}

func newTestV86PluginLoadController() *testV86PluginLoadController {
	return &testV86PluginLoadController{
		requests: make(chan testV86PluginLoadRequest, 4),
	}
}

func (c *testV86PluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("resource/world/test-v86-plugin-load", controller.MustParseVersion("0.0.1"), "test")
}

func (c *testV86PluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testV86PluginLoadController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	c.requests <- testV86PluginLoadRequest{
		pluginID:    dir.LoadPluginID(),
		instanceKey: dir.LoadPluginInstanceKey(),
		inst:        inst,
	}
	return directive.R(
		directive.NewValueResolver([]bldr_plugin.LoadPluginValue{
			bldr_plugin.NewRunningPlugin(nil),
		}),
		nil,
	)
}

func (c *testV86PluginLoadController) Close() error {
	return nil
}

func (c *testV86PluginLoadController) expectLoad(
	t *testing.T,
	ctx context.Context,
) testV86PluginLoadRequest {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case req := <-c.requests:
		return req
	case <-waitCtx.Done():
		t.Fatalf("timed out waiting for plugin load directive: %v", waitCtx.Err())
		return testV86PluginLoadRequest{}
	}
}

// _ is a type assertion
var _ controller.Controller = ((*testV86PluginLoadController)(nil))
