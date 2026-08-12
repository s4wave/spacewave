package resource_server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

const (
	pythonCrossRuntimeRootService  = "test.Root"
	pythonCrossRuntimeChildService = "test.Child"
)

// controlAckGateConn holds the first ResourceClient control acknowledgement
// until the test proves no nested route reached the selected root.
type controlAckGateConn struct {
	net.Conn

	once    sync.Once
	held    chan struct{}
	release chan struct{}
	holdErr error
	holdMtx sync.Mutex
}

func newControlAckGateConn(conn net.Conn) *controlAckGateConn {
	return &controlAckGateConn{
		Conn:    conn,
		held:    make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (c *controlAckGateConn) Write(data []byte) (int, error) {
	if c.isControlAck(data) {
		c.once.Do(func() {
			close(c.held)
			<-c.release
		})
	}
	return c.Conn.Write(data)
}

func (c *controlAckGateConn) isControlAck(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	size := binary.LittleEndian.Uint32(data[:4])
	if int(size) != len(data)-4 {
		return false
	}
	packet := new(srpc.Packet)
	if err := packet.UnmarshalVT(data[4:]); err != nil {
		c.holdMtx.Lock()
		c.holdErr = err
		c.holdMtx.Unlock()
		return false
	}
	callData := packet.GetCallData()
	if callData == nil {
		return false
	}
	response := new(resource.ResourceClientResponse)
	if err := response.UnmarshalVT(callData.Data); err != nil {
		return false
	}
	return response.GetControlAck() != nil
}

func (c *controlAckGateConn) err() error {
	c.holdMtx.Lock()
	defer c.holdMtx.Unlock()
	return c.holdErr
}

func TestPythonClientResourceLifecycleAgainstGoServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	rootEntered := make(chan struct{}, 1)
	handlerFinally := make(chan struct{}, 2)
	releaseEntered := make(chan struct{}, 1)
	releaseComplete := make(chan struct{}, 1)
	releaseGate := make(chan struct{})
	rootMux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		switch {
		case serviceID == pythonCrossRuntimeRootService && methodID == "Spawn":
			select {
			case rootEntered <- struct{}{}:
			default:
			}
			request := srpc.NewRawMessage(nil, true)
			if err := strm.MsgRecv(request); err != nil {
				return true, err
			}
			if string(request.GetData()) != "spawn" {
				t.Errorf("Spawn request = %q, want spawn", request.GetData())
			}
			owner, err := MustGetResourceClientContext(strm.Context())
			if err != nil {
				return true, err
			}
			childID, err := owner.AddResource(srpc.NewMux(srpc.InvokerFunc(func(childService, childMethod string, child srpc.Stream) (bool, error) {
				switch {
				case childService == pythonCrossRuntimeChildService && childMethod == "Echo":
					request := srpc.NewRawMessage(nil, true)
					if err := child.MsgRecv(request); err != nil {
						return true, err
					}
					return true, child.MsgSend(request)
				case childService == pythonCrossRuntimeChildService && childMethod == "Stream":
					request := srpc.NewRawMessage(nil, true)
					if err := child.MsgRecv(request); err != nil {
						return true, err
					}
					if string(request.GetData()) != "stream" {
						t.Errorf("Stream request = %q, want stream", request.GetData())
					}
					return true, child.MsgSend(srpc.NewRawMessage([]byte("later"), true))
				case childService == pythonCrossRuntimeChildService && childMethod == "Block":
					defer func() { handlerFinally <- struct{}{} }()
					request := srpc.NewRawMessage(nil, true)
					if err := child.MsgRecv(request); err != nil {
						return true, err
					}
					if string(request.GetData()) != "block" {
						t.Errorf("Block request = %q, want block", request.GetData())
					}
					if err := child.MsgSend(srpc.NewRawMessage([]byte("active"), true)); err != nil {
						return true, err
					}
					<-child.Context().Done()
					return true, child.Context().Err()
				}
				return false, nil
			})), func() {
				releaseEntered <- struct{}{}
				<-releaseGate
				releaseComplete <- struct{}{}
			})
			if err != nil {
				return true, err
			}
			response := make([]byte, 4)
			binary.BigEndian.PutUint32(response, childID)
			return true, strm.MsgSend(srpc.NewRawMessage(response, true))
		case serviceID == pythonCrossRuntimeRootService && methodID == "Echo":
			request := srpc.NewRawMessage(nil, true)
			if err := strm.MsgRecv(request); err != nil {
				return true, err
			}
			return true, strm.MsgSend(request)
		}
		return false, nil
	}))
	server := NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := server.Register(resourceMux); err != nil {
		t.Fatalf("register ResourceServer: %v", err)
	}
	srpcServer := srpc.NewServer(resourceMux)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gates := make(chan *controlAckGateConn, 1)
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			gate := newControlAckGateConn(conn)
			select {
			case gates <- gate:
			default:
			}
			go srpcServer.HandleStream(ctx, gate)
		}
	}()
	defer func() {
		_ = listener.Close()
		select {
		case <-acceptDone:
		case <-ctx.Done():
			t.Error("Go ResourceServer accept loop did not exit")
		}
	}()

	repositoryRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	command := exec.CommandContext(
		ctx,
		"uv",
		"run",
		"python",
		"bldr/resource/testdata/cross-runtime-python-client.py",
		listener.Addr().String(),
		"--finish",
		"close",
	)
	command.Dir = repositoryRoot
	command.Env = os.Environ()
	var stderr bytes.Buffer
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("python client stdout pipe: %v", err)
	}
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start python client: %v", err)
	}
	output := make(chan string, 64)
	var outputMtx sync.Mutex
	var outputLines []string
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputMtx.Lock()
			outputLines = append(outputLines, line)
			outputMtx.Unlock()
			output <- line
		}
	}()
	outputText := func() string {
		outputMtx.Lock()
		defer outputMtx.Unlock()
		return strings.Join(outputLines, "\n")
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	var gate *controlAckGateConn
	select {
	case gate = <-gates:
	case err := <-wait:
		t.Fatalf("python client exited before opening ResourceClient: %v\n%s%s", err, outputText(), stderr.String())
	case <-ctx.Done():
		t.Fatal("timed out waiting for Python client control stream")
	}
	select {
	case <-gate.held:
	case err := <-wait:
		t.Fatalf("python client exited before delayed acknowledgement: %v\n%s%s", err, outputText(), stderr.String())
	case <-ctx.Done():
		t.Fatal("timed out waiting for delayed ResourceClient acknowledgement")
	}
	select {
	case <-rootEntered:
		t.Fatal("Python nested route opened before delayed Adopt acknowledgement")
	default:
	}
	close(gate.release)

	select {
	case <-releaseEntered:
	case err := <-wait:
		t.Fatalf("python client exited before child release barrier: %v\n%s%s", err, outputText(), stderr.String())
	case <-ctx.Done():
		t.Fatal("timed out waiting for child release callback")
	}
	for range 2 {
		select {
		case <-handlerFinally:
		case <-ctx.Done():
			t.Fatal("active child handler did not finish before release callback")
		}
	}
	close(releaseGate)
	select {
	case <-releaseComplete:
	case <-ctx.Done():
		t.Fatal("child release callback did not complete")
	}

	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("python client failed: %v\n%s%s", err, outputText(), stderr.String())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Python client")
	}
	<-scanDone
	if !strings.Contains(outputText(), "PY_CLIENT_OWNER_ZERO") {
		t.Fatalf("Python client did not report owner zero\n%s", outputText())
	}
	if err := gate.err(); err != nil {
		t.Fatalf("decode delayed control acknowledgement: %v", err)
	}
	waitGoServerZero(t, ctx, server)
}

func waitGoServerZero(t *testing.T, ctx context.Context, server *ResourceServer) {
	t.Helper()
	for {
		var clients int
		var resources int
		var waitCh <-chan struct{}
		server.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			clients = len(server.clients)
			resources = server.countTrackedResourcesLocked()
			waitCh = getWaitCh()
		})
		if clients == 0 && resources == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Go ResourceServer owner state = clients:%d resources:%d", clients, resources)
		case <-waitCh:
		}
	}
}
