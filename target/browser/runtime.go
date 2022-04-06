//go:build js
// +build js

package browser

import (
	"context"

	storage "github.com/aperturerobotics/bldr/storage/browser"
	broadcast_channel "github.com/aperturerobotics/bldr/web/ipc/broadcast-channel"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// Runtime is the alias to the remote runtime type.
type Runtime = web_runtime.Remote

// NewRuntime constructs the remote web runtime.
func NewRuntime(ctx context.Context, le *logrus.Entry, b bus.Bus, id string) (*Runtime, error) {
	txID := web_runtime.Prefix + "/w/" + id
	rxID := web_runtime.Prefix + "/r/" + id
	st := storage.BuildStorage(b, "")
	ch := broadcast_channel.NewBroadcastChannel(ctx, txID, rxID)
	return web_runtime.NewRemote(le, b, id, st, ch)
}
