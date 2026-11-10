package kvtx

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestListRootPins pins a root and writes a build through an open stage, and
// checks the list names the pinned root and the stage's root with their open
// times, then drops each once released.
func TestListRootPins(t *testing.T) {
	// Pin a stored root, then write a build through a stage.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	start := time.Now()
	pinned, _, err := v.PrepareOwnedBlock(ctx, "bucket", []byte("a root a reader holds"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	releasePin, err := v.PinBucketRoot(ctx, pinned)
	if err != nil {
		t.Fatal(err)
	}
	_, releaseStage, root, _ := stageBuild(t, v, "staged")

	// Both are listed, each with an open time from this test.
	list := func() map[string]RootPin {
		t.Helper()
		pins, err := v.ListRootPins(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := make(map[string]RootPin, len(pins))
		for _, pin := range pins {
			if pin.Opened.Before(start) || pin.Opened.After(time.Now()) {
				t.Fatalf("pin %s opened %v, outside the test", pin.Root.MarshalString(), pin.Opened)
			}
			out[pin.Root.MarshalString()] = pin
		}
		return out
	}
	pins := list()
	if pin, ok := pins[pinned.MarshalString()]; !ok || pin.Stage != "" {
		t.Fatalf("pinned root listed as %+v, %v", pin, ok)
	}
	if pin, ok := pins[root.MarshalString()]; !ok || pin.Stage == "" {
		t.Fatalf("staged root listed as %+v, %v", pin, ok)
	}

	// Each release drops its entry.
	releasePin()
	if _, ok := list()[pinned.MarshalString()]; ok {
		t.Fatal("a released pin is listed")
	}
	releaseStage()
	if _, ok := list()[root.MarshalString()]; ok {
		t.Fatal("a released stage is listed")
	}
}
