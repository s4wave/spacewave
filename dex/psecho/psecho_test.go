package psecho_test

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
	ee "github.com/aperturerobotics/hydra/dex/e2e"
	"github.com/aperturerobotics/hydra/dex/psecho"
	"github.com/aperturerobotics/hydra/testbed"
)

func TestPsechoE2E_DEX(t *testing.T) {
	ee.TestMultiNodeDEX(
		t,
		nil,
		func(t *testbed.Testbed, bc *bucket.Config) ([]config.Config, error) {
			t.StaticResolver.AddFactory(psecho.NewFactory(t.Bus))
			return []config.Config{
				&psecho.Config{
					BucketId:        bc.GetId(),
					PubsubChannelId: "test-dex-channel",
				},
			}, nil
		},
	)
}
