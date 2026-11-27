//go:build !js

package plugin_host_process

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

// DistReclaimEnv names the environment variable that selects where the host
// reclaims unused dist checkouts.
// "background", or unset, reclaims in the host's own routine.
// "launch" reclaims on each launch path, after the launch's sync.
// "off" keeps every checkout.
// Any other value keeps every checkout and logs a warning.
const DistReclaimEnv = "BLDR_PLUGIN_DIST_RECLAIM"

// distTrashPrefix starts the name of a checkout renamed for removal.
// A manifest root never starts with it, so no execution reads a trash name.
const distTrashPrefix = ".reclaim-"

// distReclaimMode selects where unused dist checkouts are reclaimed.
type distReclaimMode int

const (
	// distReclaimBackground reclaims in the host's Execute routine.
	distReclaimBackground distReclaimMode = iota
	// distReclaimLaunch reclaims in place on each launch path.
	distReclaimLaunch
	// distReclaimOff keeps every checkout.
	distReclaimOff
)

// parseDistReclaimMode parses a DistReclaimEnv value.
func parseDistReclaimMode(value string) (distReclaimMode, error) {
	switch value {
	case "", "background":
		return distReclaimBackground, nil
	case "launch":
		return distReclaimLaunch, nil
	case "off":
		return distReclaimOff, nil
	default:
		return distReclaimOff, errors.Errorf("unknown %s value %q: keeping every checkout", DistReclaimEnv, value)
	}
}

// distCheckouts tracks which native dist checkouts executions use and reclaims
// the checkouts of a plugin that none uses.
//
// A checkout is <distDir>/<plugin>/manifest/<root>, written only by the sync
// of an execution of that root.
// Executions are the only readers.
// A process that runs from a checkout reads its assets until it exits.
//
// Invariants:
//
// A checkout with a reference is never renamed or removed.
// The reference is counted before the sync that creates or repairs the
// checkout and dropped after the process exits.
//
// The reference check and the claim on the root are one step under bcast.
// An acquisition of a claimed root waits until its rename to a trash name ends.
// A checkout acquired after the reclaimer listed it keeps its files.
// A root acquired after its rename syncs a new checkout.
//
// Renames and removals run outside bcast.
// A rename touches only a claimed root.
// A removal touches only trash names.
// No execution reads a trash name and no sync writes one.
//
// Only directories named by a valid manifest root or the trash prefix are
// touched.
// Staged entrypoints and unknown entries are kept.
//
// The references count only this host's executions.
// Another process running from the same dist directory is not protected.
type distCheckouts struct {
	// le is the logger.
	le *logrus.Entry
	// distDir is the host's dist directory.
	distDir string
	// mode selects where checkouts are reclaimed.
	mode distReclaimMode
	// rc runs the background reclaimer under the host's Execute context.
	rc *routine.RoutineContainer

	// bcast guards the fields below.
	bcast broadcast.Broadcast
	// refs counts executing instances by plugin artifact ID.
	refs map[string]int
	// claimed holds the artifact IDs whose checkouts are being renamed to trash.
	claimed map[string]struct{}
	// syncing counts executions between acquire and the end of their sync.
	// The background reclaimer waits for zero before it starts a removal.
	syncing int
	// pending holds the plugin IDs that wait for a reclaim pass.
	pending map[string]struct{}
}

// newDistCheckouts constructs the checkout tracker for a dist directory.
func newDistCheckouts(le *logrus.Entry, distDir string, mode distReclaimMode) *distCheckouts {
	c := &distCheckouts{
		le:      le,
		distDir: distDir,
		mode:    mode,
		rc:      routine.NewRoutineContainer(routine.WithExitLogger(le)),
		refs:    make(map[string]int),
		claimed: make(map[string]struct{}),
		pending: make(map[string]struct{}),
	}
	if mode == distReclaimBackground {
		c.rc.SetRoutine(c.execute)
	}
	return c
}

// setContext runs the background reclaimer until ctx ends.
func (c *distCheckouts) setContext(ctx context.Context) {
	_ = c.rc.SetContext(ctx, false)
}

// acquire counts a reference on an artifact's checkout and marks its sync in
// flight.
// It waits while the reclaimer renames that artifact's checkout.
// Call it before syncing the checkout.
// After a nil return, call synced after the sync and release after the process
// exits.
func (c *distCheckouts) acquire(ctx context.Context, artifactID string) error {
	return c.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		// Wait out a claim on this artifact.
		if _, ok := c.claimed[artifactID]; ok {
			return false, nil
		}

		// Count the reference and the sync.
		c.refs[artifactID]++
		c.syncing++
		return true, nil
	})
}

