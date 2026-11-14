//go:build !js

package drivebench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// ResourceConn summarizes the Resource SDK connection timing for the session.
// The bundled production bench leaves it zero because it has no SDK client.
type ResourceConn struct {
	// DurationMs is how long the SDK connection took.
	DurationMs int64 `json:"durationMs"`
	// Attempts counts the connection attempts.
	Attempts int `json:"attempts"`
	// StartupReloads counts the page reloads during startup.
	StartupReloads int `json:"startupReloads"`
}

func marshalResourceConnValue(arena *fastjson.Arena, conn ResourceConn) *fastjson.Value {
	// Encode the Resource SDK connection timing and attempt counts.
	obj := arena.NewObject()
	obj.Set("durationMs", arena.NewNumberString(strconv.FormatInt(conn.DurationMs, 10)))
	obj.Set("attempts", arena.NewNumberInt(conn.Attempts))
	obj.Set("startupReloads", arena.NewNumberInt(conn.StartupReloads))
	return obj
}
