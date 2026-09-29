package spacewave_launcher_controller

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/localdist"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/peer"
)

// defVolumeID is the default volume ID for the launcher.
const defVolumeID = "plugin-host"

// defObjectStoreID is the default object store ID for the launcher.
var defObjectStoreID = "spacewave/launcher"

// defObjectStoreKey is the default key used to store the distribution config packedmsg.
var defObjectStoreKey = "dist-conf"

// GetVolumeId returns volume the controller uses for storage.
func (c *Controller) GetVolumeId() string {
	id := c.conf.GetVolumeId()
	if id == "" {
		id = defVolumeID
	}
	return id
}

// GetObjectStoreId returns the object store id the controller uses for storage.
func (c *Controller) GetObjectStoreId() string {
	id := c.conf.GetObjectStoreId()
	if id == "" {
		id = defObjectStoreID
	}
	return id
}

// GetObjectStoreKey returns the key used for the dist conf in the object store.
func (c *Controller) GetObjectStoreKey() string {
	id := c.conf.GetObjectStoreKey()
	if id == "" {
		id = defObjectStoreKey
	}
	return id
}

// parseDistConf parses and checks a dist config packed message.
func (c *Controller) parseDistConf(distConfDat []byte) (*spacewave_launcher.DistConfig, string, peer.ID, error) {
	distConf, distConfPackedMsg, distConfSigner, err := spacewave_launcher.ParseDistConfigPackedMsg(
		c.le,
		distConfDat,
		c.distPeerIDs,
		c.conf.GetProjectId(),
	)
	if err == nil {
		err = c.conf.ValidateDistChannel(distConf)
	}
	if err != nil {
		return nil, "", "", err
	}
	return distConf, distConfPackedMsg, distConfSigner, nil
}

// loadDistConf loads the current dist conf from the store.
// returns empty if not found.
// note: returns a packed signed message
func (c *Controller) loadDistConf(ctx context.Context) ([]byte, error) {
	store, ref, err := c.openObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	var data []byte
	objs := store.GetObjectStore()
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objs.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			data, _, err = tx.Get(ctx, []byte(c.GetObjectStoreKey()))
			return err
		},
	)
	return data, err
}

// loadLocalDistConf loads a package-shipped dist config next to the daemon
// executable.
func (c *Controller) loadLocalDistConf() ([]byte, string, error) {
	if runtime.GOOS == "js" {
		return nil, "", nil
	}
	exePath, err := bldr_plugin.HostExecutable()
	if err != nil {
		return nil, "", err
	}
	resolvedExePath, err := filepath.EvalSymlinks(exePath)
	if err == nil && resolvedExePath != "" {
		exePath = resolvedExePath
	}
	return localdist.Read(localdist.Paths(exePath))
}

// storeDistConf stores an updated dist conf to the store.
// note: accepts a packed signed message
func (c *Controller) storeDistConf(ctx context.Context, data []byte) error {
	store, ref, err := c.openObjectStore(ctx)
	if err != nil {
		return err
	}
	defer ref.Release()

	objs := store.GetObjectStore()
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objs.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, []byte(c.GetObjectStoreKey()), data)
		},
	)
}

// openObjectStore opens the handle to the object store api.
func (c *Controller) openObjectStore(ctx context.Context) (volume.BuildObjectStoreAPIValue, directive.Reference, error) {
	objStoreID := c.GetObjectStoreId()
	volID := c.GetVolumeId()
	val, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, c.bus, false, objStoreID, volID, nil)
	return val, ref, err
}
