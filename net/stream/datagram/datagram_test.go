package stream_datagram

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func udpSocket(t *testing.T) *net.UDPConn {
	t.Helper()
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socket.Close() })
	if err := socket.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return socket
}

// Actual UDP endpoints exercise framing across a stream seam. net.Pipe does not
// claim WebRTC connectivity; it deliberately permits split stream reads/writes.
func TestForwardPacketsAndPlayerIsolation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := udpSocket(t)
	target := server.LocalAddr().(*net.UDPAddr)
	var results []chan error
	var upstream []string
	for _, payloads := range [][][]byte{{[]byte("first"), {}, bytes.Repeat([]byte{7}, 1400)}, {[]byte("second"), []byte("two packets")}} {
		client, endpoint := udpSocket(t), udpSocket(t)
		left, right := net.Pipe()
		result := make(chan error, 2)
		results = append(results, result)
		go func() { result <- Forward(ctx, endpoint, client.LocalAddr().(*net.UDPAddr), left) }()
		go func() { result <- ForwardTarget(ctx, target, right) }()
		for _, payload := range payloads {
			if _, err := client.WriteToUDP(payload, endpoint.LocalAddr().(*net.UDPAddr)); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, MaxPacketSize)
			n, source, err := server.ReadFromUDP(buf)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf[:n], payload) {
				t.Fatalf("request packet = %v, want %v", buf[:n], payload)
			}
			if len(upstream) < len(results) {
				upstream = append(upstream, source.String())
			}
			if _, err := server.WriteToUDP(payload, source); err != nil {
				t.Fatal(err)
			}
			n, _, err = client.ReadFromUDP(buf)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf[:n], payload) {
				t.Fatalf("reply packet differs: got %d, want %d bytes", n, len(payload))
			}
		}
	}
	if upstream[0] == upstream[1] {
		t.Fatal("players shared upstream UDP association")
	}
	cancel()
	for _, result := range results {
		for range 2 {
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("forwarder did not stop")
			}
		}
	}
}

func TestForwardRejectsForeignSenderAndOversizedFrame(t *testing.T) {
	ctx := t.Context()
	client, foreign, endpoint := udpSocket(t), udpSocket(t), udpSocket(t)
	left, right := net.Pipe()
	defer right.Close()
	done := make(chan error, 1)
	go func() { done <- Forward(ctx, endpoint, client.LocalAddr().(*net.UDPAddr), left) }()
	address := endpoint.LocalAddr().(*net.UDPAddr)
	if _, err := foreign.WriteToUDP([]byte("foreign"), address); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriteToUDP([]byte("valid"), address); err != nil {
		t.Fatal(err)
	}
	right.SetReadDeadline(time.Now().Add(5 * time.Second))
	var header [2]byte
	// Reading exactly a frame proves an untrusted sender cannot become the peer.
	if _, err := io.ReadFull(right, header[:]); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, int(binary.BigEndian.Uint16(header[:])))
	if _, err := io.ReadFull(right, packet); err != nil {
		t.Fatal(err)
	}
	if string(packet) != "valid" {
		t.Fatalf("foreign sender forwarded: %q", packet)
	}
	binary.BigEndian.PutUint16(header[:], MaxPacketSize+1)
	if _, err := right.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || err.Error() != "stream packet exceeds maximum size" {
			t.Fatalf("oversize: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("invalid frame did not stop forwarder")
	}
}

func TestForwardLocalLearnsOneSender(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client, other, endpoint, server := udpSocket(t), udpSocket(t), udpSocket(t), udpSocket(t)
	left, right := net.Pipe()
	results := make(chan error, 2)
	go func() { results <- ForwardLocal(ctx, endpoint, left) }()
	go func() { results <- ForwardTarget(ctx, server.LocalAddr().(*net.UDPAddr), right) }()
	packet := make([]byte, 128)
	for _, payload := range []string{"initial handshake", "subsequent gameplay"} {
		if _, err := client.WriteToUDP([]byte(payload), endpoint.LocalAddr().(*net.UDPAddr)); err != nil {
			t.Fatal(err)
		}
		n, source, err := server.ReadFromUDP(packet)
		if err != nil {
			t.Fatal(err)
		}
		if string(packet[:n]) != payload {
			t.Fatalf("received %q, want %q", packet[:n], payload)
		}
		if _, err := server.WriteToUDP(packet[:n], source); err != nil {
			t.Fatal(err)
		}
		n, _, err = client.ReadFromUDP(packet)
		if err != nil {
			t.Fatal(err)
		}
		if string(packet[:n]) != payload {
			t.Fatalf("reply %q, want %q", packet[:n], payload)
		}
		if _, err := other.WriteToUDP([]byte("foreign sender"), endpoint.LocalAddr().(*net.UDPAddr)); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	for range 2 {
		select {
		case err := <-results:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("forwarder did not stop")
		}
	}
}

func TestForwardLocalCancellationBeforeFirstPacket(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	endpoint := udpSocket(t)
	left, right := net.Pipe()
	defer right.Close()
	result := make(chan error, 1)
	go func() { result <- ForwardLocal(ctx, endpoint, left) }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting for initial sender prevented shutdown")
	}
}
