//go:build !js

package browser_testbed

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Server exposes an srpc.Mux over WebSocket for browser E2E tests.
type Server struct {
	le         *logrus.Entry
	mux        srpc.Mux
	listener   net.Listener
	httpServer *http.Server

	mu      sync.Mutex
	running bool
}

// NewServer creates a new Server with the given mux.
func NewServer(le *logrus.Entry, mux srpc.Mux) *Server {
	return &Server{
		le:  le,
		mux: mux,
	}
}

// Start starts the WebSocket server on an available loopback port.
func (s *Server) Start(ctx context.Context) (int, error) {
	// Serialize startup with Stop and reject a duplicate listener.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reject a duplicate start without replacing the active listener.
	if s.running {
		return 0, errors.New("server already running")
	}

	// Bind an ephemeral loopback listener for the browser client.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, errors.Wrap(err, "failed to create listener")
	}
	s.listener = listener

	// Capture the assigned port for the test client.
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// Serve the RPC mux at the browser WebSocket endpoint.
	httpServer, err := srpc.NewHTTPServer(s.mux, "/ws", &websocket.AcceptOptions{
		InsecureSkipVerify: true, // browser tests accept clients from ephemeral origins.
	})
	if err != nil {
		listener.Close()
		return 0, errors.Wrap(err, "failed to create HTTP server")
	}

	// Configure and start the HTTP server on the listener.
	s.httpServer = &http.Server{
		Handler:           httpServer,
		ReadHeaderTimeout: time.Second * 30,
	}
	s.running = true

	// Serve requests until Stop shuts down the HTTP server.
	go func() {
		s.le.Infof("browser test server listening on port %d", port)
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.le.Errorf("HTTP server error: %v", err)
		}
	}()

	return port, nil
}

// Stop stops the server.
func (s *Server) Stop(ctx context.Context) error {
	// Serialize shutdown with Start and leave an idle server unchanged.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Leave the server unchanged when it has not been started.
	if !s.running {
		return nil
	}

	// Mark the server stopped before shutting down its listener.
	s.running = false

	// Shut down the HTTP server and its active connections.
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// GetPort returns the port the server is listening on, or 0 if not running.
func (s *Server) GetPort() int {
	// Read the assigned port while startup and shutdown are excluded.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return 0
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}
