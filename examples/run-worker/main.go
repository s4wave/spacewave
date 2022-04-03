package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"

	podman_client "github.com/aperturerobotics/containers/podman/client"
	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
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
		return errors.New("usage: ./run-worker ./test-target.yaml [podman url]")
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
	verbose := false
	tb, err := testbed.Default(ctx, world_testbed.WithWorldVerbose(verbose))
	if err != nil {
		return err
	}
	forge_lib_all.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(podman_client.NewFactory(tb.Bus))

	// add a podman controller w/ default podman url
	var podmanURL string
	if len(os.Args) >= 3 {
		podmanURL = os.Args[2]
	}
	if podmanURL != "" {
		le.Infof("starting podman client for url: %s", podmanURL)
		_, clientRef, err := podman_client.StartControllerWithConfig(ctx, tb.Bus, &podman_client.Config{
			EngineId: "podman/client",
			Url:      podmanURL,
		})
		if err != nil {
			return err
		}
		defer clientRef.Release()
	}

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
