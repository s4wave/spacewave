//go:build !js

package spacewave_cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/db/world"
	s4wave_k8s "github.com/s4wave/spacewave/sdk/k8s"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// k8sArgs selects the daemon, session and Space of a k8s command.
type k8sArgs struct {
	statePath  string
	spaceID    string
	sessionIdx int
}

// BuildFlags returns the flags shared by every k8s subcommand.
func (a *k8sArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		statePathFlag(&a.statePath),
		socketPathFlag(),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space ID or name (default: the only Space)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: &a.spaceID,
		},
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session index",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: &a.sessionIdx,
		},
	}
}

// newK8sCommand builds the k8s command group.
func newK8sCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "k8s",
		Usage: "serve a Space's Kubernetes API to kubectl with the spacewave-k8s plugin",
		Subcommands: []*cli.Command{
			buildK8sBindCommand(true),
			buildK8sBindCommand(false),
			buildK8sKubeconfigCommand(),
		},
	}
}

// k8sBindArgs turns the Space's Kubernetes API on or off on this Device.
type k8sBindArgs struct {
	k8sArgs
	enable bool
}

// Run creates the Space's k8s/cluster object when enabling, then records
// this Device's binding decision for it.
func (a *k8sBindArgs) Run(c *cli.Context) error {
	idx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}
	return withVmSpace(c, a.statePath, idx, a.spaceID, func(client *sdkClient, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient, engine world.Engine) error {
		// Create the cluster object.
		if a.enable {
			err := world.ExecTransaction(c.Context, engine, true, func(ctx context.Context, tx world.WorldState) error {
				return s4wave_k8s.EnsureCluster(ctx, tx)
			})
			if err != nil {
				return err
			}
		}

		// Set the binding through the Space contents resource.
		contents, release, err := client.mountSpaceContents(c.Context, spaceSvc)
		if err != nil {
			return err
		}
		defer release()
		_, err = contents.SetProcessBinding(c.Context, &s4wave_space.SetProcessBindingRequest{
			ObjectKey: s4wave_k8s.ClusterObjectKey,
			TypeId:    s4wave_k8s.ClusterTypeID,
			Approved:  a.enable,
		})
		return errors.Wrap(err, "set cluster process binding")
	})
}

// buildK8sBindCommand builds the enable or disable subcommand.
func buildK8sBindCommand(enable bool) *cli.Command {
	args := &k8sBindArgs{enable: enable}
	name, usage := "enable", "serve the Space's Kubernetes API from this Device"
	if !enable {
		name, usage = "disable", "stop serving the Space's Kubernetes API from this Device"
	}
	return &cli.Command{
		Name:   name,
		Usage:  usage,
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// k8sKubeconfigArgs prints a kubeconfig for the Space.
type k8sKubeconfigArgs struct {
	k8sArgs
	port int
}

// BuildFlags adds the port bkctl proxy serves on.
func (a *k8sKubeconfigArgs) BuildFlags() []cli.Flag {
	return append(a.k8sArgs.BuildFlags(), &cli.IntFlag{
		Name:        "port",
		Usage:       "loopback port bkctl proxy serves the Space's API on",
		Value:       8001,
		Destination: &a.port,
	})
}

// Run prints a kubeconfig with one context for the Space, whose server is the
// loopback port of bkctl proxy, and the bkctl proxy command that serves it.
func (a *k8sKubeconfigArgs) Run(c *cli.Context) error {
	// Resolve the Space through its cluster object, which must exist.
	idx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}
	mount, err := mountObjectChain(c, a.statePath, fsURI{sessionIdx: idx, spaceID: a.spaceID, objectKey: s4wave_k8s.ClusterObjectKey}, nil)
	if err != nil {
		return errors.Wrap(err, "open the Space's k8s/cluster object (enable it with spacewave k8s enable)")
	}
	spaceID := mount.SpaceID
	mount.release()

	// Name the daemon in the proxy command, since it runs from anywhere.
	args := []string{"bkctl", "proxy", "--port", strconv.Itoa(a.port), "--space", spaceID, "--session-index", strconv.Itoa(a.sessionIdx)}
	flag, path := "--socket-path", effectiveSocketPath(c, "")
	if path == "" {
		flag = "--state-path"
		path, err = resolveStatePathFromContext(c, a.statePath)
		if err != nil {
			return err
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	args = append(args, flag, path)

	// Print the kubeconfig. JSON strings are YAML ones.
	var arena fastjson.Arena
	name := "spacewave-" + spaceID
	q := func(s string) string {
		return string(arena.NewString(s).MarshalTo(nil))
	}
	server := "http://127.0.0.1:" + strconv.Itoa(a.port)
	_, err = fmt.Fprintf(os.Stdout, `apiVersion: v1
kind: Config
clusters:
- name: %[1]s
  cluster:
    server: %[2]s
contexts:
- name: %[1]s
  context:
    cluster: %[1]s
current-context: %[1]s
`, q(name), q(server))
	if err != nil {
		return err
	}

	// Print the command that serves it.
	_, err = fmt.Fprintf(os.Stderr, "Serve %s for kubectl with:\n  %s\n", server, shellJoin(args))
	return err
}

// buildK8sKubeconfigCommand builds the kubeconfig subcommand.
func buildK8sKubeconfigCommand() *cli.Command {
	args := &k8sKubeconfigArgs{}
	return &cli.Command{
		Name:  "kubeconfig",
		Usage: "print a kubeconfig that reaches the Space's Kubernetes API through bkctl proxy",
		Description: "Print a kubeconfig with one context for the Space, named spacewave-<space-id>,\n" +
			"whose server is bkctl proxy on a loopback port, and print the bkctl proxy\n" +
			"command that serves it to stderr. bkctl proxy reaches the Space through the\n" +
			"daemon as the session's participant, so the file holds no secret. The\n" +
			"Space's Kubernetes API must be enabled on a Device of the Space.\n\n" +
			"EXAMPLES:\n" +
			"  spacewave k8s enable --space my-space\n" +
			"  spacewave k8s kubeconfig --space my-space > ~/.kube/spacewave\n" +
			"  bkctl proxy --port 8001 --space <space-id> &\n" +
			"  KUBECONFIG=~/.kube/spacewave kubectl get nodes",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
