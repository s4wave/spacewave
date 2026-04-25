//go:build !js

package browser_testbed

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// Server provides a WebSocket-based RPC server for browser E2E tests.
// It exposes an srpc.Mux over WebSocket for browser clients.
type Server struct {
	// le is the logger
	le *logrus.Entry
	// mux is the RPC mux to serve
	mux srpc.Mux
	// listener is the HTTP listener
	listener net.Listener
	// httpServer is the HTTP server
	httpServer *http.Server
	// mu protects server state
	mu sync.Mutex
	// running indicates if the server is running
	running bool
}

// NewServer creates a new Server with the given mux.
func NewServer(le *logrus.Entry, mux srpc.Mux) *Server {
	return &Server{
		le:  le,
		mux: mux,
	}
}

// Start starts the WebSocket server on a random available port.
// Returns the port number the server is listening on.
func (s *Server) Start(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return 0, fmt.Errorf("server already running")
	}

	// Create listener on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	// Get the port
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// Create HTTP/WebSocket server
	httpServer, err := srpc.NewHTTPServer(s.mux, "/ws", &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin for tests
	})
	if err != nil {
		listener.Close()
		return 0, fmt.Errorf("failed to create HTTP server: %w", err)
	}

	s.httpServer = &http.Server{
		Handler:           httpServer,
		ReadHeaderTimeout: time.Second * 30,
	}
	s.running = true

	// Start serving in background
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// GetPort returns the port the server is listening on, or 0 if not running.
func (s *Server) GetPort() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return 0
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}
