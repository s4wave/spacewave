package volume_kvtxinmem

import (
	"context"
	"testing"

	volume_test "github.com/aperturerobotics/hydra/volume/test"
	"github.com/sirupsen/logrus"
)

// TestKVTxInmem runs the basic volume test suite.
func TestKVTxInmem(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	vol, err := NewKVTxInmem(ctx, le, &Config{
		Verbose: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := volume_test.CheckVolume(ctx, le, vol); err != nil {
		t.Fatal(err.Error())
	}
}
