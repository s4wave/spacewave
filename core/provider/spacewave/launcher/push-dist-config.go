package spacewave_launcher

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ExPushDistConfigMsg pushes a signed DistConfig packedmsg to the launcher
// reachable on b, waiting for its Launcher service. The launcher verifies the
// message and adopts it only when it is newer than its current config.
func ExPushDistConfigMsg(ctx context.Context, b bus.Bus, body string) (*PushDistConfigResponse, error) {
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(ctx, b, PluginLauncherServiceID, "", true, nil)
	if err != nil {
		return nil, err
	}
	if len(invokers) == 0 {
		return nil, errors.New("launcher service not found")
	}
	defer invokerRef.Release()

	client := NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
	return client.PushDistConfigMsg(ctx, &PushDistConfigRequest{Body: body})
}
