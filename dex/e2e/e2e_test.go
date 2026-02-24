package e2e_test

import (
	"testing"

	link_solicit_controller "github.com/aperturerobotics/bifrost/link/solicit/controller"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
	ee "github.com/aperturerobotics/hydra/dex/e2e"
	"github.com/aperturerobotics/hydra/dex/psecho"
	dex_solicit "github.com/aperturerobotics/hydra/dex/solicit"
	"github.com/aperturerobotics/hydra/testbed"
)

// TestCoexistenceDEX tests that both psecho and solicit backends can
// coexist on the same bucket. Either backend may resolve the block.
func TestCoexistenceDEX(t *testing.T) {
	ee.TestMultiNodeDEX(
		t,
		nil,
		func(t *testbed.Testbed, bc *bucket.Config) ([]config.Config, error) {
			t.StaticResolver.AddFactory(psecho.NewFactory(t.Bus))
			t.StaticResolver.AddFactory(link_solicit_controller.NewFactory())
			t.StaticResolver.AddFactory(dex_solicit.NewFactory(t.Bus))
			return []config.Config{
				&psecho.Config{
					BucketId:        bc.GetId(),
					PubsubChannelId: "test-dex-coexist",
				},
				&link_solicit_controller.Config{},
				&dex_solicit.Config{
					BucketId: bc.GetId(),
				},
			}, nil
		},
	)
}
