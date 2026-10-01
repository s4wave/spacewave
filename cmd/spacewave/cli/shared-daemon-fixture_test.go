//go:build !js

package spacewave_cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// sharedDaemonFixtureMode selects the native or forwarded core in this test binary.
const sharedDaemonFixtureMode = "SPACEWAVE_TEST_DAEMON_MODE"

// sharedDaemonUpdateTarget selects the controlled CLI copy in the relaunch fixture.
const sharedDaemonUpdateTarget = "SPACEWAVE_TEST_DAEMON_UPDATE_TARGET"

// sharedDaemonCorruptOnAccept changes selected fixture bytes after acceptance.
const sharedDaemonCorruptOnAccept = "SPACEWAVE_TEST_DAEMON_CORRUPT_ON_ACCEPT"

// sharedDaemonFailSelected holds an unready selected fixture until startup stops it.
const sharedDaemonFailSelected = "SPACEWAVE_TEST_DAEMON_FAIL_SELECTED"

// sharedDaemonFixtureBus keeps a real writable CLI bus while selecting the
// distribution serve branch, whose Resource RPCs load the core plugin.
type sharedDaemonFixtureBus struct {
	cli_entrypoint.CliBus
}

// GetPluginHostObjectKey selects distribution Resource forwarding.
func (b *sharedDaemonFixtureBus) GetPluginHostObjectKey() string { return "fixture-host" }

