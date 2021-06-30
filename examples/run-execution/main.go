package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"

	execution_mock "github.com/aperturerobotics/forge/execution/mock"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := runExecutionDemo(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runExecutionDemo runs the Execution demo.
func runExecutionDemo(ctx context.Context, le *logrus.Entry) error {
	// read target path
	if len(os.Args) < 2 {
		return errors.New("usage: ./run-execution ./test-target.yaml")
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
	var tgt target_json.Target
	if err := tgt.UnmarshalYAML(targetData); err != nil {
		return err
	}
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return err
	}
	_, err = execution_mock.RunTargetInTestbed(
		tb,
		&tgt,
		nil,
		nil,
	)
	return err
}
