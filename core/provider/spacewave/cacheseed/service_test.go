//go:build alphadebug

package provider_spacewave_cacheseed

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
)

// TestCacheSeedInspector exercises the end-to-end wiring: Register installs
// the service on an srpc.Mux, a client streams entries back, and live
// Record calls on the underlying buffer propagate through the stream.
func TestCacheSeedInspector(t *testing.T) {
	buf := provider_spacewave.NewCacheSeedBuffer(8)
	buf.Record(provider_spacewave.SeedReasonColdSeed, "/pre-0")
	buf.Record(provider_spacewave.SeedReasonGapRecovery, "/pre-1")

	mux := srpc.NewMux()
	if err := Register(mux, buf); err != nil {
		t.Fatalf("Register: %v", err)
	}

	server := srpc.NewServer(mux)
	client := NewSRPCCacheSeedInspectorClient(srpc.NewClient(srpc.NewServerPipe(server)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	strm, err := client.GetCacheSeedReasons(ctx, &GetCacheSeedReasonsRequest{})
	if err != nil {
		t.Fatalf("GetCacheSeedReasons: %v", err)
	}

	// Drain initial snapshot.
	for i, want := range []string{"/pre-0", "/pre-1"} {
		msg, err := strm.Recv()
		if err != nil {
			t.Fatalf("recv snapshot[%d]: %v", i, err)
		}
		if msg.GetPath() != want {
			t.Fatalf("snapshot[%d].Path = %q, want %q", i, msg.GetPath(), want)
		}
	}

	// Record a new entry after the snapshot is drained and assert it streams
	// through as a live update.
	buf.Record(provider_spacewave.SeedReasonMutation, "/live-0")
	msg, err := strm.Recv()
	if err != nil {
		t.Fatalf("recv live: %v", err)
	}
	if msg.GetPath() != "/live-0" {
		t.Fatalf("live.Path = %q, want %q", msg.GetPath(), "/live-0")
	}
	if msg.GetReason() != string(provider_spacewave.SeedReasonMutation) {
		t.Fatalf("live.Reason = %q, want %q", msg.GetReason(), provider_spacewave.SeedReasonMutation)
	}
	if msg.GetTimestampMs() == 0 {
		t.Fatalf("live.TimestampMs = 0, want non-zero")
	}

	// Cancel the stream and expect it to unwind.
	cancel()
	for {
		_, err := strm.Recv()
		if err == nil {
			continue
		}
		if err == io.EOF || err == context.Canceled {
			break
		}
		// Accept any stream-close error; SRPC may wrap the cancellation.
		break
	}
}
