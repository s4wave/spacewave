package signaling_rpc_frame

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
)

// dialEcho serves the starpc echo service over frames and returns a client
// connection to it.
func dialEcho(t *testing.T, ctx context.Context) (*Conn, echo.SRPCEchoerClient, chan error) {
	t.Helper()
	mux := srpc.NewMux()
	if err := echo.NewEchoServer(mux).Register(mux); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		_ = NewConn(r.Context(), ws).ReadPump(NewServerAccept(r.Context(), mux))
	}))
	t.Cleanup(srv.Close)

	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	conn := NewConn(ctx, ws)
	pumpErr := make(chan error, 1)
	go func() { pumpErr <- conn.ReadPump(nil) }()
	return conn, echo.NewSRPCEchoerClient(srpc.NewClient(conn.OpenStream)), pumpErr
}

func TestUnaryAndServerStream(t *testing.T) {
	ctx := t.Context()
	_, client, _ := dialEcho(t, ctx)

	// Run several calls to exercise stream id allocation and teardown.
	for range 3 {
		out, err := client.Echo(ctx, &echo.EchoMsg{Body: "hello"})
		if err != nil {
			t.Fatal(err)
		}
		if out.GetBody() != "hello" {
			t.Fatalf("echo returned %q", out.GetBody())
		}
	}

	strm, err := client.EchoServerStream(ctx, &echo.EchoMsg{Body: "stream"})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if msg.GetBody() != "stream" {
			t.Fatalf("stream returned %q", msg.GetBody())
		}
		n++
	}
	if n == 0 {
		t.Fatal("stream returned no messages")
	}
}

func TestSocketFailureFailsStreams(t *testing.T) {
	ctx := t.Context()
	conn, client, pumpErr := dialEcho(t, ctx)

	strm, err := client.EchoBidiStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The bidi handler greets first, which proves the stream is attached.
	if _, err := strm.Recv(); err != nil {
		t.Fatal(err)
	}

	_ = conn.ws.CloseNow()
	if err := <-pumpErr; err == nil {
		t.Fatal("read pump returned no error after the socket closed")
	}
	if _, err := strm.Recv(); err == nil {
		t.Fatal("stream received after the socket closed")
	}
	if _, err := client.Echo(ctx, &echo.EchoMsg{Body: "late"}); err == nil {
		t.Fatal("call succeeded after the socket closed")
	}
}
