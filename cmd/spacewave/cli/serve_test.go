//go:build !js

package spacewave_cli

import (
	"context"
	"flag"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

func TestServeCommandTraceFlag(t *testing.T) {
	cmd := newServeCommand(nil, yield_policy.NewBroker())
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if set.Lookup("trace") == nil {
		t.Fatal("trace flag missing")
	}
}

func TestServeCommandIdleTimeoutFlag(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "")
	cmd := newServeCommand(nil, yield_policy.NewBroker())
	idleFlag := findServeIdleTimeoutFlag(t, cmd)

	if idleFlag.Value != defaultDaemonIdleTimeout {
		t.Fatalf("idle-timeout default = %v, want %v", idleFlag.Value, defaultDaemonIdleTimeout)
	}
	if idleFlag.GetDefaultText() != defaultDaemonIdleTimeout.String() {
		t.Fatalf("idle-timeout default text = %q, want %q", idleFlag.GetDefaultText(), defaultDaemonIdleTimeout)
	}
	if !strings.Contains(idleFlag.Usage, "last active client/service") ||
		!strings.Contains(idleFlag.Usage, "zero disables idle shutdown") {
		t.Fatalf("idle-timeout usage does not describe shutdown behavior: %q", idleFlag.Usage)
	}
}

func TestServeCommandIdleTimeoutFlagDefersEnvironmentParsing(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "not-a-duration")
	cmd := newServeCommand(nil, yield_policy.NewBroker())
	idleFlag := findServeIdleTimeoutFlag(t, cmd)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	if err := idleFlag.Apply(set); err != nil {
		t.Fatalf("apply idle-timeout flag: %v", err)
	}

	if idleFlag.Value != defaultDaemonIdleTimeout {
		t.Fatalf("idle-timeout environment value parsed during flag setup = %v, want %v", idleFlag.Value, defaultDaemonIdleTimeout)
	}
}

func TestServeCommandIdleTimeoutFlagOverridesEnvironment(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "45s")
	cmd := newServeCommand(nil, yield_policy.NewBroker())
	idleFlag := findServeIdleTimeoutFlag(t, cmd)
	set := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if err := set.Parse([]string{"--idle-timeout", "0s"}); err != nil {
		t.Fatalf("parse idle-timeout flag: %v", err)
	}
	if idleFlag.Destination == nil {
		t.Fatal("idle-timeout flag destination missing")
	}
	if *idleFlag.Destination != 0 {
		t.Fatalf("idle-timeout destination = %v, want 0", *idleFlag.Destination)
	}
}

func findServeIdleTimeoutFlag(t *testing.T, cmd *cli.Command) *cli.DurationFlag {
	t.Helper()
	for _, fl := range cmd.Flags {
		idleFlag, ok := fl.(*cli.DurationFlag)
		if ok && idleFlag.Name == "idle-timeout" {
			return idleFlag
		}
	}
	t.Fatal("idle-timeout flag missing")
	return nil
}

type daemonTestReference struct {
	once     sync.Once
	released chan struct{}
}

func (r *daemonTestReference) Release() {
	r.once.Do(func() { close(r.released) })
}

func newDaemonTestResourceClient(t *testing.T) srpc.Client {
	t.Helper()
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(srpc.NewMux()).Register(mux); err != nil {
		t.Fatal(err)
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
}

func TestDaemonResourceInvokerRoutesOnlyResourceService(t *testing.T) {
	loadCalled := false
	invoker := &daemonResourceInvoker{loadClient: func(context.Context) (srpc.Client, directive.Reference, error) {
		loadCalled = true
		return nil, nil, nil
	}}
	found, err := invoker.InvokeMethod("other.Service", "Method", nil)
	if err != nil {
		t.Fatal(err)
	}
	if found || loadCalled {
		t.Fatal("daemon forwarded a non-Resource service to spacewave-core")
	}
}

func TestDaemonResourceStreamWaitsForCurrentCoreGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loadStarted := make(chan int, 2)
	generationReady := []chan struct{}{make(chan struct{}), make(chan struct{})}
	clients := []srpc.Client{newDaemonTestResourceClient(t), newDaemonTestResourceClient(t)}
	references := []*daemonTestReference{
		{released: make(chan struct{})},
		{released: make(chan struct{})},
	}
	var loadMtx sync.Mutex
	loadCount := 0
	invoker := &daemonResourceInvoker{loadClient: func(loadCtx context.Context) (srpc.Client, directive.Reference, error) {
		loadMtx.Lock()
		generation := loadCount
		loadCount++
		loadMtx.Unlock()
		loadStarted <- generation
		select {
		case <-loadCtx.Done():
			return nil, nil, loadCtx.Err()
		case <-generationReady[generation]:
			return clients[generation], references[generation], nil
		}
	}}

	open := func() <-chan *resource_client.Client {
		clientCh := make(chan *resource_client.Client, 1)
		service := resource.NewSRPCResourceServiceClient(
			srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker))),
		)
		go func() {
			client, err := resource_client.NewClient(ctx, service)
			if err != nil {
				t.Errorf("open Resource client: %v", err)
				clientCh <- nil
				return
			}
			clientCh <- client
		}()
		return clientCh
	}

	firstCh := open()
	if generation := <-loadStarted; generation != 0 {
		t.Fatalf("first lookup generation = %d, want 0", generation)
	}
	select {
	case client := <-firstCh:
		if client != nil {
			client.Release()
		}
		t.Fatal("Resource client opened before its plugin generation was released")
	default:
	}
	close(generationReady[0])
	first := <-firstCh
	if first == nil {
		t.Fatal("first Resource generation did not open")
	}
	first.Release()
	select {
	case <-references[0].released:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	secondCh := open()
	if generation := <-loadStarted; generation != 1 {
		t.Fatalf("second lookup generation = %d, want 1", generation)
	}
	close(generationReady[1])
	second := <-secondCh
	if second == nil {
		t.Fatal("replacement Resource generation did not open")
	}
	second.Release()
	select {
	case <-references[1].released:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	loadMtx.Lock()
	defer loadMtx.Unlock()
	if loadCount != 2 {
		t.Fatalf("plugin generation lookups = %d, want 2", loadCount)
	}
}

func TestDaemonResourceStreamCancellationStopsPluginWait(t *testing.T) {
	loadStarted := make(chan struct{})
	loadExited := make(chan struct{})
	invoker := &daemonResourceInvoker{loadClient: func(ctx context.Context) (srpc.Client, directive.Reference, error) {
		close(loadStarted)
		<-ctx.Done()
		close(loadExited)
		return nil, nil, ctx.Err()
	}}
	service := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker))),
	)
	clientCtx, cancelClient := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := resource_client.NewClient(clientCtx, service)
		errCh <- err
	}()
	<-loadStarted
	cancelClient()

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("Resource client error = %v, want context canceled", err)
		}
		select {
		case <-loadExited:
		case <-time.After(5 * time.Second):
			t.Fatal("plugin wait remained active after Resource stream cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Resource client did not cancel its spacewave-core wait")
	}
}
