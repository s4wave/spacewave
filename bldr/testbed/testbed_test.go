package testbed

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestTestbed(t *testing.T) {
	// Build the debug logger and the testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the testbed and verify the World engine accepts a transaction.
	tb, err := BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// b, sr := tb.GetBus(), tb.GetStaticResolver()

	// verify the world started ok
	eng := tb.GetWorldEngine()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()
}
