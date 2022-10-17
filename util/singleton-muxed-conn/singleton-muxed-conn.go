package singleton_muxed_conn

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/libp2p/go-libp2p/core/network"
)

// SingletonMuxedConn treats a net.Listener as a single multiplexed conn.
//
// Only the most recent net.MultiplexedConn will be kept.
// Calls are forwarded to the net.MultiplexConn.
type SingletonMuxedConn struct {
	// ctx is the listener context
	ctx       context.Context
	ctxCancel context.CancelFunc
	// trig triggers watchers when the state changes
	trig chan struct{}
	// conn is the most recent conn
	conn network.MuxedConn
	// mtx guards below fields
	mtx sync.Mutex
	// closedErr if set, indicates the conn is closed.
	closedErr error
}

// NewSingletonMuxedConn builds a new singleton MuxedConn.
func NewSingletonMuxedConn(ctx context.Context) *SingletonMuxedConn {
	sctx, sctxCancel := context.WithCancel(ctx)
	return &SingletonMuxedConn{
		ctx:       sctx,
		ctxCancel: sctxCancel,

		trig: make(chan struct{}, 1),
	}
}

// IsClosed returns whether a connection is fully closed.
func (l *SingletonMuxedConn) IsClosed() bool {
	l.mtx.Lock()
	closed := l.closedErr != nil
	l.mtx.Unlock()
	return closed
}

// SetConnection sets the latest MuxedConn and clears old streams.
// returns an error if the Singleton is closed.
func (l *SingletonMuxedConn) SetConnection(conn network.MuxedConn) error {
	l.mtx.Lock()
	if l.conn != nil {
		_ = l.conn.Close()
	}
	err := l.closedErr
	if err != nil {
		_ = conn.Close()
	} else {
		l.conn = conn
		l.doTrig()
	}
	l.mtx.Unlock()
	return err
}

// AcceptPump is a goroutine which accepts from a net.Listener.
// Closes the Singleton if Accept() returns an error.
func (l *SingletonMuxedConn) AcceptPump(list net.Listener) {
	for {
		nc, err := list.Accept()
		if err != nil {
			_ = l.CloseWithErr(err)
			return
		}

		mc, err := srpc.NewMuxedConn(nc, false)
		if err != nil {
			_ = nc.Close()
			continue
		}

		if err := l.SetConnection(mc); err != nil {
			_ = mc.Close()
			_ = list.Close()
			return
		}
	}
}

// OpenStream creates a new stream.
func (l *SingletonMuxedConn) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	var out network.MuxedStream
	err := l.tryConn(ctx, func(conn network.MuxedConn) error {
		var err error
		out, err = conn.OpenStream(ctx)
		return err
	})
	return out, err
}

// AcceptStream accepts a stream opened by the other side.
func (l *SingletonMuxedConn) AcceptStream() (network.MuxedStream, error) {
	var out network.MuxedStream
	ctx := l.ctx
	err := l.tryConn(ctx, func(conn network.MuxedConn) error {
		var err error
		out, err = conn.AcceptStream()
		return err
	})
	return out, err
}

// Close closes the Mplex listener.
func (l *SingletonMuxedConn) Close() error {
	return l.CloseWithErr(nil)
}

// CloseWithErr closes with an error.
// returns the l.closedErr
func (l *SingletonMuxedConn) CloseWithErr(closeErr error) error {
	var err error
	l.mtx.Lock()
	if l.closedErr == nil {
		l.closedErr = closeErr
	}
	if l.closedErr == nil {
		l.closedErr = io.EOF
	}
	err = l.closedErr
	l.ctxCancel()
	l.mtx.Unlock()
	return err
}

// doTrig triggers the listeners.
// expects mtx to be locked
func (l *SingletonMuxedConn) doTrig() {
	for {
		select {
		case l.trig <- struct{}{}:
		default:
			return
		}
	}
}

// waitConn waits for l.conn or for the Singleton to be closed.
func (l *SingletonMuxedConn) waitConn(ctx context.Context) (network.MuxedConn, error) {
	for {
		var conn network.MuxedConn
		l.mtx.Lock()
		err := l.closedErr
		if err == nil {
			if l.conn != nil {
				if l.conn.IsClosed() {
					l.conn = nil
					_ = l.conn.Close()
				} else {
					conn = l.conn
				}
			}
		}
		l.mtx.Unlock()
		if err != nil {
			return nil, err
		}
		if conn != nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-l.ctx.Done():
			l.mtx.Lock()
			err := l.closedErr
			l.mtx.Unlock()
			if err == nil || err == context.Canceled {
				err = io.EOF
			}
			return nil, err
		case <-l.trig:
		}
	}
}

// tryConn waits for l.conn, calls the callback, and closes the conn if it returns an error
// keeps trying until ctx is canceled or the Singleton is closed.
func (l *SingletonMuxedConn) tryConn(ctx context.Context, cb func(conn network.MuxedConn) error) error {
	for {
		conn, err := l.waitConn(ctx)
		if err != nil {
			return err
		}

		err = cb(conn)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		l.mtx.Lock()
		if l.conn == conn {
			_ = l.conn.Close()
			l.conn = nil
		}
		l.mtx.Unlock()
	}
}

// _ is a type assertion
var _ network.MuxedConn = ((*SingletonMuxedConn)(nil))
