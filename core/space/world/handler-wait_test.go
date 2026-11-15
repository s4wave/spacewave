package space_world

import (
	"context"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestHandlerWaitWakesAfterEarlyEnd registers the handler between replay's
// lookup and the watch's, so the watch ends before the driver takes its wake
// channel: the wake the driver then takes must already have fired, or the
// driver sleeps on a channel nothing closes.
func TestHandlerWaitWakesAfterEarlyEnd(t *testing.T) {
	// Register the handler on a bus before the watch starts.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	b, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	release, err := b.AddController(ctx, world.NewLookupOpController("test-plugin", "", world_mock.LookupMockOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The watch ends at once, before the report takes the wake.
	w := &handlerWait{opTypeID: world_mock.MockWorldOpTypeID, cancel: cancel}
	w.watch(ctx, b, "engine")
	if !w.ended() {
		t.Fatal("the watch returned without ending the wait")
	}
	select {
	case <-w.report().Wake:
	default:
		t.Fatal("the wake taken after the watch ended never fires")
	}
}
