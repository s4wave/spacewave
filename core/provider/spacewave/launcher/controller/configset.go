package spacewave_launcher_controller

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/directive"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

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
		distConfCs := &configset_proto.ConfigSet{Configs: distConf.GetLauncherConfigSet()}
		if currCs != nil && currCs.EqualVT(distConfCs) {
			continue
		}

		launcherConfigSet := configset_proto.ConfigSetMap(distConf.GetLauncherConfigSet())
		if err := launcherConfigSet.Validate(); err != nil {
			c.le.WithError(err).Warn("ignoring invalid launcher config set")
			launcherConfigSet = nil
		}

		var cs configset.ConfigSet
		if len(launcherConfigSet) != 0 {
			resolveCtx, cancel := context.WithCancel(ctx)
			resInfo := info
			var changed atomic.Bool
			go func() {
				_, _ = c.launcherInfoCtr.WaitValueChange(resolveCtx, resInfo, nil)
				changed.Store(true)
				cancel()
			}()
			cs, err = launcherConfigSet.Resolve(resolveCtx, c.bus)
			cancel()
			if err != nil {
				if !changed.Load() {
					c.le.WithError(err).Warn("unable to resolve launcher config set")
				}
				continue
			}
		}

		var nextRef directive.Reference
		if len(cs) != 0 {
			_, nextRef, err = c.bus.AddDirective(configset.NewApplyConfigSet(cs), nil)
			if err != nil {
				c.le.WithError(err).Warn("unable to apply launcher config set")
				continue
			}
		}
		if currRef != nil {
			currRef.Release()
		}
		currRef = nextRef
		currCs = distConfCs
	}
}
