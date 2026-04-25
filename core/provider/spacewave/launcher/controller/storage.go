package spacewave_launcher_controller

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/peer"
)

// defVolumeID is the default volume ID for the launcher.
const defVolumeID = "plugin-host"

// defObjectStoreID is the default object store ID for the launcher.
var defObjectStoreID = "spacewave/launcher"

// defObjectStoreKey is the default key used to store the distribution config packedmsg.
var defObjectStoreKey = "dist-conf"

// localDistConfigFilename is the package-shipped fallback dist config filename.
const localDistConfigFilename = "dist-config.packedmsg"

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

// loadLocalDistConf loads a package-shipped dist config next to the entrypoint.
func (c *Controller) loadLocalDistConf() ([]byte, string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, "", err
	}
	resolvedExePath, err := filepath.EvalSymlinks(exePath)
	if err == nil && resolvedExePath != "" {
		exePath = resolvedExePath
	}
	return readLocalDistConf(localDistConfPaths(exePath))
}

// localDistConfPaths returns candidate package dist-config paths for exePath.
func localDistConfPaths(exePath string) []string {
	exeDir := filepath.Dir(exePath)
	return []string{
		filepath.Join(exeDir, localDistConfigFilename),
		filepath.Join(exeDir, "..", "Resources", localDistConfigFilename),
	}
}

// readLocalDistConf reads the first available local dist config path.
func readLocalDistConf(paths []string) ([]byte, string, error) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", err
		}
		if len(data) == 0 {
			return nil, "", errors.Errorf("local dist config is empty: %s", p)
		}
		return data, p, nil
	}
	return nil, "", nil
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
	volID := c.GetVolumeId()
	val, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, c.bus, false, objStoreID, volID, nil)
	return val, ref, err
}
