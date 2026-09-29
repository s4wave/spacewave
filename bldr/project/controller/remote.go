//go:build !js

package bldr_project_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/db/world"
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
	// Publish the result promise so waiters observe every outcome.
	t.c.le.WithField("remote-id", t.remoteID).Debug("remote tracker starting")
	resultPromise := promise.NewPromise[*world.Engine]()
	t.resultPromise.SetPromise(resultPromise)
	if err := t.remote.Validate(); err != nil {
		err := errors.Wrap(err, "invalid remote config")
		resultPromise.SetResult(nil, err)
		return err
	}

	// Apply the remote's host config set, releasing it when the tracker exits.
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

	// Look up the World engine handle and publish it as the tracker result.
	// build world engine handle
	worldEngineID := t.remote.GetEngineId()
	engineHandle, _, engineRef, err := world.ExLookupWorldEngine(ctx, t.c.bus, false, worldEngineID, nil)
	if err != nil {
		resultPromise.SetResult(nil, err)
		return err
	}
	defer engineRef.Release()

	// Publish the engine handle as the tracker result.
	engine := engineHandle
	resultPromise.SetResult(&engine, nil)

	// Hold the engine until the tracker's context is canceled.
	<-ctx.Done()
	t.c.le.WithField("remote-id", t.remoteID).Debug("remote tracker exiting")
	return nil
}