// synced ends a sync begun by acquire.
// After a successful sync, it reclaims the plugin's unused checkouts as mode
// selects.
// In launch mode the reclaim runs before synced returns.
func (c *distCheckouts) synced(ctx context.Context, pluginID string, ok bool) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.syncing--
		if ok && c.mode == distReclaimBackground {
			c.pending[pluginID] = struct{}{}
		}
		broadcast()
	})
	if ok && c.mode == distReclaimLaunch {
		c.reclaim(ctx, pluginID)
	}
}

// release drops a reference counted by acquire.
func (c *distCheckouts) release(artifactID string) {
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if c.refs[artifactID]--; c.refs[artifactID] <= 0 {
			delete(c.refs, artifactID)
		}
	})
}

// execute runs reclaim passes for pending plugins until ctx ends.
// A pass starts only while no sync is in flight.
func (c *distCheckouts) execute(ctx context.Context) error {
	for {
		var pluginID string
		err := c.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			if c.syncing != 0 {
				return false, nil
			}
			for id := range c.pending {
				delete(c.pending, id)
				pluginID = id
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return nil
		}
		c.reclaim(ctx, pluginID)
	}
}

// reclaim lists a plugin's checkouts and reclaims the unused ones.
func (c *distCheckouts) reclaim(ctx context.Context, pluginID string) {
	// List the checkouts without holding bcast.
	manifestsDir := c.manifestsDir(pluginID)
	dirents, err := os.ReadDir(manifestsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			c.le.WithError(err).WithField("plugin-id", pluginID).Warn("unable to list plugin dist checkouts")
		}
		return
	}

	// Reclaim the listed checkouts.
	names := make([]string, 0, len(dirents))
	for _, ent := range dirents {
		if ent.IsDir() {
			names = append(names, ent.Name())
		}
	}
	c.reclaimListed(ctx, pluginID, names)
}

// reclaimListed reclaims the listed checkout directories of a plugin.
// A listed checkout acquired since the listing is kept.
func (c *distCheckouts) reclaimListed(ctx context.Context, pluginID string, names []string) {
	// Retire each unused checkout, collecting the trash to remove.
	manifestsDir := c.manifestsDir(pluginID)
	var trash []string
	for _, name := range names {
		dir := filepath.Join(manifestsDir, name)
		if strings.HasPrefix(name, distTrashPrefix) {
			trash = append(trash, dir)
			continue
		}
		artifactID := bldr_plugin.PluginArtifactID(pluginID, name)
		if _, _, err := bldr_plugin.ParsePluginArtifactID(artifactID, false); err != nil {
			continue
		}
		if moved := c.retire(artifactID, dir); moved != "" {
			trash = append(trash, moved)
		}
	}

	// Remove the trash without holding bcast, yielding to syncs in background mode.
	for _, dir := range trash {
		if c.mode == distReclaimBackground && c.waitNoSync(ctx) != nil {
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			c.le.WithError(err).WithField("dist-dir", dir).Warn("unable to remove unused plugin dist checkout")
		}
	}
}

// retire takes an unreferenced checkout out of use.
// Background mode claims the root under bcast, renames the checkout to a trash
// name outside bcast and returns that name.
// Launch mode removes it in place under bcast and returns empty.
// Returns empty when the checkout is referenced or the rename fails.
func (c *distCheckouts) retire(artifactID, dir string) string {
	// Check the reference and claim the root in one locked step.
	var claimed bool
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Keep a checkout an execution references.
		if c.refs[artifactID] > 0 {
			return
		}

		// Launch mode removes the checkout in place under the lock.
		if c.mode == distReclaimLaunch {
			if err := os.RemoveAll(dir); err != nil {
				c.le.WithError(err).WithField("dist-dir", dir).Warn("unable to remove unused plugin dist checkout")
			}
			return
		}
		c.claimed[artifactID] = struct{}{}
		claimed = true
	})
	if !claimed {
		return ""
	}

	// Rename the claimed checkout to a trash name, then drop the claim.
	trashDir := filepath.Join(filepath.Dir(dir), distTrashPrefix+randstring.RandomIdentifier(0))
	err := os.Rename(dir, trashDir)
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(c.claimed, artifactID)
		broadcast()
	})
	if err != nil {
		c.le.WithError(err).WithField("dist-dir", dir).Warn("unable to retire unused plugin dist checkout")
		return ""
	}
	return trashDir
}

// waitNoSync waits until no sync is in flight.
func (c *distCheckouts) waitNoSync(ctx context.Context) error {
	return c.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return c.syncing == 0, nil
	})
}

// manifestsDir returns the directory holding a plugin's checkouts by root.
func (c *distCheckouts) manifestsDir(pluginID string) string {
	return filepath.Join(c.distDir, pluginID, "manifest")
}
