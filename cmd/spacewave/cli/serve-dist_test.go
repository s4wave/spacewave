//go:build !js

package spacewave_cli

import (
	"context"
	"flag"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	"github.com/sirupsen/logrus"
)

// distCliBus is a Dist CLI bus: core runs in a plugin, so the bus has no
// root resource controller factory.
type distCliBus struct {
	cli_entrypoint.CliBus
	ctx context.Context
	b   bus.Bus
	le  *logrus.Entry
}

func (d *distCliBus) GetContext() context.Context    { return d.ctx }
func (d *distCliBus) GetBus() bus.Bus                { return d.b }
func (d *distCliBus) GetLogger() *logrus.Entry       { return d.le }
func (d *distCliBus) GetPluginHostObjectKey() string { return "spacewave-plugin-host" }
func (d *distCliBus) AddRelease(func())              {}

// A Dist daemon serves its socket without the native root resource controller.
func TestRunServeCommandDistAcceptsConnections(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	cliBus := &distCliBus{ctx: ctx, b: b, le: le}

	statePath := shortStatePath(t)
	app := cli.NewApp()
	parentFlags := flag.NewFlagSet("spacewave", flag.ContinueOnError)
	parentFlags.String("state-path", statePath, "state directory path")
	if err := parentFlags.Parse([]string{"--state-path", statePath}); err != nil {
		t.Fatal(err)
	}
	parent := cli.NewContext(app, parentFlags, nil)
	child := cli.NewContext(app, flag.NewFlagSet("serve", flag.ContinueOnError), parent)
	child.Context = ctx

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- runServeCommand(child, func() cli_entrypoint.CliBus {
			return cliBus
		}, yield_policy.NewBroker(), "", false, 0)
	}()

	// The socket appears once serve binds it; before the fix it bound but
	// never accepted, so the shutdown request below timed out.
	sockPath := filepath.Join(statePath, socketName)
	var conn net.Conn
	for {
		conn, err = connectDaemonDial(ctx, sockPath)
		if err == nil {
			break
		}
		select {
		case err := <-serveErr:
			t.Fatalf("serve exited before accepting: %v", err)
		case <-ctx.Done():
			t.Fatalf("connect to daemon: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer conn.Close()
	if err := requestDaemonShutdown(ctx, conn); err != nil {
		t.Fatalf("request shutdown: %v", err)
	}
	_ = conn.Close()
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("serve did not exit after shutdown")
	}
}
