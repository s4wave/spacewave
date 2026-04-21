package coord

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Mesh manages the cross-process SRPC mesh. Each participant listens on a
// Unix domain socket and connects to other participants discovered via the
// participant watcher.
type Mesh struct {
	le         *logrus.Entry
	mux        srpc.Mux
	srv        *srpc.Server
	listener   net.Listener
	socketPath string

	mu      sync.Mutex
	clients map[uint32]*meshClient // PID -> client
}

// meshClient tracks a connection to a remote participant.
type meshClient struct {
	conn   srpc.MuxedConn
	client srpc.Client
}

// NewMesh creates a new SRPC mesh. The mux is used for both the local
// server and can have additional services registered on it.
func NewMesh(le *logrus.Entry, pid uint32, getRole func() ParticipantRole, caps []string) *Mesh {
	mux := srpc.NewMux()
	m := &Mesh{
		le:      le,
		mux:     mux,
		srv:     srpc.NewServer(mux),
		clients: make(map[uint32]*meshClient),
	}
	// Register the base ParticipantService.
	_ = SRPCRegisterParticipantService(mux, &participantServiceServer{
		pid:     pid,
		getRole: getRole,
		caps:    caps,
	})
	return m
}

// participantServiceServer implements SRPCParticipantServiceServer.
type participantServiceServer struct {
	pid     uint32
	getRole func() ParticipantRole
	caps    []string
}

// GetParticipantInfo implements SRPCParticipantServiceServer.
func (s *participantServiceServer) GetParticipantInfo(ctx context.Context, req *GetParticipantInfoRequest) (*GetParticipantInfoResponse, error) {
	return &GetParticipantInfoResponse{
		Pid:          s.pid,
		Role:         s.getRole(),
		Capabilities: s.caps,
	}, nil
}

// _ is a type assertion.
var _ SRPCParticipantServiceServer = (*participantServiceServer)(nil)

// Mux returns the local SRPC mux for registering services.
func (m *Mesh) Mux() srpc.Mux {
	return m.mux
}

// SocketPath returns the path of the local Unix socket listener.
func (m *Mesh) SocketPath() string {
	return m.socketPath
}

// Listen creates a Unix domain socket listener at the given directory.
// The socket is named coord-{pid}.sock.
func (m *Mesh) Listen(dir string) error {
	pid := os.Getpid()
	m.socketPath = filepath.Join(dir, "coord-"+strconv.Itoa(pid)+".sock")

	// Remove stale socket file.
	_ = os.Remove(m.socketPath)

	lis, err := net.Listen("unix", m.socketPath)
	if err != nil {
		return errors.Wrap(err, "listen unix socket")
	}
	m.listener = lis
	return nil
}

// Serve accepts connections on the listener. Blocks until ctx is cancelled.
func (m *Mesh) Serve(ctx context.Context) error {
	if m.listener == nil {
		return errors.New("mesh: not listening")
	}
	return srpc.AcceptMuxedListener(ctx, m.listener, m.srv, nil)
}

// Close shuts down the mesh: closes all client connections, the listener,
// and removes the socket file.
func (m *Mesh) Close() {
	m.mu.Lock()
	for pid, mc := range m.clients {
		mc.conn.Close()
		delete(m.clients, pid)
	}
	m.mu.Unlock()

	if m.listener != nil {
		m.listener.Close()
	}
	if m.socketPath != "" {
		_ = os.Remove(m.socketPath)
	}
}

// Connect dials a remote participant's Unix socket and establishes a
// Yamux + starpc client connection. Caches the connection by PID.
func (m *Mesh) Connect(ctx context.Context, pid uint32, socketPath string) (srpc.Client, error) {
	m.mu.Lock()
	if mc, ok := m.clients[pid]; ok {
		if !mc.conn.IsClosed() {
			m.mu.Unlock()
			return mc.client, nil
		}
		delete(m.clients, pid)
	}

	// Hold the lock during dial to prevent duplicate connections.
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		m.mu.Unlock()
		return nil, errors.Wrap(err, "dial participant "+strconv.Itoa(int(pid)))
	}

	muxed, err := srpc.NewMuxedConn(conn, true, nil)
	if err != nil {
		m.mu.Unlock()
		conn.Close()
		return nil, errors.Wrap(err, "mux connection to "+strconv.Itoa(int(pid)))
	}

	client := srpc.NewClientWithMuxedConn(muxed)
	m.clients[pid] = &meshClient{conn: muxed, client: client}
	m.mu.Unlock()

	m.le.WithField("pid", pid).Debug("connected to participant")
	return client, nil
}

// Disconnect closes and removes the connection to a participant.
func (m *Mesh) Disconnect(pid uint32) {
	m.mu.Lock()
	if mc, ok := m.clients[pid]; ok {
		mc.conn.Close()
		delete(m.clients, pid)
	}
	m.mu.Unlock()
}

// GetClient returns the cached starpc client for a participant, or nil.
func (m *Mesh) GetClient(pid uint32) srpc.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mc, ok := m.clients[pid]; ok && !mc.conn.IsClosed() {
		return mc.client
	}
	return nil
}
