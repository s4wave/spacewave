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
	engine := &Engine{ctx: t.Context()}
	ctx, active := engine.begin(true, true, "", "code", "", StatusCodeGenerated)
	defer engine.Clear()
	engine.runSolicit(ctx, active, &transport.SessionTransport{})
	snapshot, _ := engine.Snapshot()
	if snapshot.Status != StatusFailed || snapshot.ErrMsg == "" {
		t.Fatalf("stopped transport did not fail pairing: %+v", snapshot)
	}
}

// TestPairingRejectionDelivery keeps the rejecting side alive until its peer
// receives the decision, whether that peer has already approved or is waiting.
func TestPairingRejectionDelivery(t *testing.T) {
	for _, approved := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting", true: "approved"}[approved], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			localChoice, remoteChoice := make(chan bool, 1), make(chan bool, 1)
			remoteSent := make(chan struct{})
			results := make(chan Status, 2)
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
			if approved {
				remoteChoice <- true
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-remoteSent:
				}
			}
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
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sess := stream_packet.NewSession(left, 1024)
	status, err := exchangeApproval(ctx, sess, make(chan bool), "operation", func(Status) {})
	if err == nil || status != StatusConfirmationTimeout {
		t.Fatalf("canceled approval returned status %v, error %v", status, err)
	}
	_ = right.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := right.Write([]byte{0}); err == nil {
		t.Fatal("canceled approval left its peer stream open")
	}
}
