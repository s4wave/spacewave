package pairing

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/transport"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestPairingSolicitStoppedTransport reports teardown instead of dereferencing its cleared bus.
func TestPairingSolicitStoppedTransport(t *testing.T) {
	// Start a soliciting attempt against a stopped transport.
	engine := &Engine{ctx: t.Context()}
	ctx, active := engine.begin(true, true, "", "code", "", StatusCodeGenerated)
	defer engine.Clear()
	engine.runSolicit(ctx, active, &transport.SessionTransport{})

	// Require the attempt to fail with a message.
	snapshot, _ := engine.Snapshot()
	if snapshot.Status != StatusFailed || snapshot.ErrMsg == "" {
		t.Fatalf("stopped transport did not fail pairing: %+v", snapshot)
	}
}

// TestPairingRejectionDelivery keeps the rejecting side alive until its peer
// receives the decision, whether that peer has already approved or is waiting.
func TestPairingRejectionDelivery(t *testing.T) {
	for _, approved := range []bool{false, true} {
		// Run the case with the remote side pre-approved or waiting.
		t.Run(map[bool]string{false: "waiting", true: "approved"}[approved], func(t *testing.T) {
			// Open a duplex pipe and run the exchange on both ends.
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			localChoice, remoteChoice := make(chan bool, 1), make(chan bool, 1)
			remoteSent := make(chan struct{})
			results := make(chan Status, 2)

			// Report each side's final status through the results channel.
			go func() {
				defer left.Close()
				status, _ := exchangeApproval(ctx, stream_packet.NewSession(left, 1024), localChoice, "operation", func(Status) {})
				results <- status
			}()
			go func() {
				defer right.Close()
				status, _ := exchangeApproval(ctx, stream_packet.NewSession(right, 1024), remoteChoice, "operation", func(Status) { close(remoteSent) })
				results <- status
			}()

			// Optionally approve the remote side first and wait for its send.
			if approved {
				remoteChoice <- true
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-remoteSent:
				}
			}

			// Reject locally and require both sides to report rejection.
			localChoice <- false
			for range 2 {
				select {
				case <-ctx.Done():
					t.Fatal("rejected exchange did not close")
				case status := <-results:
					if status != StatusPairingRejected {
						t.Fatalf("rejection returned status %v", status)
					}
				}
			}
		})
	}
}

// TestPairingApprovalCancellation closes a blocked duplex exchange when the
// user leaves pairing, including while the other client has not decided.
func TestPairingApprovalCancellation(t *testing.T) {
	// Open a pipe and cancel the exchange context immediately.
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sess := stream_packet.NewSession(left, 1024)

	// Run the exchange on an already canceled context.
	status, err := exchangeApproval(ctx, sess, make(chan bool), "operation", func(Status) {})
	if err == nil || status != StatusConfirmationTimeout {
		t.Fatalf("canceled approval returned status %v, error %v", status, err)
	}

	// Require the peer stream closed by writing to the canceled pipe.
	_ = right.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := right.Write([]byte{0}); err == nil {
		t.Fatal("canceled approval left its peer stream open")
	}
}
