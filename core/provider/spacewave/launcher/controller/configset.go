package spacewave_launcher_controller

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/directive"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// applyDistConfigSet applies the config set from the app dist config.
func (c *Controller) applyDistConfigSet(ctx context.Context) error {
	var info *spacewave_launcher.LauncherInfo
	var currRef directive.Reference
	var currCs *configset_proto.ConfigSet
	defer func() {
		if currRef != nil {
			currRef.Release()
		}
	}()

	for {
		var err error
		info, err = c.launcherInfoCtr.WaitValueChange(ctx, info, nil)
		if err != nil {
			return err
		}
		distConf := info.GetDistConfig()
		if distConf.GetRev() == 0 {
			continue
		}
		distConfCs := &configset_proto.ConfigSet{
			Configs: distConf.GetLauncherConfigSet(),
		}
		if currCs != nil && currCs.EqualVT(distConfCs) {
			// configset is identical, continue
			continue
		}
		launcherConfigSet := configset_proto.ConfigSetMap(distConf.GetLauncherConfigSet())
		if err := launcherConfigSet.Validate(); err != nil {
			c.le.WithError(err).Warn("ignoring invalid launcher config set from dist config")
			launcherConfigSet = nil
		}
		var cs configset.ConfigSet
		if len(launcherConfigSet) != 0 {
			c.le.Debugf("resolving launcher config set with %d configs", len(launcherConfigSet))
			resolveCtx, resolveCtxCancel := context.WithCancel(ctx)
			resInfo := info
			var changed atomic.Bool
			go func() {
				// check if the value changed while we were Resolving
				_, _ = c.launcherInfoCtr.WaitValueChange(resolveCtx, resInfo, nil)
				changed.Store(true)
				resolveCtxCancel()
			}()
			cs, err = launcherConfigSet.Resolve(resolveCtx, c.bus)
			resolveCtxCancel()
			if err != nil {
				if !changed.Load() {
					c.le.WithError(err).Warn("unable to resolve launcher config set")
				}
				continue
			}
		}

		// apply next config set
		var nextRef directive.Reference
		if len(cs) != 0 {
			var err error
			_, nextRef, err = c.bus.AddDirective(configset.NewApplyConfigSet(cs), nil)
			if err != nil {
				c.le.WithError(err).Warn("unable to apply launcher config set")
				continue
			}
		}

		// update refs
		if currRef != nil {
			currRef.Release()
		}
		currRef = nextRef
		currCs = distConfCs
		c.le.Debugf("applied launcher config set with %d configs", len(cs))
	}
}
