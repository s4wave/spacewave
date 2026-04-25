//go:build !js

package main

import (
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
)

func buildWatchCommand() *cli.Command {
	var filePath string
	var interval time.Duration
	return &cli.Command{
		Name:  "watch",
		Usage: "repeatedly evaluate a JS file at an interval",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "file",
				Aliases:     []string{"f"},
				Usage:       "path to JavaScript file to evaluate",
				Destination: &filePath,
				Required:    true,
			},
			&cli.DurationFlag{
				Name:        "interval",
				Aliases:     []string{"i"},
				Usage:       "polling interval",
				Value:       2 * time.Second,
				Destination: &interval,
			},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			run := func() error {
				data, err := os.ReadFile(filePath)
				if err != nil {
					return errors.Wrapf(err, "read %s", filePath)
				}
				return args.EvalCode(ctx, string(data))
			}

			// Run immediately on first tick.
			if err := run(); err != nil {
				os.Stderr.WriteString(err.Error() + "\n")
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					if err := run(); err != nil {
						os.Stderr.WriteString(err.Error() + "\n")
					}
				}
			}
		},
	}
}
