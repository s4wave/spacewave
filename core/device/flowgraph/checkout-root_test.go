//go:build !js

package device_flowgraph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/core/transport"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

// checkCheckoutRootAccess reads and attempts a fully specified write to a host file.
func checkCheckoutRootAccess(t *testing.T, ctx context.Context, device peer.ID, viewer *transport.SessionTransport, id protocol.ID, dir string, writable bool) {
	// Open the advertised stream as a known member of the Device's account.
	t.Helper()
	ms := openFrameControl(t, ctx, device, viewer, id)
	mc, err := srpc.NewMuxedConnWithRwc(ctx, ms.GetStream(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mc.Close()
	client := unixfs_rpc.NewSRPCFSCursorServiceClient(srpc.NewClientWithMuxedConn(mc))

	// Read the cursor client and obtain handles for the real root and file.
	events, err := client.FSCursorClient(ctx, &unixfs_rpc.FSCursorClientRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	opened, err := events.Recv()
	if err != nil || opened.GetInit() == nil {
		t.Fatalf("checkout root opened with %v, error %v", opened, err)
	}
	init := opened.GetInit()
	root, err := client.GetCursorOps(ctx, &unixfs_rpc.GetCursorOpsRequest{CursorHandleId: init.GetCursorHandleId()})
	if err != nil || root.GetUnixfsError() != nil {
		t.Fatalf("root operations = %v, error %v", root, err)
	}
	entry, err := client.OpsLookup(ctx, &unixfs_rpc.OpsLookupRequest{
		CursorHandleId: init.GetCursorHandleId(), OpsHandleId: root.GetOpsHandleId(),
		ClientHandleId: init.GetClientHandleId(), Name: "file.txt",
	})
	if err != nil || entry.GetUnixfsError() != nil {
		t.Fatalf("file lookup = %v, error %v", entry, err)
	}
	file, err := client.GetCursorOps(ctx, &unixfs_rpc.GetCursorOpsRequest{CursorHandleId: entry.GetCursorHandleId()})
	if err != nil || file.GetOpsHandleId() == 0 {
		t.Fatalf("file operations = %v, error %v", file, err)
	}

	// Read a range from the real host file before attempting the write.
	read, err := client.OpsReadAt(ctx, &unixfs_rpc.OpsReadAtRequest{OpsHandleId: file.GetOpsHandleId(), Offset: 1, Size: 2})
	if err != nil || read.GetUnixfsError() != nil || string(read.GetData()) != "ri" {
		t.Fatalf("file range = %v, error %v, want ri", read, err)
	}

	// Populate every request field so refusal depends on the configured root.
	written, err := client.OpsWriteAt(ctx, &unixfs_rpc.OpsWriteAtRequest{
		OpsHandleId: file.GetOpsHandleId(), Offset: 1, Data: []byte("WAVE"), Timestamp: timestamppb.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeErr := written.GetUnixfsError().ToGoError()
	want := "original"
	if writable {
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		want = "oWAVEnal"
	}
	if !writable && !errors.Is(writeErr, unixfs_errors.ErrReadOnly) {
		t.Fatalf("write error = %v, want read-only refusal", writeErr)
	}

	// Check the host bytes independently of the RPC service.
	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != want {
		t.Fatalf("host file = %q, error %v, want %q", data, err, want)
	}
}

// checkCheckoutRootResource resolves a Device protocol as an ordinary FSHandle Resource.
func checkCheckoutRootResource(t *testing.T, ctx context.Context, le *logrus.Entry, b bus.Bus, session peer.ID, engine world.Engine, capabilityID, dir string) {
	// Serve the Device over the Resource RPC with a read-only World grant.
	t.Helper()
	device := s4wave_device.NewDeviceResource(le, b, session, engine, world.NewEngineWorldState(engine, false), testSelfKey)
	resources, cleanup := resource_testbed.SetupResourceClientWithRoot(ctx, t, device.GetMux())
	defer cleanup()
	deviceRef := resources.AccessRootResource()
	defer deviceRef.Release()
	deviceClient, err := deviceRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	deviceService := s4wave_device.NewSRPCDeviceResourceServiceClient(deviceClient)

	// Select the live protocol by name and require a readable filesystem resource.
	opened, err := deviceService.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{Name: "skiffos"})
	if err != nil {
		t.Fatal(err)
	}
	if opened.GetCapabilityId() != capabilityID || opened.GetWriteEnabled() || !opened.GetWriteAvailable() {
		t.Fatalf("opened checkout root = %v", opened)
	}
	rootRef := resources.CreateResourceReference(opened.GetResourceId())
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	root := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)
	entry, err := root.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: "file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resources.CreateResourceReference(entry.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	file := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)

	// Read the host bytes from the child and require it to inherit read-only access.
	want, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	read, err := file.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{Length: int64(len(want))})
	if err != nil || string(read.GetData()) != string(want) {
		t.Fatalf("live resource data = %v, error %v, want %q", read, err, want)
	}
	_, err = file.WriteAt(ctx, &s4wave_unixfs.HandleWriteAtRequest{Data: []byte("bad")})
	if err == nil || err.Error() != unixfs_errors.ErrReadOnly.Error() {
		t.Fatalf("write through read-only handle = %v", err)
	}
}
