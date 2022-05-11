//go:build js
// +build js

package browser

import (
	"context"
	"strings"

	broadcast_channel "github.com/aperturerobotics/bldr/web/ipc/broadcast-channel"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// Runtime is the alias to the remote runtime type.
type Runtime = web_runtime.Remote

// NewRuntime constructs the remote web runtime.
func NewRuntime(ctx context.Context, le *logrus.Entry, b bus.Bus, runtimeID, workerUuid string) (*Runtime, error) {
	rxID := strings.Join([]string{web_runtime.Prefix, runtimeID, "r"}, "/")
	txID := strings.Join([]string{web_runtime.Prefix, runtimeID, "w"}, "/")
	ch := broadcast_channel.NewBroadcastChannel(ctx, txID, rxID)
	return web_runtime.NewRemote(le, b, runtimeID, ch)
}
