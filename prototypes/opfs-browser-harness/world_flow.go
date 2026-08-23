//go:build goscript

package opfsbrowserharness

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	world "github.com/s4wave/spacewave/db/world"
)

// Run writes world objects, closes the world, and reads them from a fresh
// world instance. The returned error is nil only after world state survives
// the OPFS close and reopen boundary.
func Run() error {
	ctx := context.Background()
	root, err := opfs.GetRoot()
	if err != nil {
		return errors.Wrap(err, "get OPFS root")
	}
	if err := opfs.DeleteEntry(root, volumeRoot, true); err != nil && !opfs.IsNotFound(err) {
		return errors.Wrap(err, "clear harness volume")
	}

	w, err := OpenWorld(ctx)
	if err != nil {
		return errors.Wrap(err, "open OPFS world for write")
	}
	if err := CheckWorld(ctx, w.WS); err != nil {
		w.Close()
		return errors.Wrap(err, "write world state")
	}
	if _, err := w.WS.Sync(ctx); err != nil {
		w.Close()
		return errors.Wrap(err, "sync world state")
	}
	w.Close()

	w, err = OpenWorld(ctx)
	if err != nil {
		return errors.Wrap(err, "reopen OPFS world")
	}
	defer w.Close()
	for _, key := range []string{"task/a", "task/b"} {
		obj, found, err := w.WS.GetObject(ctx, key)
		if err != nil {
			return errors.Wrapf(err, "read world object %s", key)
		}
		if obj != nil {
			world.ReleaseObjectState(obj)
		}
		if !found {
			return errors.Errorf("world object %s missing after reopen", key)
		}
	}
	quads, err := w.WS.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("task/a", "<assigned-to>", "", ""), 0)
	if err != nil {
		return errors.Wrap(err, "read world graph")
	}
	if len(quads) != 1 || quads[0].GetObj() != world.KeyToGraphValue("task/b").String() {
		return errors.Errorf("world graph after reopen = %#v", quads)
	}
	return nil
}