// TestMain permits the shared launcher to run this binary's actual serve command
// in a detached child, with all writable data constrained to the test root.
func TestMain(m *testing.M) {
	if os.Getenv(sharedDaemonFixtureMode) != "" && len(os.Args) > 1 && os.Args[1] == "--state-path" {
		if err := runSharedDaemonFixture(); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runSharedDaemonFixture runs production serve with a durable StateAtom root.
// Distribution mode uses the ordinary LoadPlugin Resource forwarding boundary;
// the fixture core implementation is in-process to avoid building UI artifacts.
func runSharedDaemonFixture() error {
	// Retain the same signal and bus release boundary as a native executable.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Read the state path and optional failure injection flag.
	statePath := os.Args[2]
	if os.Getenv(sharedDaemonFailSelected) != "" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if strings.HasSuffix(executable, ".sh") {
			if err := os.WriteFile(filepath.Join(statePath, "failed-new-pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}
	}

	// Build the discarded logger and the CLI bus holder.
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	le := logrus.NewEntry(logger)
	var cliBus *cli_entrypoint.CliBusImpl
	var buildErr error
	if os.Getenv(sharedDaemonUpdateTarget) != "" {
		logFile, err := os.Create(filepath.Join(statePath, "fixture-"+strconv.Itoa(os.Getpid())+".log"))
		if err != nil {
			return err
		}
		defer logFile.Close()
		logger.SetOutput(logFile)
	}
	defer func() {
		if cliBus != nil {
			cliBus.Release()
			_ = os.WriteFile(filepath.Join(statePath, "stopped"), nil, 0o600)
			_ = os.WriteFile(filepath.Join(statePath, "stopped-"+strconv.Itoa(os.Getpid())), nil, 0o600)
		}
	}()

	// getBus is called only after production serve acquires the state lease.
	getBus := func() cli_entrypoint.CliBus {
		// Write the runtime identity marker with the process ID.
		identity := strconv.Itoa(os.Getpid())
		markerFlags := os.O_CREATE | os.O_EXCL | os.O_WRONLY
		if os.Getenv(sharedDaemonUpdateTarget) != "" {
			markerFlags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		}
		marker, err := os.OpenFile(filepath.Join(statePath, "runtime-identity"), markerFlags, 0o600)
		if err != nil {
			buildErr = err
			return nil
		}
		_, buildErr = marker.WriteString(identity)
		_ = marker.Close()
		if buildErr != nil {
			return nil
		}

		// Record the runtime executable path and build the CLI bus.
		executable, err := os.Executable()
		if err != nil {
			buildErr = err
			return nil
		}
		if buildErr = os.WriteFile(filepath.Join(statePath, "runtime-executable"), []byte(executable), 0o600); buildErr != nil {
			return nil
		}
		cliBus, buildErr = cli_entrypoint.BuildCliBus(ctx, le, "spacewave", statePath)
		if buildErr != nil {
			return nil
		}
		for _, factory := range plugin_host_default.PluginHostControllerFactories {
			cliBus.GetStaticResolver().AddFactory(factory(cliBus.GetBus()))
		}
		if os.Getenv(sharedDaemonFixtureMode) == "distribution" {
			pluginStateRoot := filepath.Join(statePath, "plugin", "state")
			pluginDistRoot := filepath.Join(statePath, "plugin", "dist")
			for _, dir := range []string{pluginStateRoot, pluginDistRoot} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					buildErr = err
					return nil
				}
			}
			_, releaseHost, err := plugin_host_default.StartPluginHost(ctx, cliBus.GetBus(), pluginStateRoot, pluginDistRoot, "")
			if err != nil {
				buildErr = err
				return nil
			}
			cliBus.AddRelease(releaseHost)
		}

		// A real stored atom supplies a watch that crosses the Resource Init stream.
		atoms := resource_state.NewStateAtomManager(cliBus.GetBus(), "fixture-atoms", cliBus.GetVolume().GetID())
		cliBus.AddRelease(atoms.Release)

		// Store the identity value in the lifetime atom store.
		store, err := atoms.GetOrCreateStore(ctx, "lifetime")
		if err != nil {
			buildErr = err
			return nil
		}
		if _, err := store.Set(ctx, identity); err != nil {
			buildErr = err
			return nil
		}
		resourceMux := srpc.NewMux(resource_state.NewStateAtomResource(store).GetMux())
		if target := os.Getenv(sharedDaemonUpdateTarget); target != "" && filepath.Dir(executable) != filepath.Join(statePath, "daemon-bin") {
			input, err := os.Open(target)
			if err != nil {
				buildErr = err
				return nil
			}
			digest := sha256.New()
			_, buildErr = io.Copy(digest, input)
			_ = input.Close()
			if buildErr != nil {
				return nil
			}
			launcher := newFixtureLauncher(target, hex.EncodeToString(digest.Sum(nil)), statePath, os.Getenv(sharedDaemonCorruptOnAccept) != "")
			if buildErr = resourceMux.Register(&fixtureUpdateTrigger{launcher: launcher}); buildErr != nil {
				return nil
			}
			launcherMux := srpc.NewMux()
			if buildErr = spacewave_launcher.SRPCRegisterLauncher(launcherMux, launcher); buildErr != nil {
				return nil
			}
			launcherController := &fixtureLauncherController{mux: launcherMux}
			release, err := cliBus.GetBus().AddController(ctx, launcherController, nil)
			if err != nil {
				buildErr = err
				return nil
			}
			cliBus.AddRelease(release)
		}
		mux := srpc.NewMux()
		buildErr = resource_server.NewResourceServer(resourceMux).Register(mux)
		if buildErr != nil {
			return nil
		}

		// Select the actual native lookup or distribution plugin forwarding path.
		var ctrl controller.Controller = bifrost_rpc.NewInvokerController(le, cliBus.GetBus(),
			controller.NewInfo("test/resource", controller.MustParseVersion("0.0.1"), ""), mux,
			[]string{resource.SRPCResourceServiceServiceID})
		if os.Getenv(sharedDaemonFixtureMode) == "distribution" {
			ctrl = newNativeCorePlugin(mux)
		}
		release, err := cliBus.GetBus().AddController(ctx, ctrl, nil)
		if err != nil {
			buildErr = err
			return nil
		}
		cliBus.AddRelease(release)
		if os.Getenv(sharedDaemonFixtureMode) == "distribution" {
			return &sharedDaemonFixtureBus{CliBus: cliBus}
		}
		return cliBus
	}

	// Use the same flags and daemon lifetime for both host shapes.
	app := cli.NewApp()
	app.Flags = []cli.Flag{statePathFlag(nil), socketPathFlag()}
	app.Commands = []*cli.Command{newServeCommand(getBus, yield_policy.NewBroker())}
	err := app.RunContext(ctx, os.Args)
	if buildErr != nil {
		return buildErr
	}
	return err
}
