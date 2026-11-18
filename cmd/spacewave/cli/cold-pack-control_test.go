//go:build !js && !windows

package spacewave_cli

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/structpb"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/wrapperspb"
	"github.com/aperturerobotics/starpc/srpc"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// TestColdPackControlUnix checks the cold control on the daemon's protected
// SRPC transport, including the false BoolValue's zero-byte protobuf payload.
func TestColdPackControlUnix(t *testing.T) {
	// Restore the process-wide switch when this test ends.
	t.Cleanup(func() { block_store_s3.SetColdPackReads(true) })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Bind the existing Unix control transport and install its cold handler.
	sock := filepath.Join(shortSocketDir(t), socketName)
	lis, err := resource_listener.ListenProtectedUnix(sock, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	mux := srpc.NewMux()
	if err := mux.Register(coldPackControlHandler{}); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	go func() {
		// Serve one daemon connection.
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		mp, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			return
		}
		_ = server.AcceptMuxedConn(ctx, mp)
	}()

	// Dial that socket without any Resource service or fallback transport.
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Both directions of the runtime switch must be acknowledged.
	for _, on := range []bool{false, true} {
		if err := client.ExecCall(ctx, coldPackControlServiceID, coldPackSetReadsMethodID, wrapperspb.Bool(on), &emptypb.Empty{}); err != nil {
			t.Fatalf("toggle %t: %v", on, err)
		}
	}

	// A missing mount is not a zero-counter snapshot of a live cold tier.
	var stats structpb.Struct
	err = client.ExecCall(ctx, coldPackControlServiceID, coldPackStatsMethodID, &emptypb.Empty{}, &stats)
	if err == nil || !strings.Contains(err.Error(), "cold pack tier is not mounted") {
		t.Fatalf("stats without mounted cold packs: %v", err)
	}
}
