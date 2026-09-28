package electron

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/core"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	"github.com/sirupsen/logrus"
)

// TestControllerResolvesOnlyDesktopLookup proves the controller answers the
// desktop lookup and leaves other directives on the plugin bus to their owners.
func TestControllerResolvesOnlyDesktopLookup(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	r, err := NewController(le, nil, "", "", "", "lookup", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	release, err := b.AddHandler(r)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The desktop lookup resolves to the controller.
	desktop, _, desktopRef, err := bldr_web_plugin.ExLookupDesktop(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if desktop != r {
		t.Fatalf("desktop lookup = %v, want controller", desktop)
	}
	desktopRef.Release()

	// A plugin asset lookup has no resolver here and goes idle without a value.
	access, accessRef, err := unixfs_access.ExAccessUnixFS(ctx, b, "plugin-assets/app", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if access != nil || accessRef != nil {
		t.Fatal("desktop controller resolved a filesystem lookup")
	}
}
