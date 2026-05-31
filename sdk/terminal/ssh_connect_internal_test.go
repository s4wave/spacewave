//go:build !js

package s4wave_terminal

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDialSshClientHonorsContextDuringHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
		close(accepted)
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	client, err := dialSshClient(ctx, listener.Addr().String(), &ssh.ClientConfig{
		User:            "deploy",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if client != nil {
		_ = client.Close()
	}
	if err == nil {
		t.Fatal("expected stalled SSH handshake to fail")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("stalled SSH handshake returned after %s", elapsed)
	}
	if conn := <-accepted; conn != nil {
		_ = conn.Close()
	}
}
