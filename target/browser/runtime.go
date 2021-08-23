//go:build js
// +build js

package browser

import (
	"context"

	broadcast_channel "github.com/aperturerobotics/bldr/runtime/ipc/broadcast-channel"
	"github.com/aperturerobotics/bldr/runtime/web"
	storage "github.com/aperturerobotics/bldr/target/browser/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// Runtime is the alias to the remote runtime type.
type Runtime = web.Remote

// NewRuntime constructs the remote web runtime.
func NewRuntime(ctx context.Context, le *logrus.Entry, b bus.Bus, id string) (*Runtime, error) {
	txID := web.Prefix + "/r/" + id
	rxID := web.Prefix + "/runtime"
	st := storage.BuildStorage(b)
	ch := broadcast_channel.NewBroadcastChannel(ctx, txID, rxID)
	return web.NewRemote(le, b, id, st, ch)
}
