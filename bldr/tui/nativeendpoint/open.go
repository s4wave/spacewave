//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/s4wave/spacewave/bldr/tui/nativehost"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

var socketPair = func() ([2]int, error) {
	return syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
}

// endpointTransport keeps the child descriptor and parent SRPC transport together.
type endpointTransport struct {
	// child is the endpoint inherited by the child process.
	child *os.File
	// conn is the parent endpoint connection.
	conn net.Conn
	// mux carries SRPC over conn.
	mux srpc.MuxedConn
	// invoker serves the endpoint API.
	invoker srpc.Invoker
}

// open creates and serves one independent endpoint set.
func open(ctx context.Context, c Config) (*nativehost.EndpointSet, error) {
	// Validate dependencies before acquiring sockets or server custody.
	if err := validateConfig(c); err != nil {
		return nil, err
	}
	serveCtx, cancel := context.WithCancel(ctx)
	transports := make([]endpointTransport, 0, 3)
	cleanup := func() error {
		cancel()
		var closeErr error
		for _, transport := range transports {
			if transport.mux != nil {
				closeErr = errors.Join(closeErr, transport.mux.Close())
			} else if transport.conn != nil {
				closeErr = errors.Join(closeErr, transport.conn.Close())
			}
		}
		return closeErr
	}
	makeTransport := func(name string, invoker srpc.Invoker) (endpointTransport, error) {
		fds, err := socketPair()
		if err != nil {
			return endpointTransport{}, err
		}
		child := os.NewFile(uintptr(fds[0]), name+" child")
		parentFile := os.NewFile(uintptr(fds[1]), name+" parent")
		conn, err := net.FileConn(parentFile)
		_ = parentFile.Close()
		if err != nil {
			_ = child.Close()
			return endpointTransport{}, err
		}
		mc, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			_ = conn.Close()
			_ = child.Close()
			return endpointTransport{}, err
		}
		return endpointTransport{child: child, conn: conn, mux: mc, invoker: invoker}, nil
	}

	// Build isolated resource, state, and control transports for inheritance.
	resourceMux := srpc.NewMux(&serviceFilter{serviceID: resource.SRPCResourceServiceServiceID, invoker: srpc.NewClientInvoker(c.ResourceClient)})
	resourceTransport, err := makeTransport("resource", resourceMux)
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	transports = append(transports, resourceTransport)

	stateMux := srpc.NewMux()
	if err := native.SRPCRegisterStateService(stateMux, newStateService(c.StateStore, c.SelectedStateKey)); err != nil {
		return nil, errors.Join(err, cleanup())
	}
	stateTransport, err := makeTransport("state", stateMux)
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	transports = append(transports, stateTransport)

	controlMux := srpc.NewMux()
	if err := native.SRPCRegisterControlService(controlMux, newControlBridge(
		native.NewSRPCControlServiceClient(c.ResourceClient), c.CommandRegistryClient,
	)); err != nil {
		return nil, errors.Join(err, cleanup())
	}
	controlTransport, err := makeTransport("control", controlMux)
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	transports = append(transports, controlTransport)

	// Serve every parent transport and aggregate failures until shutdown.
	var servers sync.WaitGroup
	var serverErrorsMu sync.Mutex
	var serverErrors error
	for _, transport := range transports {
		server := srpc.NewServer(transport.invoker)
		servers.Add(1)
		go func(server *srpc.Server, mux srpc.MuxedConn) {
			defer servers.Done()
			if err := server.AcceptMuxedConn(serveCtx, mux); err != nil && serveCtx.Err() == nil {
				serverErrorsMu.Lock()
				serverErrors = errors.Join(serverErrors, err)
				serverErrorsMu.Unlock()
			}
		}(server, transport.mux)
	}

	// Expose idempotent close and join operations to the endpoint owner.
	var closeOnce sync.Once
	var closeErr error
	var waitOnce sync.Once
	var waitErr error
	closeFunc := func() error {
		closeOnce.Do(func() { closeErr = cleanup() })
		return closeErr
	}
	waitFunc := func() error {
		waitOnce.Do(func() {
			servers.Wait()
			serverErrorsMu.Lock()
			waitErr = serverErrors
			serverErrorsMu.Unlock()
		})
		return waitErr
	}
	return &nativehost.EndpointSet{
		Resource:  transports[0].child,
		State:     transports[1].child,
		Control:   transports[2].child,
		CloseFunc: closeFunc,
		WaitFunc:  waitFunc,
	}, nil
}

// serviceFilter exposes exactly one service through a shared resource client.
type serviceFilter struct {
	// serviceID is the only service accepted by the filter.
	serviceID string
	// invoker handles accepted methods.
	invoker srpc.Invoker
}

// InvokeMethod forwards only the selected resource service.
func (f *serviceFilter) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	if serviceID != f.serviceID {
		return false, nil
	}
	return f.invoker.InvokeMethod(serviceID, methodID, stream)
}

var _ srpc.Invoker = (*serviceFilter)(nil)
