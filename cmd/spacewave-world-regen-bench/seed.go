//go:build !js && !wasip1

package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/world"
)

// seedArgs seed the World with results, resuming from the last seeded batch.
type seedArgs struct {
	// Path is the database file.
	Path string
	// Results is the number of results the seed holds.
	Results int
	// Batch is the results per commit.
	Batch int
}

// BuildFlags binds the flags.
func (a *seedArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "path", Usage: "database file", Destination: &a.Path, Required: true},
		&cli.IntFlag{Name: "results", Usage: "results the seed holds", Destination: &a.Results, Value: 2_200_000},
		&cli.IntFlag{Name: "batch", Usage: "results per commit", Destination: &a.Batch, Value: 5000},
	}
}

// Run seeds the World.
func (a *seedArgs) Run(c *cli.Context) error {
	// Open the World and find where the seed stopped.
	ctx := c.Context
	w, err := openBenchWorld(ctx, a.Path)
	if err != nil {
		return err
	}
	defer w.Close()
	done, err := lastInt(seededPath(a.Path))
	if err != nil {
		return err
	}

	// Commit the hubs with the first batch, then each batch, recording each.
	start := time.Now()
	for done < a.Results {
		end := min(done+a.Batch, a.Results)
		err := commitTx(ctx, w, func(ws world.WorldState) error {
			if done == 0 {
				if err := writeHubs(ctx, ws); err != nil {
					return err
				}
			}
			return writeResults(ctx, ws, done, end)
		})
		if err != nil {
			return err
		}
		if err := appendLine(seededPath(a.Path), strconv.Itoa(end)); err != nil {
			return err
		}
		done = end
		fmt.Fprintf(os.Stderr, "seeded %d of %d in %v\n", done, a.Results, time.Since(start).Round(time.Second))
	}
	return nil
}
