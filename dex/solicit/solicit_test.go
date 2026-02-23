package dex_solicit_test

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/bucket"
	lc "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	ee "github.com/aperturerobotics/hydra/dex/e2e"
	dex_solicit "github.com/aperturerobotics/hydra/dex/solicit"
	"github.com/aperturerobotics/hydra/testbed"

	link_solicit_controller "github.com/aperturerobotics/bifrost/link/solicit/controller"
)

func TestSolicitE2E_DEX(t *testing.T) {
	ee.TestMultiNodeDEX(
		t,
		func(bc *bucket.Config) error {
			lookupConf := &lc.Config{
				NotFoundBehavior:  lc.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
				PutBlockBehavior:  lc.PutBlockBehavior_PutBlockBehavior_ALL,
				WritebackBehavior: lc.WritebackBehavior_WritebackBehavior_ALL,
			}
			cc, err := csp.NewControllerConfig(configset.NewControllerConfig(1, lookupConf), false)
			if err != nil {
				return err
			}
			bc.Lookup = &bucket.LookupConfig{
				Controller: cc,
			}
			return nil
		},
		func(t *testbed.Testbed, bc *bucket.Config) ([]config.Config, error) {
			// Register factories for solicitation and DEX controllers.
			t.StaticResolver.AddFactory(link_solicit_controller.NewFactory())
			t.StaticResolver.AddFactory(dex_solicit.NewFactory(t.Bus))
			return []config.Config{
				&link_solicit_controller.Config{},
				&dex_solicit.Config{
					BucketId: bc.GetId(),
				},
			}, nil
		},
	)
}
