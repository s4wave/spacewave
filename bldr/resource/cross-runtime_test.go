package resource_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
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
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

const crossRuntimeFixtureTimeout = 20 * time.Second

type crossRuntimeProcess struct {
	cmd   *exec.Cmd
	input io.WriteCloser

	outputMtx sync.Mutex
	output    []string
	stderr    bytes.Buffer
	lines     chan string
	done      chan struct{}
	err       error
}

func startCrossRuntimeProcess(
	t *testing.T,
	ctx context.Context,
	name string,
	args ...string,
) *crossRuntimeProcess {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = root
	command.Env = os.Environ()
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("%s stdin: %v", name, err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("%s stdout: %v", name, err)
	}
	process := &crossRuntimeProcess{
		cmd:   command,
		input: input,
		lines: make(chan string, 64),
		done:  make(chan struct{}),
	}
	command.Stderr = &process.stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			process.outputMtx.Lock()
			process.output = append(process.output, line)
			process.outputMtx.Unlock()
			process.lines <- line
		}
	}()
	go func() {
		process.err = command.Wait()
		<-scanDone
		close(process.done)
	}()
	return process
}

func (p *crossRuntimeProcess) send(t *testing.T, command string) {
	t.Helper()
	if _, err := io.WriteString(p.input, command+"\n"); err != nil {
		t.Fatalf("send %q to %s: %v\n%s", command, p.cmd.Path, err, p.logs())
	}
}

func (p *crossRuntimeProcess) waitLine(
	t *testing.T,
	ctx context.Context,
	want string,
) string {
	t.Helper()
	for {
		select {
		case line := <-p.lines:
			if line == want || strings.HasPrefix(line, want) {
				return line
			}
		case <-p.done:
			t.Fatalf("%s exited before %q: %v\n%s", p.cmd.Path, want, p.err, p.logs())
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %q from %s\n%s", want, p.cmd.Path, p.logs())
		}
	}
}

func (p *crossRuntimeProcess) waitExit(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-p.done:
		if p.err != nil {
			t.Fatalf("%s failed: %v\n%s", p.cmd.Path, p.err, p.logs())
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s\n%s", p.cmd.Path, p.logs())
	}
}

func (p *crossRuntimeProcess) logs() string {
	p.outputMtx.Lock()
	defer p.outputMtx.Unlock()
	logs := strings.Join(p.output, "\n")
	if p.stderr.Len() != 0 {
		logs += "\n" + p.stderr.String()
	}
	return logs
}

func TestPythonClientResourceLifecycleAgainstTypeScriptServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), crossRuntimeFixtureTimeout)
	defer cancel()
	server := startCrossRuntimeProcess(
		t,
		ctx,
		"bun",
		"run",
		"./bldr/resource/testdata/cross-runtime-ts-core-server.ts",
		"127.0.0.1:0",
	)
	ready := server.waitLine(t, ctx, "READY ")
	address := strings.TrimPrefix(ready, "READY ")
	client := startCrossRuntimeProcess(
		t,
		ctx,
		"uv",
		"run",
		"python",
		"bldr/resource/testdata/cross-runtime-python-client.py",
		address,
	)

	server.waitLine(t, ctx, "ADOPT_ACK_HELD")
	server.send(t, "ALLOW_ADOPT")
	server.waitLine(t, ctx, "HANDLER_FINALLY")
	server.waitLine(t, ctx, "RELEASE_COMPLETE")
	server.waitLine(t, ctx, "HANDLER_FINALLY")
	client.waitLine(t, ctx, "PY_CLIENT_READY_TO_INVALIDATE")
	server.send(t, "INVALIDATE")
	server.waitLine(t, ctx, "TS_SERVER_OWNER_ZERO")
	client.waitLine(t, ctx, "PY_CLIENT_OWNER_ZERO")
	client.waitExit(t, ctx)
	server.waitExit(t, ctx)
}

