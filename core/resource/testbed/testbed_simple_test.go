package resource_testbed_test

import (
	"context"
	"testing"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/sirupsen/logrus"
)

// TestTestbedSimpleWrapper demonstrates the new simple wrapper API for TypeScript E2E tests.
// This uses RunTypeScriptTest() which handles all the boilerplate setup.
func TestTestbedSimpleWrapper(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Run the TypeScript test using the simple wrapper API
	success, errorMsg, err := resource_testbed.RunTypeScriptTest(
		ctx,
		le,
		"testbed-e2e-simple",
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
