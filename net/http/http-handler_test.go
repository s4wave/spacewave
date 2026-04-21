package bifrost_http

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

func TestHTTPHandler(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{
		NoEcho: true,
		NoPeer: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer startMockHandler(t, tb)()

	busHandler := NewBusHandler(tb.Bus, "test-client", true)
	handler := NewHTTPHandler(ctx, NewHTTPHandlerBuilder(busHandler))

	// perform a request
	checkMockRequest(t, handler)
}
