package resource_session_test

import (
	"bufio"
	"bytes"
	"context"
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
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

const (
	// pythonSessionJourneySessionIdx is the Session index the maintained
	// TypeScript Unix story mounts, reused here so both stories read alike.
	pythonSessionJourneySessionIdx = 4
	pythonSessionJourneySpaceName  = "Terminal"
	pythonSessionJourneySpaceID    = "terminal-id"
)

// TestPythonSessionJourneyAgainstGoSessionOwner drives the Python Root and
// Session wrappers over TCP against a real Go SessionResource, then requires
// the Session watch handler, the child release callback, the Python client,
// and the Go generation to settle before the listener closes.
func TestPythonSessionJourneyAgainstGoSessionOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	// Build the real Go Session owner and one accessible Space to list.
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)
	env.createSpaceOnAccount(ctx, t, account, pythonSessionJourneySpaceName)
	sessResource := env.buildSessionResource(ctx, t, sessRef)
	t.Cleanup(sessResource.Close)

	// Record when the delegated Session watch handler leaves the Go owner.
	watchExited := make(chan struct{}, 1)
	sessionOwnerMux := sessResource.GetMux()
	sessionMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		handled, err := sessionOwnerMux.InvokeMethod(serviceID, methodID, strm)
		if serviceID == s4wave_session.SRPCSessionResourceServiceServiceID &&
			methodID == "WatchResourcesList" {
			select {
			case watchExited <- struct{}{}:
			default:
			}
		}
		return handled, err
	})

	// Serve one bounded Root that mounts that Session owner as a child.
	releaseComplete := make(chan struct{}, 1)
	rootMux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != s4wave_root.SRPCRootResourceServiceServiceID ||
			methodID != "MountSessionByIdx" {
			return false, nil
		}
		request := new(s4wave_root.MountSessionByIdxRequest)
		if err := strm.MsgRecv(request); err != nil {
			return true, err
		}
		if request.GetSessionIdx() != pythonSessionJourneySessionIdx {
			return true, strm.MsgSend(&s4wave_root.MountSessionByIdxResponse{NotFound: true})
		}
		owner, err := resource_server.MustGetResourceClientContext(strm.Context())
		if err != nil {
			return true, err
		}
		childID, err := owner.AddResource(sessionMux, func() {
			releaseComplete <- struct{}{}
		})
		if err != nil {
			return true, err
		}
		return true, strm.MsgSend(&s4wave_root.MountSessionByIdxResponse{ResourceId: childID})
	}))

	// Register the Resource service and observe generation teardown. The
	// ResourceClient handler removes its client and every resource it retained
	// before it returns, so its completion is the Go owner-zero observation.
	server := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := server.Register(resourceMux); err != nil {
		t.Fatalf("register ResourceServer: %v", err)
	}
	generationDone := make(chan struct{}, 1)
	observedMux := srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		handled, err := resourceMux.InvokeMethod(serviceID, methodID, strm)
		if serviceID == resource.SRPCResourceServiceServiceID && methodID == "ResourceClient" {
			select {
			case generationDone <- struct{}{}:
			default:
			}
		}
		return handled, err
	})
	srpcServer := srpc.NewServer(observedMux)

	// Serve one StarPC RPC per accepted TCP connection.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	acceptDone := make(chan struct{})
	var handlers sync.WaitGroup
	connections := make(map[net.Conn]struct{})
	var connectionsMtx sync.Mutex
	go func() {
		defer close(acceptDone)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			connectionsMtx.Lock()
			connections[conn] = struct{}{}
			connectionsMtx.Unlock()
			handlers.Go(func() {
				defer func() {
					connectionsMtx.Lock()
					delete(connections, conn)
					connectionsMtx.Unlock()
					_ = conn.Close()
				}()
				srpcServer.HandleStream(ctx, conn)
			})
		}
	}()
	defer func() {
		_ = listener.Close()
		<-acceptDone
		cancel()
		connectionsMtx.Lock()
		remaining := make([]net.Conn, 0, len(connections))
		for conn := range connections {
			remaining = append(remaining, conn)
		}
		connectionsMtx.Unlock()
		for _, conn := range remaining {
			_ = conn.Close()
		}
		handlers.Wait()
	}()

	// Run the Python journey against that endpoint.
	repositoryRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	command := exec.CommandContext(
		ctx,
		"uv",
		"run",
		"python",
		"bldr/resource/testdata/python-session-journey-client.py",
		listener.Addr().String(),
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
	var outputMtx sync.Mutex
	var outputLines []string
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			outputMtx.Lock()
			outputLines = append(outputLines, scanner.Text())
			outputMtx.Unlock()
		}
	}()
	outputText := func() string {
		outputMtx.Lock()
		defer outputMtx.Unlock()
		return strings.Join(outputLines, "\n")
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("python client failed: %v\n%s%s", err, outputText(), stderr.String())
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Python client\n%s%s", outputText(), stderr.String())
	}
	<-scanDone

	// Require the Space projection the Go Session owner listed.
	output := outputText()
	for _, want := range []string{
		"PY_SPACE_NAME " + pythonSessionJourneySpaceName,
		"PY_SPACE_ID " + pythonSessionJourneySpaceID,
		"PY_CLIENT_OWNER_ZERO",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("python client did not report %q\n%s%s", want, output, stderr.String())
		}
	}

	// Require the Go watch handler, release callback, and generation to settle.
	for _, settled := range []struct {
		name string
		ch   chan struct{}
	}{
		{"Session watch handler", watchExited},
		{"child release callback", releaseComplete},
		{"ResourceClient generation", generationDone},
	} {
		select {
		case <-settled.ch:
		case <-ctx.Done():
			t.Fatalf("%s did not complete\n%s%s", settled.name, output, stderr.String())
		}
	}
}
