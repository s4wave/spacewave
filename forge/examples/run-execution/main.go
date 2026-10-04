package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	forge_lib_all "github.com/s4wave/spacewave/forge/lib/all"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	"github.com/s4wave/spacewave/forge/testbed"
	"github.com/sirupsen/logrus"
)

func main() {
	// Prepare logging and context for the execution demo.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Run the demo and exit with a readable failure message.
	if err := runExecutionDemo(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runExecutionDemo runs the Execution demo.
func runExecutionDemo(ctx context.Context, le *logrus.Entry) error {
	// Require a target path before reading the execution definition.
	if len(os.Args) < 2 {
		return errors.New("usage: ./run-execution ./test-target.yaml")
	}

	// Normalize the requested path and confirm that it exists.
	targetPath := filepath.Clean(os.Args[1])
	if _, err := os.Stat(targetPath); err != nil {
		return err
	}

	// Read the target definition for YAML resolution.
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}

	// Resolve the YAML target after registering the execution factories.
	tb, err := testbed.Default(ctx)
	if err != nil {
		return err
	}
	forge_lib_all.AddFactories(tb.Bus, tb.StaticResolver)
	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, targetData)
	if err != nil {
		return err
	}

	// Run the target as a new execution with the current timestamp.
	ts := timestamp.Now()
	_, err = tb.RunExecutionWithTarget(tgt, nil, ts)
	return err
}
