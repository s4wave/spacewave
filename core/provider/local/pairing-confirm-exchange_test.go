//go:build !goscript

package provider_local

import (
	"context"
	"io"
	"net"
	"testing"

	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

type confirmPipeStream struct {
	net.Conn
}

func (s *confirmPipeStream) Close() error {
	return s.Conn.Close()
}

func TestRecvPairingConfirmStopsOnContextCancel(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	sess := stream_packet.NewSession(&confirmPipeStream{Conn: left}, 1024)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err, timedOut := recvPairingConfirm(ctx, sess)
	if err == nil {
		t.Fatal("expected canceled receive to return error")
	}
	if !timedOut {
		t.Fatal("expected receive to report timeout/cancel")
	}
	if _, err := right.Write([]byte{0}); err == nil {
		t.Fatal("expected paired stream to close after canceled receive")
	}
}

func TestRecvPairingConfirmReceivesMessage(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	recvSess := stream_packet.NewSession(&confirmPipeStream{Conn: left}, 1024)
	sendSess := stream_packet.NewSession(&confirmPipeStream{Conn: right}, 1024)
	sendErr := make(chan error, 1)
	go func() {
		sendErr <- sendSess.SendMsg(&PairingConfirmMessage{Confirmed: true})
	}()

	msg, err, timedOut := recvPairingConfirm(context.Background(), recvSess)
	if err != nil {
		t.Fatal(err)
	}
	if timedOut {
		t.Fatal("receive should not report timeout")
	}
	if !msg.GetConfirmed() || msg.GetRejected() {
		t.Fatalf("unexpected confirm message: confirmed=%v rejected=%v", msg.GetConfirmed(), msg.GetRejected())
	}
	if err := <-sendErr; err != nil && err != io.ErrClosedPipe {
		t.Fatalf("send failed: %v", err)
	}
}
