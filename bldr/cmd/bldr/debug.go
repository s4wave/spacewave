//go:build !js

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/web/plugin/saucer"
)

const maxWalkDepth = 10

func init() {
	commands = append(commands, buildDebugCommand())
}

func buildDebugCommand() *cli.Command {
	return &cli.Command{
		Name:  "debug",
		Usage: "debug tools for the saucer webview",
		Subcommands: []*cli.Command{
			buildDebugEvalCommand(),
		},
	}
}

func buildDebugEvalCommand() *cli.Command {
	var filePath string
	return &cli.Command{
		Name:      "eval",
		Usage:     "evaluate JavaScript in the saucer webview",
		ArgsUsage: "[code]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "file",
				Aliases:     []string{"f"},
				Usage:       "read code from a file instead of arguments",
				Destination: &filePath,
			},
		},
		Action: func(c *cli.Context) error {
			var code string
			if filePath != "" {
				data, err := os.ReadFile(filePath)
				if err != nil {
					return errors.Wrap(err, "read "+filePath)
				}
				code = string(data)
			} else if c.NArg() > 0 {
				code = c.Args().First()
			} else {
				return errors.New("provide code as argument or use --file")
			}
			return runDebugEval(c.Context, code)
		},
	}
}

// findDebugSocket walks up from cwd looking for .bldr/saucer-debug.sock.
func findDebugSocket() (string, error) {
	if p := os.Getenv("BLDR_DEBUG_SOCK"); p != "" {
		// #nosec G703 -- BLDR_DEBUG_SOCK intentionally accepts a user-provided local socket path.
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", errors.New("socket not found at BLDR_DEBUG_SOCK=" + p)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for range maxWalkDepth {
		p := filepath.Join(dir, ".bldr", "saucer-debug.sock")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("saucer-debug.sock not found (searched " + strconv.Itoa(maxWalkDepth) + " levels up from cwd)")
}

func dialDebug() (srpc.Client, error) {
	sockPath, err := findDebugSocket()
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, errors.Wrap(err, "connect to "+sockPath)
	}
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

func runDebugEval(ctx context.Context, code string) error {
	client, err := dialDebug()
	if err != nil {
		return err
	}
	svc := saucer.NewSRPCSaucerDebugServiceClient(client)
	resp, err := svc.EvalJS(ctx, &saucer.EvalJSRequest{Code: code})
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		return errors.New("eval: " + resp.GetError())
	}
	result := resp.GetResult()
	if result != "" {
		os.Stdout.WriteString(result + "\n")
	}
	return nil
}
