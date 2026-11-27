package optypes

import (
	"context"
	"slices"
	"strings"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// PluginWriteRefusedCode is the rejection code of a write a plugin may not make.
const PluginWriteRefusedCode = "plugin/refused"

// pluginRefusedOps are the refused op types NewPluginWriteGuard lists.
var pluginRefusedOps = []string{
	space_world_ops.SetSpaceSettingsOpTypeID,
	space_world_ops.AddSpacePluginsOpTypeID,
	space_world_ops.SetSpaceIndexPathOpTypeID,
	bldr_manifest_world.StoreManifestOpTypeID,
	bldr_manifest_world.ExtractManifestBundleOpTypeID,
}

// pluginRefusedTypes are the refused object types NewPluginWriteGuard lists.
var pluginRefusedTypes = []string{
	bldr_manifest_world.ManifestStoreTypeID,
	bldr_manifest_world.ManifestTypeID,
	bldr_manifest_world.ManifestBundleTypeID,
	bldr_manifest_world.ManifestRefTypeID,
}

// pluginWriteGuard refuses one plugin the writes NewPluginWriteGuard lists.
type pluginWriteGuard struct {
	// pluginID identifies the plugin in refusals.
	pluginID string
}

// NewPluginWriteGuard returns the guard for the World handle of the plugin
// pluginID in a Space. It reads only the World it checks, so the same guard
// serves every handle that carries a plugin's identity.
//
// The guard refuses the writes that would change which plugins the Space runs
// or what they are, with a rejection coded PluginWriteRefusedCode:
//
//   - the Space settings ops set-settings, add-plugins and set-index-path;
//   - the manifest ops bldr/manifest/store and bldr/manifest/bundle/extract;
//   - any write to the Space settings object, or to an object typed as a
//     manifest store, manifest, manifest bundle or manifest ref;
//   - any graph quad touching such an object, any bldr/manifest edge, and
//     any type edge to a manifest type;
//   - any write to a type object other than creating it, and any quad
//     referencing a type object other than as the type of a type edge, so only
//     a type edge on an object, checked against its old and new type, changes
//     which type any object has;
//   - any write that cannot identify the objects it changes beforehand: a
//     writable graph handle, a rename of an object's descendants, and a block
//     removal or root move through a cursor.
//
// An op the plugin applies is checked by its type, then each write it makes is
// checked as the plugin's own: what an op may write is what the plugin may
// write.
// The Space's own session handle is not guarded.
func NewPluginWriteGuard(pluginID string) world.WriteGuard {
	return &pluginWriteGuard{pluginID: pluginID}
}

// CheckWrite refuses w when it is a write the plugin may not make.
func (g *pluginWriteGuard) CheckWrite(ctx context.Context, ws world.WorldState, w *world.GuardedWrite) error {
	// Refuse an unscoped write and a refused op by their description alone.
	if w.Unscoped {
		return g.refuse("a write that does not name its objects")
	}
	if slices.Contains(pluginRefusedOps, w.OpTypeID) {
		return g.refuse("op " + w.OpTypeID)
	}

	// Check each object the write identifies and the edge the write sets or
	// deletes.
	for _, key := range w.ObjectKeys {
		if isTypeObjectKey(key) {
			if w.Create {
				continue
			}
			return g.refuse("the type object " + key)
		}
		if err := g.checkKey(ctx, ws, key); err != nil {
			return err
		}
	}
	if w.Quad != nil {
		return g.checkQuad(ctx, ws, w.Quad)
	}
	return nil
}

// checkKey refuses a write to the settings object or a manifest object.
func (g *pluginWriteGuard) checkKey(ctx context.Context, ws world.WorldState, key string) error {
	// The settings object is refused by its key.
	if key == space_world.SpaceSettingsObjectKey {
		return g.refuse("the Space settings object")
	}

	// A manifest object is refused by its type, read in one graph lookup.
	typeID, err := world_types.GetObjectType(ctx, ws, key)
	if err != nil {
		return err
	}
	if slices.Contains(pluginRefusedTypes, typeID) {
		return g.refuse("a " + typeID + " object")
	}
	return nil
}

// checkQuad refuses a manifest edge, a type edge to a manifest type, an edge
// referencing a type object other than as its type, and any edge touching a
// refused object.
func (g *pluginWriteGuard) checkQuad(ctx context.Context, ws world.WorldState, q world.GraphQuad) error {
	switch q.GetPredicate() {
	case bldr_manifest_world.PredManifest.String():
		return g.refuse("a manifest edge")
	case world_types.TypePred.String():
		for _, typeID := range pluginRefusedTypes {
			if q.GetObj() == world_types.BuildTypeQuadValue(typeID).String() {
				return g.refuse("a type edge to " + typeID)
			}
		}
	}
	for i, value := range []string{q.GetSubject(), q.GetObj()} {
		key, err := world.GraphValueToKey(value)
		if err != nil {
			// A literal identifies no object.
			continue
		}
		if isTypeObjectKey(key) {
			// Only the object of a type edge may reference a type
			// object.
			if i == 1 && q.GetPredicate() == world_types.TypePred.String() {
				continue
			}
			return g.refuse("an edge on the type object " + key)
		}
		if err := g.checkKey(ctx, ws, key); err != nil {
			return err
		}
	}
	return nil
}

// isTypeObjectKey reports whether key is the key of a type object, whose type
// edges into it type other objects.
func isTypeObjectKey(key string) bool {
	return strings.HasPrefix(key, world.TypeObjectKeyPrefix)
}

// refuse returns the final rejection of a write to what.
func (g *pluginWriteGuard) refuse(what string) error {
	return &world.OperationRejection{
		Code:    PluginWriteRefusedCode,
		Message: "plugin " + g.pluginID + " may not write " + what,
	}
}

// _ is a type assertion
var _ world.WriteGuard = (*pluginWriteGuard)(nil)
