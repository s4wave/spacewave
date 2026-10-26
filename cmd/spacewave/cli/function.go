//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/db/world"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// functionKeyPrefix prefixes the object key of a function set from a bare ID.
const functionKeyPrefix = "function/"

// newFunctionCommand builds the function command group.
func newFunctionCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:    "function",
		Aliases: []string{"functions"},
		Usage:   "store PureFunctions: TypeScript that turns protobuf input into output",
		Subcommands: []*cli.Command{
			buildFunctionSetCommand(),
		},
	}
}

// functionSetArgs stores a PureFunction.
type functionSetArgs struct {
	promArgs
	inputType       string
	outputType      string
	memoryLimit     uint
	interruptChecks uint
}

// BuildFlags returns the set flags.
func (a *functionSetArgs) BuildFlags() []cli.Flag {
	return append(a.promArgs.BuildFlags(),
		&cli.StringFlag{
			Name:        "input-type",
			Usage:       "full name of the input message, such as s4wave.alarm.watch.WatchInput",
			Required:    true,
			Destination: &a.inputType,
		},
		&cli.StringFlag{
			Name:        "output-type",
			Usage:       "full name of the output message, such as s4wave.alarm.watch.WatchResult",
			Required:    true,
			Destination: &a.outputType,
		},
		&cli.UintFlag{
			Name:        "memory-limit",
			Usage:       "heap limit in bytes (default: 32 MiB)",
			Destination: &a.memoryLimit,
		},
		&cli.UintFlag{
			Name:        "interrupt-checks",
			Usage:       "interrupt checks before a run fails, each about 10000 operations (default: 1000)",
			Destination: &a.interruptChecks,
		},
	)
}

// Run compiles the TypeScript file and stores it at the key, replacing the
// function already there.
func (a *functionSetArgs) Run(c *cli.Context) error {
	// Read the key and the source.
	if c.NArg() != 2 {
		return errors.New("set requires <id-or-object-key> <file.ts>, or - for stdin")
	}
	key := c.Args().Get(0)
	if !strings.Contains(key, "/") {
		key = functionKeyPrefix + key
	}
	source, err := readFunctionSource(c.Args().Get(1))
	if err != nil {
		return err
	}

	// Compile it.
	module, err := s4wave_function.Compile(source)
	if err != nil {
		return errors.Wrap(err, "compile")
	}
	fn := &s4wave_function.PureFunction{
		Source:     source,
		Module:     module,
		InputType:  a.inputType,
		OutputType: a.outputType,
	}
	if a.memoryLimit != 0 || a.interruptChecks != 0 {
		fn.Budget = &s4wave_function.Budget{
			MemoryLimitBytes: uint32(min(a.memoryLimit, 1<<32-1)),
			InterruptChecks:  uint32(min(a.interruptChecks, 1<<32-1)),
		}
	}

	// Store it in one transaction.
	err = a.withSpace(c, func(_ *sdkClient, _ s4wave_space.SRPCSpaceResourceServiceClient, engine world.Engine) error {
		return world.ExecTransaction(c.Context, engine, true, func(ctx context.Context, tx world.WorldState) error {
			return s4wave_function.SetPureFunction(ctx, tx, key, fn)
		})
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, key+"\n")
	return err
}

// readFunctionSource reads the file at path, or stdin for "-".
func readFunctionSource(path string) (string, error) {
	// Read stdin for "-", otherwise the file.
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", errors.Wrap(err, "read source")
	}
	return string(data), nil
}

// buildFunctionSetCommand builds the function set subcommand.
func buildFunctionSetCommand() *cli.Command {
	args := &functionSetArgs{}
	return &cli.Command{
		Name:      "set",
		Usage:     "compile a TypeScript PureFunction and store it, replacing the one at the key",
		ArgsUsage: "<id-or-object-key> <file.ts|->",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}
