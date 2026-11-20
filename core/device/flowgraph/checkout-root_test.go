//go:build !js

package device_flowgraph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/transport"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
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
