package world_vlogger_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	core_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/db/world/testbed"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

// TestWorldVlogger tests the world engine w/ vlogger enabled.
func TestWorldVlogger(t *testing.T) {
	// Open a World testbed with verbose transaction logging.
	ctx := context.Background()
	tb, err := testbed.Default(ctx, testbed.WithWorldVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the World engine contract through the logging wrapper.
	// basic sanity tests
	le, eng := tb.Logger, tb.Engine
	err = world_mock.TestWorldEngine_Basic(ctx, le, eng)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Report completion of the World engine checks.
	// success
	t.Log("tests successful")
}

func TestWorldVloggerRedactsObjectKeys(t *testing.T) {
	// Capture verbose World logs for the object-key redaction checks.
	ctx := context.Background()
	logBuf := bytes.NewBuffer(nil)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(logBuf)
	le := logrus.NewEntry(log)

	// Open the storage and World testbeds for the logging wrapper.
	coreTB, err := core_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer coreTB.Release()
	tb, err := testbed.NewTestbed(coreTB)
	if err != nil {
		t.Fatal(err)
	}

	// Create a World object whose key must remain absent from logs.
	const secretObjectKey = "secrets/ssh/password"
	ws := world_vlogger.NewWorldState(le, tb.WorldState)
	obj, err := ws.CreateObject(ctx, secretObjectKey, nil)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Exercise root reads, object lookup, and graph deletion through the logger.
	if _, _, err := obj.GetRootRef(ctx); err != nil {
		t.Fatal(err)
	}
	{
		objectState, _, err := ws.GetObject(ctx, secretObjectKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := ws.DeleteGraphObject(ctx, secretObjectKey); err != nil {
		t.Fatal(err)
	}

	// Verify the World logs summarize the key without exposing its contents.
	output := logBuf.String()
	if strings.Contains(output, secretObjectKey) {
		t.Fatalf("world vlogger exposed object key in logs: %s", output)
	}
	if !strings.Contains(output, "len=") {
		t.Fatalf("world vlogger did not include structural key summary: %s", output)
	}
}
