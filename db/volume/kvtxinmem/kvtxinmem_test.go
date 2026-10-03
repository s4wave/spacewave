package volume_kvtxinmem

import (
	"context"
	"testing"

	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/sirupsen/logrus"
)

// TestKVTxInmem runs the basic volume test suite.
func TestKVTxInmem(t *testing.T) {
	// Prepare the volume conformance test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Exercise the in-memory volume through the shared conformance suite.
	vol, err := NewKVTxInmem(ctx, le, &Config{
		Verbose: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := volume_test.CheckVolume(ctx, vol); err != nil {
		t.Fatal(err.Error())
	}
}
