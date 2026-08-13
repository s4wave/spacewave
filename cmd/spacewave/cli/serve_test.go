//go:build !js

package spacewave_cli

import (
	"flag"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

func TestServeCommandTraceFlag(t *testing.T) {
	cmd := newServeCommand(nil)
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
	cmd := newServeCommand(nil)
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
	cmd := newServeCommand(nil)
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
	cmd := newServeCommand(nil)
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

func TestDaemonResourceInvokerUsesResourceServiceLookup(t *testing.T) {
	tb, err := testbed.NewTestbed(t.Context(), logrus.NewEntry(logrus.New()), testbed.TestbedOpts{
		NoEcho: true,
		NoPeer: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	service := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(newDaemonResourceInvoker(tb.Bus)))),
	)
	clientCh := make(chan *resource_client.Client, 1)
	errCh := make(chan error, 1)
	go func() {
		client, err := resource_client.NewClient(t.Context(), service)
		if err != nil {
			errCh <- err
			return
		}
		clientCh <- client
	}()

	select {
	case client := <-clientCh:
		client.Release()
		t.Fatal("Resource client opened before the Resource service was available")
	case err := <-errCh:
		t.Fatalf("Resource client failed before the Resource service was available: %v", err)
	default:
	}

	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(srpc.NewMux()).Register(mux); err != nil {
		t.Fatal(err)
	}
	resourceController := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test/resource-service", controller.MustParseVersion("0.0.1"), "test Resource service"),
		bifrost_rpc.NewRpcServiceBuilder(mux),
		nil,
		false,
		nil,
		[]string{resource.SRPCResourceServiceServiceID},
		nil,
	)
	release, err := tb.Bus.AddController(t.Context(), resourceController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	select {
	case client := <-clientCh:
		client.Release()
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("Resource client did not use the available Resource service")
	}
}