func TestTypeScriptClientResourceLifecycleAgainstPythonServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), crossRuntimeFixtureTimeout)
	defer cancel()
	server := startCrossRuntimeProcess(
		t,
		ctx,
		"uv",
		"run",
		"python",
		"bldr/resource/testdata/cross-runtime-python-server.py",
		"--listen",
		"127.0.0.1:0",
	)
	ready := server.waitLine(t, ctx, "READY ")
	client := startCrossRuntimeProcess(
		t,
		ctx,
		"bun",
		"run",
		"./bldr/resource/testdata/cross-runtime-ts-core-client.ts",
		strings.TrimPrefix(ready, "READY "),
	)

	server.waitLine(t, ctx, "ADOPT_ACK_HELD")
	server.send(t, "ALLOW_ADOPT")
	server.waitLine(t, ctx, "HANDLER_FINALLY")
	server.waitLine(t, ctx, "HANDLER_FINALLY")
	server.waitLine(t, ctx, "RELEASE_ENTERED")
	server.send(t, "ALLOW_RELEASE")
	server.waitLine(t, ctx, "RELEASE_COMPLETE")
	client.waitLine(t, ctx, "TS_CLIENT_READY_TO_INVALIDATE")
	server.send(t, "INVALIDATE")
	server.waitLine(t, ctx, "PY_SERVER_OWNER_ZERO")
	client.waitLine(t, ctx, "TS_CLIENT_OWNER_ZERO")
	client.waitExit(t, ctx)
	server.waitExit(t, ctx)
}

