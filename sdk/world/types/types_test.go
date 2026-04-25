package world_types_test

import (
	"context"
	"testing"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/sirupsen/logrus"
)

// TestWorldTypes tests the world types functionality using the TypeScript SDK.
// This test validates type management operations including:
// - Setting and getting object types
// - Checking object types
// - Listing objects by type
// - Type object creation
// - Error handling
func TestWorldTypes(t *testing.T) {
	t.Skip("blocked: testbed resource routing needs bldr plugin host changes")
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Run the TypeScript test using the wrapper API
	success, errorMsg, err := resource_testbed.RunTypeScriptTest(
		ctx,
		le,
		"world-types-testbed",
		"types-testbed.ts",
	)
	if err != nil {
		t.Fatalf("error running test: %v", err)
	}

	if !success {
		t.Fatalf("test failed: %s", errorMsg)
	}

	t.Log("world types test completed successfully")
}
