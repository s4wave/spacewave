package world_vlogger_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	core_testbed "github.com/s4wave/spacewave/db/testbed"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/db/world/testbed"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

// TestWorldVlogger tests the world engine w/ vlogger enabled.
func TestWorldVlogger(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx, testbed.WithWorldVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	// basic sanity tests
	le, eng := tb.Logger, tb.Engine
	err = world_mock.TestWorldEngine_Basic(ctx, le, eng)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}

func TestWorldVloggerRedactsObjectKeys(t *testing.T) {
	ctx := context.Background()
	logBuf := bytes.NewBuffer(nil)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(logBuf)
	le := logrus.NewEntry(log)

	coreTB, err := core_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer coreTB.Release()
	tb, err := testbed.NewTestbed(coreTB)
	if err != nil {
		t.Fatal(err)
	}

	const secretObjectKey = "secrets/ssh/password"
	ws := world_vlogger.NewWorldState(le, tb.WorldState)
	obj, err := ws.CreateObject(ctx, secretObjectKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := obj.GetRootRef(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.GetObject(ctx, secretObjectKey); err != nil {
		t.Fatal(err)
	}
	if err := ws.DeleteGraphObject(ctx, secretObjectKey); err != nil {
		t.Fatal(err)
	}

	output := logBuf.String()
	if strings.Contains(output, secretObjectKey) {
		t.Fatalf("world vlogger exposed object key in logs: %s", output)
	}
	if !strings.Contains(output, "len=") {
		t.Fatalf("world vlogger did not include structural key summary: %s", output)
	}
}
