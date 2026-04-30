package resource_session

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

// StatusResource implements the SystemStatusService for a session.
type StatusResource struct {
	b bus.Bus
}

// NewStatusResource creates a new StatusResource.
func NewStatusResource(b bus.Bus) *StatusResource {
	return &StatusResource{b: b}
}

// WatchControllers streams the list of active controllers on change.
func (r *StatusResource) WatchControllers(
	_ *s4wave_status.WatchControllersRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchControllersStream,
) error {
	bcast := r.b.GetControllersBroadcast()
	return broadcast.WatchBroadcastVT(
		strm.Context(),
		bcast,
		func() *s4wave_status.WatchControllersResponse {
			ctrls := r.b.GetControllers()
			infos := make([]*s4wave_status.ControllerInfo, len(ctrls))
			for i, c := range ctrls {
				ci := c.GetControllerInfo()
				infos[i] = &s4wave_status.ControllerInfo{
					Id:          ci.GetId(),
					Version:     ci.GetVersion(),
					Description: ci.GetDescription(),
				}
			}
			return &s4wave_status.WatchControllersResponse{
				Controllers:     infos,
				ControllerCount: uint32(len(infos)),
			}
		},
		func(resp *s4wave_status.WatchControllersResponse) error {
			return strm.Send(resp)
		},
	)
}

// WatchDirectives streams the list of active directives on change.
func (r *StatusResource) WatchDirectives(
	_ *s4wave_status.WatchDirectivesRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchDirectivesStream,
) error {
	bcast := r.b.GetDirectivesBroadcast()
	return broadcast.WatchBroadcastVT(
		strm.Context(),
		bcast,
		func() *s4wave_status.WatchDirectivesResponse {
			dirs := r.b.GetDirectives()
			infos := make([]*s4wave_status.DirectiveInfo, len(dirs))
			for i, d := range dirs {
				infos[i] = &s4wave_status.DirectiveInfo{
					Name:  d.GetDirective().GetName(),
					Ident: d.GetDirectiveIdent(),
				}
			}
			return &s4wave_status.WatchDirectivesResponse{
				Directives:     infos,
				DirectiveCount: uint32(len(infos)),
			}
		},
		func(resp *s4wave_status.WatchDirectivesResponse) error {
			return strm.Send(resp)
		},
	)
}

// WatchPlugins streams the plugin host scheduler's live plugin instances.
func (r *StatusResource) WatchPlugins(
	_ *s4wave_status.WatchPluginsRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchPluginsStream,
) error {
	ctx := strm.Context()
	for {
		statusCtr, err := r.waitPluginStatusCtr(ctx)
		if err != nil {
			return err
		}
		current := statusCtr.GetValue()
		if err := strm.Send(buildPluginsResponse(current)); err != nil {
			return err
		}
		if err := ccontainer.WatchChanges(
			ctx,
			current,
			statusCtr,
			func(snapshot *plugin_host_scheduler.PluginStatusSnapshot) error {
				return strm.Send(buildPluginsResponse(snapshot))
			},
			nil,
		); err != nil {
			if ctx.Err() != nil {
				return err
			}
		}
	}
}

func (r *StatusResource) waitPluginStatusCtr(
	ctx context.Context,
) (ccontainer.Watchable[*plugin_host_scheduler.PluginStatusSnapshot], error) {
	for {
		if ctr := r.findPluginStatusCtr(); ctr != nil {
			return ctr, nil
		}
		var waitCh <-chan struct{}
		r.b.GetControllersBroadcast().HoldLock(func(
			broadcast func(),
			getWaitCh func() <-chan struct{},
		) {
			waitCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}

func (r *StatusResource) findPluginStatusCtr() ccontainer.Watchable[*plugin_host_scheduler.PluginStatusSnapshot] {
	for _, ctrl := range r.b.GetControllers() {
		scheduler, ok := ctrl.(*plugin_host_scheduler.Controller)
		if ok {
			return scheduler.GetPluginStatusCtr()
		}
	}
	return nil
}

func buildPluginsResponse(snapshot *plugin_host_scheduler.PluginStatusSnapshot) *s4wave_status.WatchPluginsResponse {
	var infos []*s4wave_status.PluginInfo
	if snapshot != nil {
		infos = make([]*s4wave_status.PluginInfo, 0, len(snapshot.Plugins))
		for _, plugin := range snapshot.Plugins {
			infos = append(infos, &s4wave_status.PluginInfo{
				Id:          plugin.GetPluginId(),
				InstanceKey: plugin.GetInstanceKey(),
				State:       pluginStateString(plugin.GetState()),
			})
		}
	}
	return &s4wave_status.WatchPluginsResponse{
		Plugins:     infos,
		PluginCount: uint32(len(infos)),
	}
}

func pluginStateString(state bldr_plugin.PluginState) string {
	switch state {
	case bldr_plugin.PluginState_PluginState_REQUESTED:
		return "requested"
	case bldr_plugin.PluginState_PluginState_RUNNING:
		return "running"
	default:
		return "unknown"
	}
}

// _ is a type assertion
var _ s4wave_status.SRPCSystemStatusServiceServer = ((*StatusResource)(nil))
