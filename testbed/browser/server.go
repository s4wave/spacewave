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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return 0, errors.New("server already running")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, errors.Wrap(err, "failed to create listener")
	}
	s.listener = listener

	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	httpServer, err := srpc.NewHTTPServer(s.mux, "/ws", &websocket.AcceptOptions{
		InsecureSkipVerify: true, // browser tests accept clients from ephemeral origins.
	})
	if err != nil {
		listener.Close()
		return 0, errors.Wrap(err, "failed to create HTTP server")
	}

	s.httpServer = &http.Server{
		Handler:           httpServer,
		ReadHeaderTimeout: time.Second * 30,
	}
	s.running = true

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