func TestGoClientResourceLifecycleAgainstPythonServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), crossRuntimeFixtureTimeout)
	defer cancel()
	server := startCrossRuntimeProcess(
		t,
		ctx,
		"uv",
		"run",
		"python",
		"bldr/resource/testdata/cross-runtime-python-server.py",
		"--listen",
		"127.0.0.1:0",
	)
	ready := server.waitLine(t, ctx, "READY ")
	service := resource.NewSRPCResourceServiceClient(
		newCrossRuntimeTCPClient(strings.TrimPrefix(ready, "READY ")),
	)
	client, err := resource_client.NewClient(ctx, service)
	if err != nil {
		t.Fatalf("start Go ResourceClient: %v", err)
	}
	root := client.AccessRootResource()
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatalf("get Go root client: %v", err)
	}
	spawnResult := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, callErr := crossRuntimeCall(
			ctx,
			rootClient,
			"test.Root",
			"Spawn",
			[]byte("spawn"),
		)
		spawnResult <- struct {
			data []byte
			err  error
		}{data, callErr}
	}()

	server.waitLine(t, ctx, "ADOPT_ACK_HELD")
	select {
	case result := <-spawnResult:
		t.Fatalf("Go nested route opened before delayed acknowledgement: %q/%v", result.data, result.err)
	default:
	}
	server.send(t, "ALLOW_ADOPT")
	var childData []byte
	select {
	case result := <-spawnResult:
		if result.err != nil {
			t.Fatalf("spawn Python child: %v", result.err)
		}
		childData = result.data
	case <-ctx.Done():
		t.Fatal("timed out opening Go root route after acknowledgement")
	}
	if len(childData) != 4 {
		t.Fatalf("child ID length = %d, want 4", len(childData))
	}
	childID := binary.BigEndian.Uint32(childData)
	if childID == 0 {
		t.Fatal("Spawn returned an empty child ID")
	}
	child := client.CreateResourceReference(childID)
	childClient, err := child.GetClient()
	if err != nil {
		t.Fatalf("get child client: %v", err)
	}
	data, err := crossRuntimeCall(ctx, childClient, "test.Child", "Stream", []byte("stream"))
	if err != nil {
		t.Fatalf("stream Python child: %v", err)
	}
	if string(data) != "later" {
		t.Fatalf("stream data = %q, want later", data)
	}

	canceled, err := childClient.NewStream(
		ctx,
		"test.Child",
		"Block",
		srpc.NewRawMessage([]byte("block"), true),
	)
	if err != nil {
		t.Fatalf("open cancellable child stream: %v", err)
	}
	active := srpc.NewRawMessage(nil, true)
	if err := canceled.MsgRecv(active); err != nil {
		t.Fatalf("receive cancellable child data: %v", err)
	}
	if string(active.GetData()) != "active" {
		t.Fatalf("cancellable data = %q, want active", active.GetData())
	}
	if err := canceled.Close(); err != nil {
		t.Fatalf("cancel child stream: %v", err)
	}
	server.waitLine(t, ctx, "HANDLER_FINALLY")

	released, err := childClient.NewStream(
		ctx,
		"test.Child",
		"Block",
		srpc.NewRawMessage([]byte("block"), true),
	)
	if err != nil {
		t.Fatalf("open releasable child stream: %v", err)
	}
	if err := released.MsgRecv(active); err != nil {
		t.Fatalf("receive releasable child data: %v", err)
	}
	child.Release()
	server.waitLine(t, ctx, "HANDLER_FINALLY")
	server.waitLine(t, ctx, "RELEASE_ENTERED")
	echoResult := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, callErr := crossRuntimeCall(
			ctx,
			rootClient,
			"test.Root",
			"Echo",
			[]byte("after-release"),
		)
		echoResult <- struct {
			data []byte
			err  error
		}{data, callErr}
	}()
	select {
	case result := <-echoResult:
		t.Fatalf("root route passed the child release barrier: %q/%v", result.data, result.err)
	default:
	}
	server.send(t, "ALLOW_RELEASE")
	server.waitLine(t, ctx, "RELEASE_COMPLETE")
	select {
	case result := <-echoResult:
		if result.err != nil {
			t.Fatalf("route root after child release: %v", result.err)
		}
		if string(result.data) != "after-release" {
			t.Fatalf("root response = %q, want after-release", result.data)
		}
	case <-ctx.Done():
		t.Fatal("timed out routing root after child release")
	}
	if err := released.Close(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("close released child stream: %v", err)
	}

	root.Release()
	reused := client.AccessRootResource()
	reusedClient, err := reused.GetClient()
	if err != nil {
		t.Fatalf("get retained root client: %v", err)
	}
	data, err = crossRuntimeCall(ctx, reusedClient, "test.Root", "Echo", []byte("reused"))
	if err != nil {
		t.Fatalf("route retained root: %v", err)
	}
	if string(data) != "reused" {
		t.Fatalf("retained root response = %q, want reused", data)
	}
	reused.Release()

	server.send(t, "INVALIDATE")
	select {
	case <-client.Done():
	case <-ctx.Done():
		t.Fatal("timed out invalidating Go ResourceClient generation")
	}
	stale := client.AccessRootResource()
	if _, err := stale.GetClient(); !errors.Is(err, resource.ErrResourceOrClientReleased) {
		t.Fatalf("invalidated root error = %v, want %v", err, resource.ErrResourceOrClientReleased)
	}
	server.waitLine(t, ctx, "PY_SERVER_OWNER_ZERO")
	server.waitExit(t, ctx)
}

func newCrossRuntimeTCPClient(address string) srpc.Client {
	return srpc.NewClient(func(
		ctx context.Context,
		handler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (srpc.PacketWriter, error) {
		connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, err
		}
		packets := srpc.NewPacketReadWriter(connection)
		go packets.ReadPump(handler, closeHandler)
		return packets, nil
	})
}

func crossRuntimeCall(
	ctx context.Context,
	client srpc.Client,
	service string,
	method string,
	data []byte,
) ([]byte, error) {
	response := srpc.NewRawMessage(nil, true)
	err := client.ExecCall(
		ctx,
		service,
		method,
		srpc.NewRawMessage(data, true),
		response,
	)
	return response.GetData(), err
}
