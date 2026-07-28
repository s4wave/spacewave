//go:build !js && !windows

package bldr_tui_host

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

type proxyDialFunc func(context.Context, string, string) (net.Conn, error)

type proxyConnection struct {
	client net.Conn
	target net.Conn
}

type unixProxy struct {
	listener net.Listener
	path     string
	dir      string
	cancel   context.CancelFunc
	dial     proxyDialFunc

	mu          sync.Mutex
	connections map[*proxyConnection]struct{}
	closed      bool
	wg          sync.WaitGroup

	closeOnce sync.Once
	closeErr  error
	stopped   chan struct{}
	done      chan error
}

func startUnixProxy(ctx context.Context, targetPath string) (*unixProxy, error) {
	var dialer net.Dialer
	return startUnixProxyWithDial(ctx, targetPath, dialer.DialContext)
}

func startUnixProxyWithDial(
	ctx context.Context,
	targetPath string,
	dial proxyDialFunc,
) (*unixProxy, error) {
	dir, err := os.MkdirTemp("", "spacewave-tui-")
	if err != nil {
		return nil, errors.Wrap(err, "create TUI runtime directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, errors.Wrap(err, "secure TUI runtime directory")
	}
	path := filepath.Join(dir, "resource.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, errors.Wrap(err, "listen on private Resource socket")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.RemoveAll(dir)
		return nil, errors.Wrap(err, "secure private Resource socket")
	}
	proxyCtx, cancel := context.WithCancel(ctx)
	proxy := &unixProxy{
		listener:    listener,
		path:        path,
		dir:         dir,
		cancel:      cancel,
		dial:        dial,
		connections: make(map[*proxyConnection]struct{}),
		stopped:     make(chan struct{}),
		done:        make(chan error, 1),
	}
	go proxy.serve(proxyCtx, targetPath)
	return proxy, nil
}

func (p *unixProxy) endpoint() string {
	return "unix://" + p.path
}

func (p *unixProxy) close() error {
	p.closeOnce.Do(func() {
		p.closeErr = p.closeLaunch()
	})
	return p.closeErr
}

func (p *unixProxy) closeLaunch() error {
	p.cancel()

	p.mu.Lock()
	p.closed = true
	connections := make([]*proxyConnection, 0, len(p.connections))
	for connection := range p.connections {
		connections = append(connections, connection)
	}
	p.mu.Unlock()

	var closeErr error
	if err := p.listener.Close(); err != nil && !stderrors.Is(err, net.ErrClosed) {
		closeErr = stderrors.Join(closeErr, err)
	}
	for _, connection := range connections {
		if err := connection.client.Close(); err != nil && !stderrors.Is(err, net.ErrClosed) {
			closeErr = stderrors.Join(closeErr, err)
		}
		if err := connection.target.Close(); err != nil && !stderrors.Is(err, net.ErrClosed) {
			closeErr = stderrors.Join(closeErr, err)
		}
	}
	<-p.stopped
	p.wg.Wait()
	if err := os.RemoveAll(p.dir); err != nil {
		closeErr = stderrors.Join(closeErr, err)
	}
	return closeErr
}

func (p *unixProxy) serve(ctx context.Context, targetPath string) {
	defer close(p.stopped)
	for {
		client, err := p.listener.Accept()
		if err != nil {
			p.finish(proxyAcceptError(ctx, err))
			return
		}

		target, err := p.dial(ctx, "unix", targetPath)
		if err != nil {
			_ = client.Close()
			if ctx.Err() != nil {
				p.finish(nil)
				return
			}
			p.finish(errors.Wrap(err, "connect private Resource proxy to daemon"))
			return
		}

		connection := &proxyConnection{
			client: client,
			target: target,
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = client.Close()
			_ = target.Close()
			p.finish(nil)
			return
		}
		p.connections[connection] = struct{}{}
		p.wg.Add(1)
		p.mu.Unlock()
		go p.serveConnection(connection)
	}
}

func (p *unixProxy) serveConnection(connection *proxyConnection) {
	defer p.wg.Done()
	defer func() {
		p.mu.Lock()
		delete(p.connections, connection)
		p.mu.Unlock()
	}()

	copyDone := make(chan struct{}, 2)
	go proxyCopy(connection.target, connection.client, copyDone)
	go proxyCopy(connection.client, connection.target, copyDone)
	<-copyDone
	_ = connection.client.Close()
	_ = connection.target.Close()
	<-copyDone
}

func (p *unixProxy) finish(err error) {
	select {
	case p.done <- err:
	default:
	}
}

// proxyCopy treats copy failures as connection-local; closing the pair makes
// the canonical Resource client observe the failure and own reconnection.
func proxyCopy(dst net.Conn, src net.Conn, result chan<- struct{}) {
	_, _ = io.Copy(dst, src)
	result <- struct{}{}
}

func proxyAcceptError(ctx context.Context, err error) error {
	if ctx.Err() != nil || stderrors.Is(err, net.ErrClosed) {
		return nil
	}
	return errors.Wrap(err, "accept private Resource connection")
}
