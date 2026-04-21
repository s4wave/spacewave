package bifrost_http

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

func TestHTTPHandlerController(t *testing.T) {
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

	// start the http server
	// note: we set not found if idle because AddController waits to return
	// until the controller is added. therefore the resolution should never go
	// idle before returning a value.
	busHandler := NewBusHandler(tb.Bus, "test-client", true)

	// perform a request
	checkMockRequest(t, busHandler)
}
