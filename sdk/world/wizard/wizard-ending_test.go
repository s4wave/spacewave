package s4wave_wizard

import (
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// TestWizardEndingLifetime checks that an ending reads only for the wizard
// that recorded it: the ending ends with its target, and a wizard created again
// at the same key starts with none.
func TestWizardEndingLifetime(t *testing.T) {
	// Start a World over the storage testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	wtb, err := world_testbed.NewTestbed(tb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()

	const wizardKey = "wizard/canvas-1"
	apply := func(op world.Operation) {
		t.Helper()
		if _, _, err := ws.ApplyWorldOp(ctx, op, sender); err != nil {
			t.Fatal(err)
		}
	}
	createWizard := func() {
		t.Helper()
		apply(NewCreateWizardObjectOp(wizardKey, "wizard/canvas", "canvas", "canvas/", "Canvas", time.Now()))
	}
	createTarget := func(key string) {
		t.Helper()
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	// finish records the ending and removes the wizard, as a Finish list does.
	finish := func(targetKey string) {
		t.Helper()
		apply(&RecordWizardEndingOp{WizardKey: wizardKey, TargetKey: targetKey})
		if _, err := ws.DeleteObject(ctx, wizardKey); err != nil {
			t.Fatal(err)
		}
	}
	expectEnding := func(want string) {
		t.Helper()
		got, ok, err := LookupWizardEnding(ctx, ws, wizardKey)
		if err != nil {
			t.Fatal(err)
		}
		if got != want || ok != (want != "") {
			t.Fatalf("ending: got %q (%v), want %q", got, ok, want)
		}
	}

	// A Finish records its target on the typed endings object.
	createWizard()
	createTarget("canvas/one")
	finish("canvas/one")
	expectEnding("canvas/one")
	typeID, err := world_types.GetObjectType(ctx, ws, WizardEndingsObjectKey)
	if err != nil || typeID != WizardEndingsTypeID {
		t.Fatalf("endings type: got %q, %v", typeID, err)
	}

	// A wizard created again at the key is not the one that finished, and
	// deleting it without a Finish leaves it deleted.
	createWizard()
	expectEnding("")
	if _, err := ws.DeleteObject(ctx, wizardKey); err != nil {
		t.Fatal(err)
	}
	expectEnding("")

	// Deleting the target ends the ending: the wizard then reads as deleted,
	// not as finished as an object that is gone.
	createWizard()
	createTarget("canvas/two")
	finish("canvas/two")
	expectEnding("canvas/two")
	if _, err := ws.DeleteObject(ctx, "canvas/two"); err != nil {
		t.Fatal(err)
	}
	expectEnding("")

	// A Finish whose target is missing records nothing rather than failing.
	createWizard()
	finish("canvas/missing")
	expectEnding("")
}
