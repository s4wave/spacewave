//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/bldr/resource"
	"github.com/s4wave/spacewave/bldr/tui/nativehost"
	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

var socketPair = func() ([2]int, error) {
	return syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
}
var serveEndpoint = func(ctx context.Context, _ int, invoker srpc.Invoker, mux srpc.MuxedConn) error {
	return srpc.NewServer(invoker).AcceptMuxedConn(ctx, mux)
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

// endpointServerState publishes fixed-member completion and aggregate errors.
type endpointServerState struct {
	// bcast guards exited and err.
	bcast broadcast.Broadcast
	// exited records completed server keys.
	exited map[int]bool
	// err joins unexpected server failures.
	err error
}

// endpointCleanupState publishes idempotent transport cleanup completion.
type endpointCleanupState struct {
	// bcast guards running, done, and err.
	bcast broadcast.Broadcast
	// running reports transport cleanup in progress.
	running bool
	// done reports transport cleanup completion.
	done bool
	// err records transport cleanup failures.
	err error
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
	cleanupPartial := func() error {
		closeErr := cleanup()
		for _, transport := range transports {
			if transport.child != nil {
				closeErr = errors.Join(closeErr, transport.child.Close())
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
		return nil, errors.Join(err, cleanupPartial())
	}
	transports = append(transports, resourceTransport)

	stateMux := srpc.NewMux()
	if err := native.SRPCRegisterStateService(stateMux, newStateService(c.StateStore, c.SelectedStateKey)); err != nil {
		return nil, errors.Join(err, cleanupPartial())
	}
	stateTransport, err := makeTransport("state", stateMux)
	if err != nil {
		return nil, errors.Join(err, cleanupPartial())
	}
	transports = append(transports, stateTransport)

	controlMux := srpc.NewMux()
	if err := native.SRPCRegisterControlService(controlMux, newControlBridge(
		native.NewSRPCControlServiceClient(c.ResourceClient), c.CommandRegistryClient,
	)); err != nil {
		return nil, errors.Join(err, cleanupPartial())
	}
	controlTransport, err := makeTransport("control", controlMux)
	if err != nil {
		return nil, errors.Join(err, cleanupPartial())
	}
	transports = append(transports, controlTransport)

	// Serve every parent transport and aggregate failures until shutdown.
	// Serve the fixed endpoint member set through keyed lifecycle custody.
	serverState := &endpointServerState{exited: make(map[int]bool, len(transports))}
	servers := keyed.NewKeyed(func(key int) (keyed.Routine, struct{}) {
		transport := transports[key]
		return func(ctx context.Context) error {
			err := serveEndpoint(ctx, key, transport.invoker, transport.mux)
			if ctx.Err() != nil {
				return nil
			}
			return err
		}, struct{}{}
	}, keyed.WithExitCb(func(key int, _ keyed.Routine, _ struct{}, err error) {
		locked := serverState.bcast.Lock()
		serverState.exited[key] = true
		serverState.err = errors.Join(serverState.err, err)
		locked.Broadcast()
		locked.Unlock()
	}))
	servers.SetContext(serveCtx, false)
	servers.SyncKeys([]int{0, 1, 2}, false)

	// Expose broadcast-guarded transport close and member completion states.
	cleanupState := new(endpointCleanupState)
	closeFunc := func() error {
		for {
			locked := cleanupState.bcast.Lock()
			if cleanupState.done {
				err := cleanupState.err
				locked.Unlock()
				return err
			}
			if cleanupState.running {
				wait := locked.WaitCh()
				locked.Unlock()
				<-wait
				continue
			}
			cleanupState.running = true
			locked.Broadcast()
			locked.Unlock()

			err := cleanup()
			locked = cleanupState.bcast.Lock()
			cleanupState.running = false
			cleanupState.done = true
			cleanupState.err = err
			locked.Broadcast()
			locked.Unlock()
		}
	}
	waitFunc := func() error {
		for {
			locked := serverState.bcast.Lock()
			if len(serverState.exited) == len(transports) {
				err := serverState.err
				locked.Unlock()
				servers.ClearContext()
				return err
			}
			wait := locked.WaitCh()
			locked.Unlock()
			<-wait
		}
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
