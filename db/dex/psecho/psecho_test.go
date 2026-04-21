package psecho_test

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/bucket"
	ee "github.com/s4wave/spacewave/db/dex/e2e"
	"github.com/s4wave/spacewave/db/dex/psecho"
	"github.com/s4wave/spacewave/db/testbed"
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
