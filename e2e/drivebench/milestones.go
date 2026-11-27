//go:build !js

package drivebench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// Milestones records wall-clock milliseconds from navigation start to each boot
// milestone. ContentReadyMs is the moment getting-started.md is present in the
// file browser, also the moment when the first file row renders.
type Milestones struct {
	// LiveAppMs is when the app became live.
	LiveAppMs int64 `json:"liveAppMs"`
	// RouteAcceptedMs is when the app accepted the Drive route.
	RouteAcceptedMs int64 `json:"routeAcceptedMs"`
	// UnixfsVisibleMs is when the Drive file system became visible.
	UnixfsVisibleMs int64 `json:"unixfsVisibleMs"`
	// ContentReadyMs is when getting-started.md appeared in the file browser.
	ContentReadyMs int64 `json:"contentReadyMs"`
}

func marshalMilestonesValue(arena *fastjson.Arena, milestones Milestones) *fastjson.Value {
	// Encode the run's startup milestone timestamps.
	obj := arena.NewObject()
	obj.Set("liveAppMs", arena.NewNumberString(strconv.FormatInt(milestones.LiveAppMs, 10)))
	obj.Set("routeAcceptedMs", arena.NewNumberString(strconv.FormatInt(milestones.RouteAcceptedMs, 10)))
	obj.Set("unixfsVisibleMs", arena.NewNumberString(strconv.FormatInt(milestones.UnixfsVisibleMs, 10)))
	obj.Set("contentReadyMs", arena.NewNumberString(strconv.FormatInt(milestones.ContentReadyMs, 10)))
	return obj
}
