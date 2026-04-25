//go:build !js

package cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	debug_projectroot "github.com/s4wave/spacewave/core/debug/projectroot"
	s4wave_debug "github.com/s4wave/spacewave/sdk/debug"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

const (
	socketName   = ".bldr/alpha-debug.sock"
	maxWalkDepth = 10
	corePluginID = "spacewave-core"
)

// ClientArgs contains the client arguments and functions.
type ClientArgs struct {
	ctx    context.Context
	conn   srpc.Client
	client s4wave_debug.SRPCDebugBridgeServiceClient

	// SessionIdx is the session index to use.
	SessionIdx uint
	// EvalFilePath is the path to a JS file for eval.
	EvalFilePath string
	// SpaceName is the name for create-space.
	SpaceName string
	// WebPkgs is extra web packages for the eval bundler.
	WebPkgs cli.StringSlice
}

// BuildFlags returns the common flags.
func (a *ClientArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.UintFlag{
			Name:        "session-idx",
			Usage:       "session index to use",
			Value:       1,
			Destination: &a.SessionIdx,
		},
	}
}

// BuildCommands returns the command list.
func (a *ClientArgs) BuildCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:      "eval",
			Usage:     "evaluate JavaScript or TypeScript in the page context",
			ArgsUsage: "[code]",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "file",
					Aliases:     []string{"f"},
					Usage:       "read code from a file (.js, .ts, .tsx)",
					Destination: &a.EvalFilePath,
				},
				&cli.StringSliceFlag{
					Name:        "web-pkgs",
					Usage:       "additional web packages to externalize for TS bundling",
					Destination: &a.WebPkgs,
				},
			},
			Action: a.RunEval,
		},
		{
			Name:   "info",
			Usage:  "print page URL, title, and IDs",
			Action: a.RunInfo,
		},
		{
			Name:   "wait",
			Usage:  "wait for the debug bridge to become ready",
			Action: a.RunWait,
		},
		{
			Name:   "list-spaces",
			Usage:  "list spaces in the current session",
			Flags:  a.BuildFlags(),
			Action: a.RunListSpaces,
		},
		{
			Name:  "create-space",
			Usage: "create a new space",
			Flags: append(a.BuildFlags(), &cli.StringFlag{
				Name:        "name",
				Usage:       "human-readable name for the space",
				Required:    true,
				Destination: &a.SpaceName,
			}),
			Action: a.RunCreateSpace,
		},
	}
}

// SetContext sets the context.
func (a *ClientArgs) SetContext(c context.Context) {
	a.ctx = c
}

// GetContext returns the context. Falls back to context.Background() for CLI
// entrypoints where no caller context is available.
func (a *ClientArgs) GetContext() context.Context {
	if c := a.ctx; c != nil {
		return c
	}
	return context.Background()
}

// BuildSrpcClient builds or returns the cached SRPC client for the debug socket.
func (a *ClientArgs) BuildSrpcClient() (srpc.Client, error) {
	if a.conn != nil {
		return a.conn, nil
	}
	_, err := a.BuildClient()
	if err != nil {
		return nil, err
	}
	return a.conn, nil
}

// BuildClient builds or returns the cached debug bridge client.
func (a *ClientArgs) BuildClient() (s4wave_debug.SRPCDebugBridgeServiceClient, error) {
	if a.client != nil {
		return a.client, nil
	}

	sockPath, err := findSocket()
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, errors.Wrapf(err, "connect to %s", sockPath)
	}
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	a.conn = client
	a.client = s4wave_debug.NewSRPCDebugBridgeServiceClient(client)
	return a.client, nil
}

// DialPluginRpc opens a PluginRpc stream to the given plugin via the debug
// bridge, returning an srpc.Client for that plugin's RPC services.
func (a *ClientArgs) DialPluginRpc(pluginID string) (srpc.Client, error) {
	svc, err := a.BuildClient()
	if err != nil {
		return nil, err
	}
	return rpcstream.NewRpcStreamClient(svc.PluginRpc, pluginID, true), nil
}

// BuildCoreClient returns an SRPC client connected to the spacewave-core
// plugin via PluginRpc. Reusable for any RPC that needs core bus access.
func (a *ClientArgs) BuildCoreClient() (srpc.Client, error) {
	return a.DialPluginRpc(corePluginID)
}

// MountSession opens a ResourceClient via PluginRpc to spacewave-core, mounts
// a session by index via the root resource, and returns the Session SDK wrapper
// along with a cleanup function.
func (a *ClientArgs) MountSession(ctx context.Context, sessionIdx uint32) (*s4wave_session.Session, func(), error) {
	pluginClient, err := a.DialPluginRpc(corePluginID)
	if err != nil {
		return nil, nil, err
	}

	resourceSvc := resource.NewSRPCResourceServiceClient(pluginClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		return nil, nil, errors.Wrap(err, "resource client")
	}

	rootRef := resClient.AccessRootResource()
	root, err := s4wave_root.NewRoot(resClient, rootRef)
	if err != nil {
		resClient.Release()
		return nil, nil, errors.Wrap(err, "root resource")
	}

	resp, err := root.MountSessionByIdx(ctx, sessionIdx)
	if err != nil {
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		root.Release()
		resClient.Release()
		return nil, nil, errors.Errorf("no session found at index %d", sessionIdx)
	}

	sessRef := resClient.CreateResourceReference(resp.GetResourceId())
	sess, err := s4wave_session.NewSession(resClient, sessRef)
	if err != nil {
		sessRef.Release()
		root.Release()
		resClient.Release()
		return nil, nil, errors.Wrap(err, "session resource")
	}

	cleanup := func() {
		sess.Release()
		root.Release()
		resClient.Release()
	}
	return sess, cleanup, nil
}

// RunEvalJSON evaluates code via EvalJS and parses the JSON result with fastjson.
func (a *ClientArgs) RunEvalJSON(ctx context.Context, code string, fn func(*fastjson.Value)) error {
	svc, err := a.BuildClient()
	if err != nil {
		return err
	}
	resp, err := svc.EvalJS(ctx, &s4wave_debug.EvalJSRequest{Code: code})
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		return errors.Errorf("eval: %s", resp.GetError())
	}
	result := resp.GetResult()
	if result == "" {
		return errors.New("eval returned empty result")
	}
	var p fastjson.Parser
	v, err := p.Parse(result)
	if err != nil {
		return err
	}
	fn(v)
	return nil
}

// EscapeJSString returns s as a safe JavaScript string literal (with quotes).
func EscapeJSString(s string) string {
	var a fastjson.Arena
	return string(a.NewString(s).MarshalTo(nil))
}

// LooksLikeSyntaxError returns true if the error message suggests shell quoting issues.
func LooksLikeSyntaxError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "syntaxerror") ||
		strings.Contains(lower, "unexpected token") ||
		strings.Contains(lower, "unterminated string") ||
		strings.Contains(lower, "unexpected end of input")
}

func findProjectRoot() (string, error) {
	return debug_projectroot.FindFromCwd(maxWalkDepth)
}

func findSocket() (string, error) {
	if p := os.Getenv("ALPHA_DEBUG_SOCK"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", errors.Errorf("socket not found at ALPHA_DEBUG_SOCK=%s", p)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for range maxWalkDepth {
		p := filepath.Join(dir, socketName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.Errorf("socket not found (searched %d levels up from cwd for %s)", maxWalkDepth, socketName)
}
