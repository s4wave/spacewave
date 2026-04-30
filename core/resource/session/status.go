package resource_session

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
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

// WatchPlugins streams the list of active plugin load requests on change.
func (r *StatusResource) WatchPlugins(
	_ *s4wave_status.WatchPluginsRequest,
	strm s4wave_status.SRPCSystemStatusService_WatchPluginsStream,
) error {
	bcast := r.b.GetDirectivesBroadcast()
	return broadcast.WatchBroadcastVT(
		strm.Context(),
		bcast,
		func() *s4wave_status.WatchPluginsResponse {
			dirs := r.b.GetDirectives()
			seen := make(map[string]struct{}, len(dirs))
			infos := make([]*s4wave_status.PluginInfo, 0, len(dirs))
			for _, d := range dirs {
				lp, ok := d.GetDirective().(bldr_plugin.LoadPlugin)
				if !ok {
					continue
				}
				id := lp.LoadPluginID()
				instanceKey := lp.LoadPluginInstanceKey()
				key := id + "\x00" + instanceKey
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				infos = append(infos, &s4wave_status.PluginInfo{
					Id:          id,
					InstanceKey: instanceKey,
					State:       "requested",
				})
			}
			return &s4wave_status.WatchPluginsResponse{
				Plugins:     infos,
				PluginCount: uint32(len(infos)),
			}
		},
		func(resp *s4wave_status.WatchPluginsResponse) error {
			return strm.Send(resp)
		},
	)
}

// _ is a type assertion
var _ s4wave_status.SRPCSystemStatusServiceServer = ((*StatusResource)(nil))
