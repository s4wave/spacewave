package main

import (
	"context"
	"errors"
	"os"

	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	app := cli.NewApp()
	app.Name = "run-worker"
	app.Usage = "run a forge worker with a target"
	app.HideVersion = true
	app.Action = func(c *cli.Context) error {
		args := c.Args().Slice()
		if len(args) == 0 {
			return errors.New("usage: ./run-worker ./test-target.yaml")
		}
		return runWorkerDemo(ctx, le, args[0])
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runWorkerDemo runs the Execution demo.
func runWorkerDemo(ctx context.Context, le *logrus.Entry, targetPath string) error {
	if _, err := os.Stat(targetPath); err != nil {
		return err
	}

	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}

	// unmarshal target from yaml into a container for later type resolution
	verbose := false
	tb, err := testbed.Default(ctx, world_testbed.WithWorldVerbose(verbose))
	if err != nil {
		return err
	}
	forge_lib_all.AddFactories(tb.Bus, tb.StaticResolver)

	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, targetData)
	if err != nil {
		return err
	}

	ts := timestamp.Now()
	taskMap := map[string]*forge_target.Target{
		"cli-task": tgt,
	}
	jobKey := "job/1"
	clusterKey := "cluster/1"
	_, err = tb.RunWorkerWithTasks(taskMap, nil, 1, ts, jobKey, clusterKey, nil)
	return err
}
