package e2e_test

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/bucket"
	ee "github.com/s4wave/spacewave/db/dex/e2e"
	"github.com/s4wave/spacewave/db/dex/psecho"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/db/testbed"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
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
