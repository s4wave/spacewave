package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"os/signal"
	"time"

	random_id "github.com/aperturerobotics/bifrost/util/randstring"
	podman_client "github.com/aperturerobotics/containers/podman/client"
	forge_job "github.com/aperturerobotics/forge/job"
	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	forge_worker "github.com/aperturerobotics/forge/worker"
	hcli "github.com/aperturerobotics/hydra/cli"
	hydra_testbed "github.com/aperturerobotics/hydra/testbed"
	world "github.com/aperturerobotics/hydra/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/timestamp"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type hDaemonArgs = hcli.DaemonArgs

var progFlags struct {
	hDaemonArgs
}

var podmanURL string

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	dflags := (&progFlags.hDaemonArgs).BuildFlags()

	app := cli.NewApp()
	app.Name = "bundle"
	app.Usage = "run a forge worker"
	app.HideVersion = true
	app.Flags = append([]cli.Flag{
		&cli.StringFlag{
			Name:        "podman-url",
			Usage:       "podman url to connect to: like unix:///run/podman/podman.sock",
			Destination: &podmanURL,
			Value:       podmanURL,
		},
	}, dflags...)
	app.Action = func(c *cli.Context) error {
		args := c.Args().Slice()
		if len(args) == 0 {
			return errors.New("usage: ./forge ./test-target.yaml")
		}

		sctx, sctxStop := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
		defer sctxStop()

		filePath := args[len(args)-1]
		return runWorkerDemo(sctx, le, filePath)
	}

	if err := app.RunContext(ctx, os.Args); err != nil {
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

	targetData, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return err
	}

	volConfig := progFlags.hDaemonArgs.BuildSingleVolume()
	verbose := false
	tb, err := testbed.WithTestbedOptions(ctx,
		[]hydra_testbed.Option{hydra_testbed.WithVolumeConfig(volConfig)},
		[]world_testbed.Option{world_testbed.WithWorldVerbose(verbose)},
	)
	if err != nil {
		return err
	}
	forge_lib_all.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(podman_client.NewFactory(tb.Bus))

	// cleanup the world so that RunWorkerWithTasks doesn't fail:
	prefix := "run/" + random_id.RandomIdentifier(8)[:4] + "/"
	jobKey := prefix + "job/1"
	clusterKey := prefix + "cluster/1"
	keysToDelete := []string{
		jobKey,
		clusterKey,
	}

	ws := tb.WorldState
	typesState := world_types.NewTypesState(ctx, ws)
	workerKeys, err := forge_worker.ListWorkers(typesState)
	if err != nil {
		return err
	}
	for _, workerKey := range workerKeys {
		le.Infof("deleting old worker: %s", workerKey)
	}
	keysToDelete = append(keysToDelete, workerKeys...)

	// lookup and delete all tasks
	_, jobTaskKeys, err := forge_job.CollectJobTasks(ctx, ws, jobKey)
	if err != nil && err != world.ErrObjectNotFound {
		return err
	}
	keysToDelete = append(keysToDelete, jobTaskKeys...)

	for _, keyToDelete := range keysToDelete {
		deleted, err := ws.DeleteObject(keyToDelete)
		if err != nil {
			return err
		}
		if deleted {
			le.Infof("deleted object: %s", keyToDelete)
		}
	}
	<-time.After(time.Second)

	// add a podman controller w/ default podman url
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

	ts := timestamp.Now()
	taskMap, err := target_json.ResolveTargetMapYAML(ctx, tb.Bus, targetData)
	if err != nil {
		return err
	}
	_, err = tb.RunWorkerWithTasks(taskMap, nil, 1, &ts, jobKey, clusterKey)
	return err
}
