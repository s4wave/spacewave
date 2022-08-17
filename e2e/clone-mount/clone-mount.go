package clone_mount

import (
	"context"
	"time"

	podman_client "github.com/aperturerobotics/containers/podman/client"
	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	"github.com/aperturerobotics/forge/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/sirupsen/logrus"
)

// Run runs the clone-mount e2e test.
func Run(ctx context.Context, le *logrus.Entry, podmanURL string) error {
	// unmarshal target from yaml into a container for later type resolution
	verbose := false
	tb, err := testbed.Default(ctx, world_testbed.WithWorldVerbose(verbose))
	if err != nil {
		return err
	}
	forge_lib_all.AddFactories(tb.Bus, tb.StaticResolver)
	tb.StaticResolver.AddFactory(podman_client.NewFactory(tb.Bus))

	le.Infof("starting podman client for url: %s", podmanURL)
	_, clientRef, err := podman_client.StartControllerWithConfig(ctx, tb.Bus, &podman_client.Config{
		EngineId: "podman/client",
		Url:      podmanURL,
	})
	if err != nil {
		return err
	}
	defer clientRef.Release()

	// TODO
	time.Sleep(time.Second)
	return nil
}
