package plugin_entrypoint

import (
	"context"

	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// StartCoreBus builds the bus & starts common controllers.
func StartCoreBus(
	ctx context.Context,
	le *logrus.Entry,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
) (b bus.Bus, sr *static.Resolver, rel func(), err error) {
	var rels []func()
	rel = func() {
		for _, rel := range rels {
			rel()
		}
	}

	b, sr, err = core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, fn := range addFactoryFuncs {
		if fn != nil {
			for _, factory := range fn(b) {
				sr.AddFactory(factory)
			}
		}
	}

	// load configset controller
	_, _, csRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "construct configset controller")
	}
	rels = append(rels, csRef.Release)

	// load root config sets
	var configSets []configset.ConfigSet
	for _, configSetFn := range configSetFuncs {
		confSets, err := configSetFn(ctx, b, le)
		if err != nil {
			rel()
			return nil, nil, nil, err
		}
		configSets = append(configSets, confSets...)
	}

	// apply config sets
	mergedConfigSet := configset.MergeConfigSets(configSets...)
	if len(mergedConfigSet) != 0 {
		_, csetRef, err := b.AddDirective(configset.NewApplyConfigSet(mergedConfigSet), nil)
		if err != nil {
			rel()
			return nil, nil, nil, err
		}
		rels = append(rels, csetRef.Release)
	}

	return b, sr, rel, nil
}
