package plugin_entrypoint_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// hostPluginScheduler is the host scheduler that runs this plugin, as seen
// through the PluginHost WatchPluginStatus stream.
type hostPluginScheduler struct {
	// instanceKey is the host scheduler's instance key.
	instanceKey string
	// statusCtr holds the latest snapshot received from the host.
	statusCtr *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot]
}

// GetInstanceKey returns the host scheduler's instance key.
func (s *hostPluginScheduler) GetInstanceKey() string {
	return s.instanceKey
}

// GetPluginStatusCtr returns the host scheduler's live plugin-status snapshot.
func (s *hostPluginScheduler) GetPluginStatusCtr() ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot] {
	return s.statusCtr
}

// lookupPluginSchedulerResolver resolves LookupPluginScheduler with the host
// scheduler while the WatchPluginStatus stream stays open.
type lookupPluginSchedulerResolver struct {
	// c is the controller
	c *Controller
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupPluginSchedulerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	strm, err := r.c.srv.WatchPluginStatus(ctx, &bldr_plugin.WatchPluginStatusRequest{})
	if err != nil {
		return err
	}
	defer strm.Close()

	var scheduler *hostPluginScheduler
	var valID uint32
	for {
		msg, err := strm.Recv()
		if err != nil {
			if valID != 0 {
				_, _ = handler.RemoveValue(valID)
			}
			return err
		}

		status := msg.GetStatus()
		if status == nil {
			status = &bldr_plugin.PluginStatusSnapshot{}
		}
		if scheduler != nil {
			scheduler.statusCtr.SetValue(status)
			continue
		}

		scheduler = &hostPluginScheduler{
			instanceKey: msg.GetInstanceKey(),
			statusCtr:   ccontainer.NewCContainer(status),
		}
		var accepted bool
		valID, accepted = handler.AddValue(bldr_plugin.LookupPluginSchedulerValue(scheduler))
		if !accepted {
			return nil
		}
		handler.MarkIdle(true)
	}
}

// _ is a type assertion
var (
	_ bldr_plugin.PluginScheduler = (*hostPluginScheduler)(nil)
	_ directive.Resolver          = (*lookupPluginSchedulerResolver)(nil)
)
