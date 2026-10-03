//go:build !skip_e2e && !js

package resource_testbed_test

import (
	"context"
	"testing"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/sirupsen/logrus"
)

func TestTestbedE2EWazeroQuickjs(t *testing.T) {
	// Configure logging for the QuickJS testbed integration test.
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

	// Require the TypeScript testbed integration test to succeed.
	if !success {
		t.Fatalf("test failed: %s", errorMsg)
	}

	// Report successful QuickJS testbed integration.
	t.Log("test completed successfully")
}

func TestTestbedE2EWazeroQuickjsUnixFSTypedObject(t *testing.T) {
	// Configure logging for the QuickJS UnixFS integration test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Run the UnixFS typed object test through the QuickJS wrapper.
	success, errorMsg, err := resource_testbed.RunTypeScriptTest(
		ctx,
		le,
		"testbed-unixfs-typed-object-plugin",
		"testbed-unixfs-typed-object.ts",
	)
	if err != nil {
		t.Fatalf("error running test: %v", err)
	}

	// Require the UnixFS typed object integration test to succeed.
	if !success {
		t.Fatalf("test failed: %s", errorMsg)
	}

	// Report successful QuickJS UnixFS integration.
	t.Log("test completed successfully")
}
