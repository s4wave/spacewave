package resource_testbed

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	esbuild_api "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// SetupResourceClient creates pipes, muxed connections, and resource client for testing.
// Returns the resource client and a cleanup function.
func SetupResourceClient(ctx context.Context, t *testing.T, tb *world_testbed.Testbed) (*resource_client.Client, func()) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Create pipes for in-memory communication
	clientPipe, serverPipe := net.Pipe()

	// Create client muxed connection
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Create server mux
	mux := srpc.NewMux()
	server := srpc.NewServer(mux)

	// Create TestbedResourceServer as root
	volumeID := tb.Volume.GetID()
	bucketID := tb.BucketId
	testbedResource := NewTestbedResourceServer(ctx, le, tb.Bus, volumeID, bucketID)
	resourceServer := resource_server.NewResourceServer(testbedResource.GetMux())
	err = resourceServer.Register(mux)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start server
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	go func() {
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Create resource client
	resourceServiceClient := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceServiceClient)
	if err != nil {
		clientPipe.Close()
		serverPipe.Close()
		t.Fatal(err.Error())
	}

	cleanup := func() {
		resClient.Release()
		clientPipe.Close()
		serverPipe.Close()
	}

	return resClient, cleanup
}

// SetupTestbedWithClient creates a hydra testbed and resource client for testing.
// Returns the testbed, resource client, and a cleanup function.
func SetupTestbedWithClient(ctx context.Context, t *testing.T) (*world_testbed.Testbed, *resource_client.Client, func()) {
	// Create hydra world testbed
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Setup resource client
	resClient, clientCleanup := SetupResourceClient(ctx, t, tb)

	cleanup := func() {
		clientCleanup()
		tb.Release()
	}

	return tb, resClient, cleanup
}

// TestbedWithQuickJS wraps a bldr testbed with QuickJS plugin host and resource server setup.
type TestbedWithQuickJS struct {
	// Testbed is the underlying bldr testbed
	Testbed *testbed.Testbed
	// Bus is the controller bus
	Bus bus.Bus
	// ResourceServer is the testbed resource server
	ResourceServer *TestbedResourceServer
	// VolumeID is the volume ID used for world engines
	VolumeID string
	// BucketID is the bucket ID used for world engines
	BucketID string
	// Logger is the logger entry
	Logger *logrus.Entry
	// QuickJSHost is the QuickJS plugin host controller
	QuickJSHost *plugin_host_wazero_quickjs.Controller
	// QuickJSHostRef is the reference to release
	QuickJSHostRef directive.Reference
	// ObjectTypeCtrlRelease releases the ObjectType controller
	ObjectTypeCtrlRelease func()
}

// Release releases the testbed and all associated resources.
func (t *TestbedWithQuickJS) Release() {
	if t.ObjectTypeCtrlRelease != nil {
		t.ObjectTypeCtrlRelease()
	}
	if t.QuickJSHostRef != nil {
		t.QuickJSHostRef.Release()
	}
	t.Testbed.Release()
}

