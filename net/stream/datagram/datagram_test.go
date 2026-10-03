package stream_datagram

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/stream"
)

// pipeMessageSize is the message size limit of the test message pipes. Larger
// packets take the control stream fallback.
const pipeMessageSize = 1200

func udpSocket(t *testing.T) *net.UDPConn {
	// Bind a loopback UDP socket with test cleanup and bounded I/O.
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

// Actual UDP endpoints exercise both the message plane and the framed control
// fallback. The message pipe does not claim WebRTC connectivity.
func TestForwardPacketsAndPlayerIsolation(t *testing.T) {
	// Create the UDP service and retain forwarding results for shutdown checks.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := udpSocket(t)
	target := server.LocalAddr().(*net.UDPAddr)
	var results []chan error
	var upstream []string

	// Exchange packets through separate upstream associations for each player.
	for _, payloads := range [][][]byte{{[]byte("first"), {}, bytes.Repeat([]byte{7}, 1400)}, {[]byte("second"), []byte("two packets")}} {
		// Start the player endpoint and its dedicated upstream forwarder.
		client, endpoint := udpSocket(t), udpSocket(t)
		left, right := stream.NewMessagePipe(pipeMessageSize)
		result := make(chan error, 2)
		results = append(results, result)
		go func() { result <- Forward(ctx, endpoint, client.LocalAddr().(*net.UDPAddr), left) }()
		go func() { result <- ForwardTarget(ctx, target, right) }()

		// Verify each request and reply through the message or control stream.
		for _, payload := range payloads {
			// Verify that the service receives the player packet unchanged.
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

			// Record the upstream association used by this player.
			if len(upstream) < len(results) {
				upstream = append(upstream, source.String())
			}

			// Verify that the service reply reaches the same player unchanged.
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

	// Verify that the two players use distinct upstream source addresses.
	if upstream[0] == upstream[1] {
		t.Fatal("players shared upstream UDP association")
	}

	// Cancel both player associations and require every forwarding pump to stop.
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
	// Start a forwarder restricted to the selected local UDP client.
	ctx := t.Context()
	client, foreign, endpoint := udpSocket(t), udpSocket(t), udpSocket(t)
	left, right := stream.NewMessagePipe(pipeMessageSize)
	defer right.Close()
	done := make(chan error, 1)
	go func() { done <- Forward(ctx, endpoint, client.LocalAddr().(*net.UDPAddr), left) }()

	// Send competing datagrams from the foreign sender and the selected client.
	address := endpoint.LocalAddr().(*net.UDPAddr)
	if _, err := foreign.WriteToUDP([]byte("foreign"), address); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriteToUDP([]byte("valid"), address); err != nil {
		t.Fatal(err)
	}
	right.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Reading exactly one message proves an untrusted sender cannot become the peer.
	packet := make([]byte, MaxPacketSize)
	n, err := right.Read(packet)
	if err != nil {
		t.Fatal(err)
	}
	if string(packet[:n]) != "valid" {
		t.Fatalf("foreign sender forwarded: %q", packet[:n])
	}

	// Inject an oversized control frame into the forwarding stream.
	var header [2]byte
	binary.BigEndian.PutUint16(header[:], MaxPacketSize+1)
	if _, err := right.Control().Write(header[:]); err != nil {
		t.Fatal(err)
	}

	// Verify that the oversized frame stops forwarding with the size error.
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
	// Connect a loopback endpoint to the UDP service without a fixed local sender.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client, other, endpoint, server := udpSocket(t), udpSocket(t), udpSocket(t), udpSocket(t)
	left, right := stream.NewMessagePipe(pipeMessageSize)
	results := make(chan error, 2)
	go func() { results <- ForwardLocal(ctx, endpoint, left) }()
	go func() { results <- ForwardTarget(ctx, server.LocalAddr().(*net.UDPAddr), right) }()

	// Verify that the learned sender receives replies across successive packets.
	packet := make([]byte, 128)
	for _, payload := range []string{"initial handshake", "subsequent gameplay"} {
		// Verify that the service receives the learned sender's packet.
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

		// Verify that the service reply returns to the learned sender.
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

		// Challenge the learned association with a datagram from another sender.
		if _, err := other.WriteToUDP([]byte("foreign sender"), endpoint.LocalAddr().(*net.UDPAddr)); err != nil {
			t.Fatal(err)
		}
	}

	// Cancel the association and require both forwarders to stop.
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
	// Start a local forwarder before any UDP sender has been learned.
	ctx, cancel := context.WithCancel(t.Context())
	endpoint := udpSocket(t)
	left, right := stream.NewMessagePipe(pipeMessageSize)
	defer right.Close()
	result := make(chan error, 1)
	go func() { result <- ForwardLocal(ctx, endpoint, left) }()

	// Verify that cancellation stops the forwarder while waiting for its first sender.
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
