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

type endpointTransport struct {
	child   *os.File
	conn    net.Conn
	mux     srpc.MuxedConn
	invoker srpc.Invoker
}

func open(ctx context.Context, c Config) (*nativehost.EndpointSet, error) {
	if err := validateConfig(c); err != nil {
		return nil, err
	}
	serveCtx, cancel := context.WithCancel(ctx)
	transports := make([]endpointTransport, 0, 3)
	cleanup := func() {
		cancel()
		for _, transport := range transports {
			if transport.mux != nil {
				_ = transport.mux.Close()
			}
			if transport.conn != nil {
				_ = transport.conn.Close()
			}
			if transport.child != nil {
				_ = transport.child.Close()
			}
		}
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

	resourceMux := srpc.NewMux(&serviceFilter{serviceID: resource.SRPCResourceServiceServiceID, invoker: srpc.NewClientInvoker(c.ResourceClient)})
	resourceTransport, err := makeTransport("resource", resourceMux)
	if err != nil {
		cleanup()
		return nil, err
	}
	transports = append(transports, resourceTransport)

	stateMux := srpc.NewMux()
	if err := native.SRPCRegisterStateService(stateMux, newStateService(c.StateStore, c.SelectedStateKey)); err != nil {
		cleanup()
		return nil, err
	}
	stateTransport, err := makeTransport("state", stateMux)
	if err != nil {
		cleanup()
		return nil, err
	}
	transports = append(transports, stateTransport)

	controlMux := srpc.NewMux(&serviceFilter{serviceID: native.SRPCControlServiceServiceID, invoker: srpc.NewClientInvoker(c.ResourceClient)})
	controlTransport, err := makeTransport("control", controlMux)
	if err != nil {
		cleanup()
		return nil, err
	}
	transports = append(transports, controlTransport)

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

	var closeOnce sync.Once
	var waitOnce sync.Once
	var waitErr error
	closeFunc := func() error {
		closeOnce.Do(func() { cleanup() })
		return nil
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

type serviceFilter struct {
	serviceID string
	invoker   srpc.Invoker
}

func (f *serviceFilter) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	if serviceID != f.serviceID {
		return false, nil
	}
	return f.invoker.InvokeMethod(serviceID, methodID, stream)
}

var _ srpc.Invoker = (*serviceFilter)(nil)