// SetupTestbedWithQuickJS creates a testbed with QuickJS plugin host and resource server.
// This is useful for e2e tests that need to run TypeScript code via QuickJS.
//
// Returns the configured testbed and an error if setup fails.
func SetupTestbedWithQuickJS(ctx context.Context, le *logrus.Entry) (*TestbedWithQuickJS, error) {
	// Create testbed
	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		return nil, err
	}

	b := tb.GetBus()
	sr := tb.GetStaticResolver()

	// Add QuickJS plugin host factory
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))

	// Add world engine factory
	sr.AddFactory(world_block_engine.NewFactory(b))

	// Create testbed resource server
	volumeID := tb.GetVolume().GetID()
	bucketID := "testbed-bucket"
	testbedResourceServer := NewTestbedResourceServer(
		ctx,
		le,
		b,
		volumeID,
		bucketID,
	)

	// Create root resource mux and register testbed resource server to it
	rootResourceMux := srpc.NewMux()
	if err := testbedResourceServer.Register(rootResourceMux); err != nil {
		tb.Release()
		return nil, err
	}

	// Register testbed resource service directly on the testbed bus mux.
	// This allows the QuickJS plugin to call TestbedResourceService via
	// the bus's RPC fallback invoker, bypassing the plugin host's own
	// resource server (which has a different root resource).
	if err := testbedResourceServer.Register(tb.GetMux()); err != nil {
		tb.Release()
		return nil, err
	}

	// load the plugin host
	quickjsHost, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_wazero_quickjs.Controller](
		ctx,
		tb.GetBus(),
		resolver.NewLoadControllerWithConfig(plugin_host_wazero_quickjs.NewConfig()),
		nil,
	)
	if err != nil {
		tb.Release()
		return nil, err
	}

	// Register ObjectType controller with known types
	objectTypes := map[string]objecttype.ObjectType{
		s4wave_layout_world.ObjectLayoutTypeID: s4wave_layout_world.ObjectLayoutType,
	}
	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		return objectTypes[typeID], nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeCtrlRelease, err := b.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		quickjsHostRef.Release()
		tb.Release()
		return nil, err
	}

	return &TestbedWithQuickJS{
		Testbed:               tb,
		Bus:                   b,
		ResourceServer:        testbedResourceServer,
		VolumeID:              volumeID,
		BucketID:              bucketID,
		Logger:                le,
		QuickJSHost:           quickjsHost,
		QuickJSHostRef:        quickjsHostRef,
		ObjectTypeCtrlRelease: objectTypeCtrlRelease,
	}, nil
}

// LoadQuickJSPlugin loads a QuickJS plugin with the given script contents.
// Calls QuickJSHost.ExecutePlugin directly (bypassing the scheduler) so the
// testbed's ResourceServer is used as the plugin's resource root.
// Returns a release function.
func (t *TestbedWithQuickJS) LoadQuickJSPlugin(
	ctx context.Context,
	pluginID string,
	scriptContents string,
) (directive.Reference, error) {
	platformID := t.QuickJSHost.GetPluginHost().GetPlatformId()
	scriptPath := pluginID + ".js"

	// Create plugin manifest
	assetsFS, distFS := memfs.New(), memfs.New()
	nowTs := timestamppb.Now()
	err := billy_util.WriteFile(distFS, scriptPath, []byte(scriptContents), 0o644)
	if err != nil {
		return nil, err
	}

	manifestID := pluginID
	manifestMeta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_DEV, platformID, 1)
	manifest, manifestRef, err := t.Testbed.CreateManifestWithBilly(ctx, manifestMeta, scriptPath, distFS, assetsFS, nowTs)
	if err != nil {
		return nil, err
	}
	_ = manifestRef

	// Build a host mux for the plugin with the testbed's resource server.
	// This ensures the plugin's ResourceService root is the testbed's
	// TestbedResourceService, not the default pluginHostRoot.
	hostMux := srpc.NewMux(bifrost_rpc.NewInvoker(t.Bus, bldr_plugin.PluginServerID(pluginID, ""), true))

	// Register the testbed's resource server on the plugin's host mux.
	rootResourceMux := srpc.NewMux()
	if err := t.ResourceServer.Register(rootResourceMux); err != nil {
		return nil, err
	}
	resourceSrv := resource_server.NewResourceServer(rootResourceMux)
	if err := resourceSrv.Register(hostMux); err != nil {
		return nil, err
	}

	// Register plugin host service for GetPluginInfo etc.
	manifestSnapshot := &bldr_manifest.ManifestSnapshot{Manifest: manifest}
	pluginHostSrv := bldr_plugin_host.NewPluginHostServer(ctx, t.Bus, t.Logger, pluginID, manifestSnapshot, nil)
	_ = bldr_plugin.SRPCRegisterPluginHost(hostMux, pluginHostSrv)

	// Convert billy FS handles to unixfs handles for ExecutePlugin.
	distCursor := unixfs_billy.NewBillyFSCursor(distFS, "/")
	distRef, err := unixfs.NewFSHandle(distCursor)
	if err != nil {
		return nil, err
	}

	assetsCursor := unixfs_billy.NewBillyFSCursor(assetsFS, "/")
	assetsRef, err := unixfs.NewFSHandle(assetsCursor)
	if err != nil {
		distRef.Release()
		return nil, err
	}

	// Execute the plugin directly via QuickJS host.
	pluginCtx, pluginCancel := context.WithCancel(ctx)
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- t.QuickJSHost.GetPluginHost().ExecutePlugin(
			pluginCtx,
			pluginID,
			"",
			scriptPath,
			distRef,
			assetsRef,
			hostMux,
			func(client srpc.Client) error { return nil },
		)
	}()

	// Return a reference that cancels the plugin on release.
	return &pluginRelease{
		cancel:    pluginCancel,
		distRef:   distRef,
		assetsRef: assetsRef,
		doneCh:    doneCh,
	}, nil
}

