package bifrost_http

import (
	"context"
	"net/url"
	"testing"

	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

func TestHTTPBusHTTPHandler(t *testing.T) {
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

	// start the on-demand handler
	u, err := url.Parse("/foo")
	if err != nil {
		t.Fatal(err.Error())
	}
	rc := NewBusHTTPHandler(ctx, tb.Bus, "", u, "test-client", false)

	// perform a request
	checkMockRequest(t, rc)
}
