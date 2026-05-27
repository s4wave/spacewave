//go:build js && !tinygo

package web_runtime_wasm

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

func newTinyGoPluginOpenStream(
	context.Context,
	srpc.PacketDataHandler,
	srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	return nil, errors.New("tinygo plugin stream bridge unavailable")
}

func setTinyGoPluginAcceptStreams(context.Context, srpc.Invoker) {}
