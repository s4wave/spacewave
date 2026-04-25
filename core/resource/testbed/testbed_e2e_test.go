package resource_testbed_test

import (
	"context"
	"testing"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/sirupsen/logrus"
)

func TestTestbedE2EWazeroQuickjs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Run the TypeScript test using the wrapper API
	success, errorMsg, err := resource_testbed.RunTypeScriptTest(
		ctx,
		le,
		"testbed-e2e-plugin",
		"testbed-e2e.ts",
	)
	if err != nil {
		t.Fatalf("error running test: %v", err)
	}

	if !success {
		t.Fatalf("test failed: %s", errorMsg)
	}

	t.Log("test completed successfully")
}
