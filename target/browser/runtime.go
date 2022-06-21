//go:build js
// +build js

package browser

import (
	"context"
	"syscall/js"

	message_port "github.com/aperturerobotics/bldr/web/ipc/message-port"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// Runtime is the alias to the remote runtime type.
type Runtime = web_runtime.Remote

// NewRuntime constructs the remote web runtime.
func NewRuntime(ctx context.Context, le *logrus.Entry, b bus.Bus, runtimeID string, messagePort js.Value) (r *Runtime, rerr error) {
	defer func() {
		if err := recover(); err != nil {
			rerr = err.(error)
		}
	}()
	ch, err := message_port.NewMessagePort(ctx, messagePort)
	if err != nil {
		return nil, err
	}
	return web_runtime.NewRemote(le, b, runtimeID, ch)
}
