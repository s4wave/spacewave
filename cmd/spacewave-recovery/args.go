//go:build !js && !wasip1

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/aperturerobotics/cli"
	space_recovery "github.com/s4wave/spacewave/core/space/recovery"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	recovery "github.com/s4wave/spacewave/db/world/recovery"
)

// Args selects a count's immutable manifest and durable scratch report.
type Args struct {
	// Source is the binary recovery.Source manifest written during authorized capture.
	Source string
	// Scratch is a separate writable work index, retained for resume and report.
	Scratch string
	// Report reads only the saved result, without opening the source.
	Report bool
}

// BuildFlags binds the count and report inputs.
func (a *Args) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "source", Usage: "binary capture manifest (recovery.Source)", Destination: &a.Source},
		&cli.StringFlag{Name: "scratch", Usage: "separate durable work file", Required: true, Destination: &a.Scratch},
		&cli.BoolFlag{Name: "report", Usage: "read the saved report without opening its source", Destination: &a.Report},
	}
}

// Run resumes a count until complete or interrupted, then prints its durable report.
func (a *Args) Run(c *cli.Context) error {
	// A report remains available when its source has been taken offline.
	if a.Report {
		result, err := recovery.ReadResult(a.Scratch)
		if err != nil {
			return err
		}
		return recovery.WriteReport(c.App.Writer, result)
	}
	if a.Source == "" {
		return errors.New("count requires --source; use --report to read a saved result")
	}

	// Bound manifest allocation before decoding the captured typing context.
	file, err := os.Open(a.Source)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return errors.New("capture manifest exceeds 1 MiB")
	}
	source := &recovery.Source{}
	if err := source.UnmarshalVT(data); err != nil {
		return err
	}

	// Interrupt only scratch work; source handles expose immutable reads.
	ctx, cancel := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runCount(ctx, c.App.Writer, source, a.Scratch, recovery.Options{Lookup: optypes.LookupBlockType, Witness: space_recovery.ValidateWitness, InterpretationID: "spacewave-native-registry-v1"})
}

// runCount is the CLI's persisted-store count path, shared with its run-through.
func runCount(ctx context.Context, writer io.Writer, source *recovery.Source, scratch string, options recovery.Options) error {
	// Close immutable readers after printing the last durable state, including pauses.
	scanner, err := recovery.NewScanner(ctx, source, scratch, options)
	if err != nil {
		return err
	}
	defer scanner.Close()
	result, runErr := scanner.Run(ctx)
	return errors.Join(runErr, recovery.WriteReport(writer, result))
}
