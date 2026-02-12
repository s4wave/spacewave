//go:build !js

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/bldr/web/plugin/saucer"
	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
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
					return fmt.Errorf("read %s: %w", filePath, err)
				}
				code = string(data)
			} else if c.NArg() > 0 {
				code = c.Args().First()
			} else {
				return fmt.Errorf("provide code as argument or use --file")
			}
			return runDebugEval(c.Context, code)
		},
	}
}

// findDebugSocket walks up from cwd looking for .bldr/saucer-debug.sock.
func findDebugSocket() (string, error) {
	if p := os.Getenv("BLDR_DEBUG_SOCK"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("socket not found at BLDR_DEBUG_SOCK=%s", p)
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
	return "", fmt.Errorf("saucer-debug.sock not found (searched %d levels up from cwd)", maxWalkDepth)
}

func dialDebug() (srpc.Client, error) {
	sockPath, err := findDebugSocket()
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", sockPath, err)
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
		return fmt.Errorf("eval: %s", resp.GetError())
	}
	result := resp.GetResult()
	if result != "" {
		fmt.Println(result)
	}
	return nil
}
