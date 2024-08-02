package psecho_test

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	csp "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/bucket"
	lc "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	ee "github.com/aperturerobotics/hydra/dex/e2e"
	"github.com/aperturerobotics/hydra/dex/psecho"
	"github.com/aperturerobotics/hydra/testbed"
)

func TestPsechoE2E_DEX(t *testing.T) {
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
			t.StaticResolver.AddFactory(psecho.NewFactory(t.Bus))
			return []config.Config{
				&psecho.Config{
					BucketId:      bc.GetId(),
					PubsubChannel: "test-ps-channel",
				},
			}, nil
		},
	)
}
