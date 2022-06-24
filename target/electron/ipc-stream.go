//go:build !windows
// +build !windows

package electron

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/sirupsen/logrus"
)

// ipcStream implements the streaming to a named pipe
type ipcStream struct {
	ctx context.Context
	le  *logrus.Entry
	l   net.Listener

	t    chan struct{}
	mtx  sync.Mutex
	conn net.Conn
}

// unix socket listener -> read/writer
func newIpcStream(ctx context.Context, le *logrus.Entry, workDir, runtimeUuid string) (*ipcStream, error) {
	// pass the pipe name to use, unique generated per instance
	l, err := buildPipeListener(le, workDir, runtimeUuid)
	if err != nil {
		return nil, err
	}
	s := &ipcStream{ctx: ctx, le: le, l: l, t: make(chan struct{}, 1)}
	go s.acceptPump(l)
	return s, nil
}

func (s *ipcStream) Read(p []byte) (n int, err error) {
	for {
		s.mtx.Lock()
		conn := s.conn
		s.mtx.Unlock()

		n = 0
		err = nil

		if conn != nil {
			n, err = conn.Read(p)
		}

		if err == io.EOF {
			s.mtx.Lock()
			if s.conn == conn {
				_ = s.conn.Close()
				s.conn = nil
				select {
				case <-s.t:
				default:
				}
			}
			err = nil
			n = 0
			s.mtx.Unlock()
		}

		if err != nil || n != 0 {
			if err != nil {
				s.le.WithError(err).Warn("error receiving ipc data")
			} else {
				s.le.Debugf("received ipc data: %v", p[:n])
			}
			return
		}

		select {
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		case <-s.t:
		}
	}
}

func (s *ipcStream) Write(p []byte) (n int, err error) {
	for {
		s.mtx.Lock()
		conn := s.conn
		s.mtx.Unlock()
		if conn != nil {
			n, err = conn.Write(p)
			return n, err
		}

		select {
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		case <-s.t:
		}
	}
}

func (s *ipcStream) Close() error {
	if s.l != nil {
		return s.l.Close()
	}
	return nil
}

// acceptPump accepts incoming connections.
func (s *ipcStream) acceptPump(list net.Listener) {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := list.Accept()
		if err != nil {
			if err == io.EOF {
				return
			}
			s.le.WithError(err).Warn("error accepting ipc connections")
			return
		}

		s.le.Debug("accepted ipc connection")
		s.mtx.Lock()
		if s.conn != nil {
			_ = s.conn.Close()
		}
		s.conn = conn
		select {
		case s.t <- struct{}{}:
		default:
		}
		s.mtx.Unlock()
	}
}

// _ is a type assertion
var _ io.ReadWriteCloser = ((*ipcStream)(nil))
