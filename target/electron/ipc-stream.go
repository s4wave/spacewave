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
func newIpcStream(ctx context.Context, le *logrus.Entry, workDir, sessionUuid string) (*ipcStream, error) {
	// TODO: convert listener to read/writer
	// merge multiple sessions into a single packet stream
	// we expect only 1 stream from the child Electron instance
	// pass the pipe name to use, unique generated per instance
	l, err := buildPipeListener(le, workDir, sessionUuid)
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
		if conn != nil {
			n, err = conn.Read(p)
		}
		if err == io.EOF {
			err = nil
			n = 0
		}
		if err != nil || n != 0 {
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
			return conn.Write(p)
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

		s.mtx.Lock()
		if s.conn != nil {
			_ = s.conn.Close()
		}
		s.conn = conn
		s.mtx.Unlock()

	TrigLoop:
		for {
			select {
			case s.t <- struct{}{}:
			default:
				break TrigLoop
			}
		}
	}
}

// _ is a type assertion
var _ io.ReadWriteCloser = ((*ipcStream)(nil))
