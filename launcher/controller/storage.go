package bldr_launcher_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	aperture_launcher "github.com/aperturerobotics/bldr/launcher"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/pkg/errors"
)

// defVolumeID is the default volume ID for the launcher.
var defVolumeID = bldr_plugin.PluginVolumeID

// defObjectStoreID is the default object store ID for the launcher.
var defObjectStoreID = "bldr/launcher"

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
func (c *Controller) parseDistConf(distConfDat []byte) (*aperture_launcher.DistConfig, string, peer.ID, error) {
	distConf, distConfPackedMsg, distConfSigner, err := aperture_launcher.ParseDistConfigPackedMsg(
		c.le,
		distConfDat,
		c.distPeerIDs,
		c.conf.GetProjectId(),
	)
	if err == nil && distConf.GetProjectId() != c.conf.GetProjectId() {
		err = errors.Errorf("dist conf project id mismatch: %s != expected %s", distConf.GetProjectId(), c.conf.GetProjectId())
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

	objs := store.GetObjectStore()
	tx, err := objs.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// not found: return nil
	data, _, err := tx.Get(ctx, []byte(c.GetObjectStoreKey()))
	if err != nil {
		return nil, err
	}
	return data, nil
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
	tx, err := objs.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	if err := tx.Set(ctx, []byte(c.GetObjectStoreKey()), data); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// openObjectStore opens the handle to the object store api.
func (c *Controller) openObjectStore(ctx context.Context) (volume.BuildObjectStoreAPIValue, directive.Reference, error) {
	objStoreID := c.GetObjectStoreId()
	volID := c.conf.GetVolumeId()
	val, _, ref, err := volume.BuildObjectStoreAPIEx(ctx, c.bus, false, objStoreID, volID, nil)
	return val, ref, err
}
