//go:build !js

package bldr_project_controller

import (
	"context"

	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
)

// remoteTracker tracks a running Remote config.
type remoteTracker struct {
	// c is the controller
	c *Controller
	// remoteID is the identifier of the remote to mount
	remoteID string
	// remote is the config for the remote.
	remote *bldr_project.RemoteConfig
	// resultPromise contains the result of mounting the remote.
	resultPromise *promise.PromiseContainer[*world.Engine]
}

// newRemoteTracker constructs a new remote tracker.
func (c *Controller) newRemoteTracker(key string) (keyed.Routine, *remoteTracker) {
	tr := &remoteTracker{
		c:             c,
		remoteID:      key,
		remote:        c.conf.Load().GetProjectConfig().GetRemotes()[key],
		resultPromise: promise.NewPromiseContainer[*world.Engine](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *remoteTracker) execute(ctx context.Context) error {
	t.c.le.WithField("remote-id", t.remoteID).Debug("remote tracker starting")
	resultPromise := promise.NewPromise[*world.Engine]()
	t.resultPromise.SetPromise(resultPromise)
	if err := t.remote.Validate(); err != nil {
		err := errors.Wrap(err, "invalid remote config")
		resultPromise.SetResult(nil, err)
		return err
	}

	// apply config set if necessary
	configSetMap := t.remote.GetHostConfigSet()
	if len(configSetMap) != 0 {
		// apply config set
		configSet, err := configset_proto.ConfigSetMap(configSetMap).Resolve(ctx, t.c.bus)
		if err != nil {
			resultPromise.SetResult(nil, err)
			return err
		}
		_, configSetRef, err := t.c.bus.AddDirective(configset.NewApplyConfigSet(configSet), nil)
		if err != nil {
			resultPromise.SetResult(nil, err)
			return err
		}
		defer configSetRef.Release()
	}

	// build world engine handle
	worldEngineID := t.remote.GetEngineId()
	engineHandle, _, engineRef, err := world.ExLookupWorldEngine(ctx, t.c.bus, false, worldEngineID, nil)
	if err != nil {
		resultPromise.SetResult(nil, err)
		return err
	}
	defer engineRef.Release()

	var engine world.Engine = engineHandle
	resultPromise.SetResult(&engine, nil)

	// wait for ctx to be canceled
	<-ctx.Done()
	t.c.le.WithField("remote-id", t.remoteID).Debug("remote tracker exiting")
	return nil
}
