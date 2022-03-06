package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"

	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	"github.com/aperturerobotics/timestamp"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := runWorkerDemo(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runWorkerDemo runs the Execution demo.
func runWorkerDemo(ctx context.Context, le *logrus.Entry) error {
	// read target path
	if len(os.Args) < 2 {
		return errors.New("usage: ./run-worker ./test-target.yaml")
	}

	targetPath := os.Args[1]
	if _, err := os.Stat(targetPath); err != nil {
		return err
	}

	targetData, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return err
	}

	// unmarshal target from yaml into a container for later type resolution
	tb, err := testbed.Default(ctx)
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
	_, err = tb.RunWorkerWithTasks(taskMap, nil, 1, &ts)
	return err
}