// pluginRelease implements directive.Reference for plugin lifecycle cleanup.
type pluginRelease struct {
	cancel    context.CancelFunc
	distRef   *unixfs.FSHandle
	assetsRef *unixfs.FSHandle
	doneCh    <-chan error
}

func (p *pluginRelease) Release() {
	p.cancel()
	p.distRef.Release()
	p.assetsRef.Release()
	<-p.doneCh
}

// RunTypeScriptTest builds a wrapper around a TypeScript test file and runs it in QuickJS.
// The test file should export a default async function with signature:
//
//	export default async function main(
//	  backendAPI: BackendAPI,
//	  abortSignal: AbortSignal,
//	  testbedRoot: TestbedRoot,
//	)
//
// The wrapper automatically sets up the resources client, testbed root, and reports
// test results via MarkTestResult. This is the recommended way to run TypeScript E2E tests.
//
// Example test file (my-test.ts):
//
//	import type { BackendAPI } from '@aptre/bldr-sdk'
//	import type { TestbedRoot } from '../../../sdk/testbed/testbed.js'
//
//	export default async function main(
//	  backendAPI: BackendAPI,
//	  abortSignal: AbortSignal,
//	  testbedRoot: TestbedRoot,
//	) {
//	  // Create a world engine
//	  using engine = await testbedRoot.createWorld('my-engine')
//
//	  // Do your test logic here
//	  const ws = engine.asWorldState()
//	  using obj = await ws.createObject('test-key', {})
//	  console.log('Test passed!')
//	}
//
// Usage in Go test:
//
//	func TestMyFeature(t *testing.T) {
//	    ctx := context.Background()
//	    log := logrus.New()
//	    log.SetLevel(logrus.DebugLevel)
//	    le := logrus.NewEntry(log)
//
//	    success, errorMsg, err := RunTypeScriptTest(ctx, le, "my-test", "my-test.ts")
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//	    if !success {
//	        t.Fatalf("test failed: %s", errorMsg)
//	    }
//	}
func RunTypeScriptTest(
	ctx context.Context,
	le *logrus.Entry,
	pluginID string,
	tsFilePath string,
) (success bool, errorMsg string, err error) {
	// Setup testbed with QuickJS plugin host
	tb, err := SetupTestbedWithQuickJS(ctx, le)
	if err != nil {
		return false, "", err
	}
	defer tb.Release()

	// Determine paths
	// tsFilePath is relative to the test directory (e.g. "my-test.ts")
	// templatePath is in the same directory as this file
	templatePath := filepath.Join(filepath.Dir(tsFilePath), "testbed-wrapper.ts.tmpl")
	wrapperPath := filepath.Join(filepath.Dir(tsFilePath), pluginID+"-wrapper.ts")

	// Read the wrapper template
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return false, "", fmt.Errorf("failed to read wrapper template: %w", err)
	}

	// Replace the placeholder with the test file name without extension.
	testFileName := strings.TrimSuffix(filepath.Base(tsFilePath), ".ts")
	wrapperContents := strings.ReplaceAll(string(templateBytes), "{{TEST_FILE_NAME}}", testFileName)

	// Write the wrapper to a temporary file
	err = os.WriteFile(wrapperPath, []byte(wrapperContents), 0o644)
	if err != nil {
		return false, "", fmt.Errorf("failed to write wrapper file: %w", err)
	}
	defer os.Remove(wrapperPath) // Clean up wrapper file

	// Determine the repo root for resolving vendor paths.
	// The test CWD is typically the package directory (e.g. sdk/world/types/).
	// Walk up until we find vendor/ or go.mod.
	repoRoot, err := findRepoRoot()
	if err != nil {
		return false, "", fmt.Errorf("failed to find repo root: %w", err)
	}
	vendorDir := filepath.Join(repoRoot, "vendor")

	// Build an esbuild plugin to resolve @go/ and @aptre/ imports via vendor.
	vendorPlugin := esbuild_api.Plugin{
		Name: "vendor-resolver",
		Setup: func(build esbuild_api.PluginBuild) {
			// Resolve @go/* to vendor/*
			build.OnResolve(esbuild_api.OnResolveOptions{Filter: `^@go/`}, func(args esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
				// Strip @go/ prefix, resolve in vendor
				importPath := strings.TrimPrefix(args.Path, "@go/")
				resolved := filepath.Join(vendorDir, importPath)
				// Try .ts extension if .js was requested
				if before, ok := strings.CutSuffix(resolved, ".js"); ok {
					tsPath := before + ".ts"
					if _, err := os.Stat(tsPath); err == nil {
						return esbuild_api.OnResolveResult{Path: tsPath}, nil
					}
				}
				if _, err := os.Stat(resolved); err == nil {
					return esbuild_api.OnResolveResult{Path: resolved}, nil
				}
				return esbuild_api.OnResolveResult{}, nil
			})
			// Resolve @aptre/bldr-sdk/* to vendor bldr SDK
			build.OnResolve(esbuild_api.OnResolveOptions{Filter: `^@aptre/bldr-sdk`}, func(args esbuild_api.OnResolveArgs) (esbuild_api.OnResolveResult, error) {
				importPath := strings.TrimPrefix(args.Path, "@aptre/bldr-sdk")
				if importPath == "" {
					importPath = "/plugin.ts"
				}
				resolved := filepath.Join(vendorDir, "github.com/s4wave/spacewave/bldr/sdk", importPath)
				if before, ok := strings.CutSuffix(resolved, ".js"); ok {
					tsPath := before + ".ts"
					if _, err := os.Stat(tsPath); err == nil {
						return esbuild_api.OnResolveResult{Path: tsPath}, nil
					}
				}
				if _, err := os.Stat(resolved); err == nil {
					return esbuild_api.OnResolveResult{Path: resolved}, nil
				}
				return esbuild_api.OnResolveResult{}, nil
			})
		},
	}

	// Bundle the wrapper (which imports the test file) using esbuild
	result := esbuild_api.Build(esbuild_api.BuildOptions{
		EntryPoints: []string{wrapperPath},
		Bundle:      true,
		Format:      esbuild_api.FormatESModule,
		Target:      esbuild_api.ES2022,
		TreeShaking: esbuild_api.TreeShakingTrue,
		Platform:    esbuild_api.PlatformBrowser,
		Write:       false,
		Plugins:     []esbuild_api.Plugin{vendorPlugin},
	})

	if len(result.Errors) > 0 {
		return false, "", fmt.Errorf("esbuild errors: %v", result.Errors)
	}

	if len(result.OutputFiles) == 0 {
		return false, "", fmt.Errorf("no output files from esbuild")
	}

	scriptContents := string(result.OutputFiles[0].Contents)

	// Load the bundled script into QuickJS
	pluginRef, err := tb.LoadQuickJSPlugin(ctx, pluginID, scriptContents)
	if err != nil {
		return false, "", fmt.Errorf("failed to load QuickJS plugin: %w", err)
	}
	defer pluginRef.Release()

	le.Info("plugin started, waiting for test to complete...")

	// Wait for the test to complete and get the result
	success, errorMsg, err = tb.ResourceServer.WaitForTestResult(ctx)
	if err != nil {
		return false, "", fmt.Errorf("error waiting for test result: %w", err)
	}

	return success, errorMsg, nil
}

// findRepoRoot walks up from the current directory to find the repo root
// (identified by the presence of go.mod).
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}
