package saucer

import (
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

// TestYamuxBidirectional tests that a yamux server can open streams to a yamux client.
// This mirrors the saucer architecture: Go=server (outbound=false), C++=client (outbound=true).
func TestYamuxBidirectional(t *testing.T) {
	// Create a pipe to simulate the connection.
	serverConn, clientConn := net.Pipe()

	// Go side: yamux server (outbound=false) - matches saucer Go side.
	serverMC, err := srpc.NewMuxedConn(serverConn, false, nil)
	if err != nil {
		t.Fatalf("server muxed conn: %v", err)
	}

	// C++ side: yamux client (outbound=true) - matches saucer C++ side.
	clientMC, err := srpc.NewMuxedConn(clientConn, true, nil)
	if err != nil {
		t.Fatalf("client muxed conn: %v", err)
	}

	// Start client accept loop (simulates C++ accept_thread).
	clientDone := make(chan string, 1)
	go func() {
		t.Log("[client] waiting for server-initiated stream")
		stream, err := clientMC.AcceptStream()
		if err != nil {
			clientDone <- "accept error: " + err.Error()
			return
		}
		t.Log("[client] accepted stream, reading length prefix")

		// Read length-prefixed message (same protocol as C++ accept loop).
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(stream, lenBuf); err != nil {
			clientDone <- "read length error: " + err.Error()
			return
		}
		msgLen := binary.LittleEndian.Uint32(lenBuf)
		t.Logf("[client] read length: %d", msgLen)

		data := make([]byte, msgLen)
		if _, err := io.ReadFull(stream, data); err != nil {
			clientDone <- "read data error: " + err.Error()
			return
		}
		t.Logf("[client] read data: %q", string(data))

		// Write response.
		resp := []byte("ok")
		binary.LittleEndian.PutUint32(lenBuf, uint32(len(resp))) // #nosec G115 -- test data, bounded
		if _, err := stream.Write(lenBuf); err != nil {
			clientDone <- "write resp length error: " + err.Error()
			return
		}
		if _, err := stream.Write(resp); err != nil {
			clientDone <- "write resp error: " + err.Error()
			return
		}
		t.Log("[client] wrote response, closing stream")
		stream.Close()

		clientDone <- "ok"
	}()

	// Server opens a stream to the client (simulates debug bridge).
	ctx := t.Context()

	t.Log("[server] opening stream to client")
	stream, err := serverMC.OpenStream(ctx)
	if err != nil {
		t.Fatalf("[server] open stream: %v", err)
	}
	t.Log("[server] stream opened, writing code")

	code := "console.log('hello')"
	codeBytes := []byte(code)
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(codeBytes))) // #nosec G115 -- test data, bounded
	if _, err := stream.Write(lenBuf); err != nil {
		t.Fatalf("[server] write length: %v", err)
	}
	if _, err := stream.Write(codeBytes); err != nil {
		t.Fatalf("[server] write code: %v", err)
	}
	t.Log("[server] code written, reading response")

	// Read response.
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		t.Fatalf("[server] read response length: %v", err)
	}
	respLen := binary.LittleEndian.Uint32(lenBuf)
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(stream, resp); err != nil {
		t.Fatalf("[server] read response: %v", err)
	}
	stream.Close()
	t.Logf("[server] got response: %q", string(resp))

	// Wait for client.
	result := <-clientDone
	if result != "ok" {
		t.Fatalf("[client] error: %s", result)
	}

	serverMC.Close()
	clientMC.Close()
}
