package plugin_host

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
)

// HandleLoadPluginRpc handles an incoming LoadPlugin RPC request.
func HandleLoadPluginRpc(
	b bus.Bus,
	req *bldr_plugin.LoadPluginRequest,
	strm bldr_plugin.SRPCPluginHost_LoadPluginStream,
) error {
	pluginID := req.GetPluginId()
	dir := bldr_plugin.NewLoadPlugin(pluginID)
	resp := ccontainer.NewCContainerVT[*bldr_plugin.LoadPluginResponse](nil)

	errCh := make(chan error, 1)
	pushErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	ctx := strm.Context()
	reqCtx, reqCtxCancel := context.WithCancel(ctx)
	defer reqCtxCancel()

	var vals []directive.AttachedValue
	updResp := func() {
		resp.SetValue(&bldr_plugin.LoadPluginResponse{
			PluginStatus: &bldr_plugin.PluginStatus{
				PluginId: pluginID,
				Running:  len(vals) != 0,
			},
		})
	}

	di, ref, err := b.AddDirective(
		dir,
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				vals = append(vals, av)
				if len(vals) == 1 {
					updResp()
				}
			},
			func(av directive.AttachedValue) {
				for i, val := range vals {
					if val == av {
						vals = append(vals[:i], vals[i+1:]...)
						updResp()
						break
					}
				}
			},
			func() {
				reqCtxCancel()
			},
		),
	)
	if err != nil {
		return err
	}
	defer ref.Release()

	defer di.AddIdleCallback(func(isIdle bool, errs []error) {
		if !isIdle {
			return
		}
		for _, err := range errs {
			if err != nil && err != context.Canceled {
				pushErr(err)
				return
			}
		}
		updResp()
	})()

	var prevTx *bldr_plugin.LoadPluginResponse
	for {
		val, err := resp.WaitValueChange(reqCtx, prevTx, errCh)
		if err != nil {
			return err
		}

		prevTx = val
		if val != nil {
			if err := strm.Send(val); err != nil {
				return err
			}
		}
	}
}
