package plugin_host

import (
	"context"
	"sort"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/timestamp"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

const (
	// PluginHostTypeID is the type identifier for a PluginHost.
	PluginHostTypeID = "bldr/plugin-host"
	// PluginManifestTypeID is the type identifier for a PluginManifest.
	PluginManifestTypeID = "bldr/plugin-manifest"
	// PluginManifestBundleTypeID is the type identifier for a PluginManifestBundle.
	PluginManifestBundleTypeID = "bldr/plugin-manifest-bundle"

	// PredPlugin is the predicate linking a plugin to another object.
	//
	// Example: bldr/plugin-host <plugin> -> PluginManifest <plugin-id>
	// Example: bldr/plugin-host <plugin> -> PluginManifestBundle
	//
	// The plugin ID is stored in the Value field.
	// The value may be empty if linking to a Bundle.
	PredPlugin = quad.IRI("bldr/plugin")
)

// NewPluginQuad links to a plugin-ish object.
//
// pluginID can be empty.
func NewPluginQuad(srcObjKey, pluginObjKey, pluginID string) world.GraphQuad {
	var value string
	if pluginID != "" {
		value = quad.IRI(value).String()
	}
	return world.NewGraphQuadWithKeys(
		srcObjKey,
		PredPlugin.String(),
		pluginObjKey,
		value,
	)
}

// CreatePluginHost creates a PluginHost object if it doesn't exist.
func CreatePluginHost(ctx context.Context, ws world.WorldState, objKey string) (created bool, err error) {
	_, hostExists, err := ws.GetObject(objKey)
	if err != nil {
		return false, err
	}
	if hostExists {
		return false, nil
	}

	// TODO: plugin host object contents ?
	_, err = ws.CreateObject(objKey, nil)
	if err != nil {
		return false, err
	}

	types := world_types.NewTypesState(ctx, ws)
	err = types.SetObjectType(objKey, PluginHostTypeID)
	return true, err
}

// CheckPluginHostType checks the type graph quad for a PluginHost.
func CheckPluginHostType(typesState *world_types.TypesState, objKey string) error {
	pluginHostType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if pluginHostType != PluginHostTypeID {
		return errors.Errorf("expected plugin host type %s but got %q", PluginHostTypeID, pluginHostType)
	}
	return err
}

// SetPluginManifest creates a PluginManifest object in the world.
//
// Checks if the object exists already, and updates it if so.
func SetPluginManifest(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, error) {
	obj, objOk, err := ws.GetObject(objKey)
	if err != nil {
		return nil, err
	}
	if objOk {
		_, err = obj.SetRootRef(rootRef)
	} else {
		_, err = ws.CreateObject(objKey, rootRef)
		if err == nil {
			// create the <type> ref
			typesState := world_types.NewTypesState(ctx, ws)
			err = typesState.SetObjectType(objKey, PluginManifestTypeID)
		}
	}
	return nil, err
}

// LookupPluginManifest looks up a PluginManifest in the world.
func LookupPluginManifest(ctx context.Context, ws world.WorldState, objKey string) (*bldr_plugin.PluginManifest, *bucket.ObjectRef, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var manifest *bldr_plugin.PluginManifest
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_plugin.UnmarshalPluginManifest(bcs)
		return err
	})
	return manifest, ref, err
}

// NewListPluginManifestPath creates a Path that selects all PluginManifest
// recursively linked with <plugin>.
func NewListPluginManifestPath(p *cayley.Path) *cayley.Path {
	return world_types.LimitNodesToTypes(
		p.FollowRecursive(PredPlugin, 50, nil),
		PluginManifestTypeID,
	)
}

// ListPluginManifests lists all plugin manifests recursively linked to the given object(s).
func ListPluginManifests(ctx context.Context, w world.WorldState, startObjKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		startObjKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			// Follow <plugin> references, collecting nodes.
			// Limit those objects to the ones that have type plugin-manifest.
			return NewListPluginManifestPath(p), nil
		},
	)
}

// ListPluginManifestsWithPluginID lists all plugin manifests recursively linked to the given object(s).
// Filters to the given plugin ID.
func ListPluginManifestsWithPluginID(ctx context.Context, w world.WorldState, startObjKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		startObjKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			// Follow <plugin> references, collecting nodes.
			// Limit those objects to the ones that have type plugin-manifest.
			return NewListPluginManifestPath(p), nil
		},
	)
}

// CollectedPluginManifest contains information from CollectPluginManifest.
type CollectedPluginManifest struct {
	// Manifest is the plugin manifest object.
	Manifest *bldr_plugin.PluginManifest
	// ManifestRef is the reference to the plugin manifest object.
	ManifestRef *bucket.ObjectRef
	// ManifestKey is the object key of the manifest.
	ManifestKey string
}

// GetRev returns the revision.
func (c *CollectedPluginManifest) GetRev() uint64 {
	return c.Manifest.GetMeta().GetRev()
}

// CollectPluginManifests collects all PluginManifest linked to by the given object(s).
//
// Maps the manifests by plugin ID.
// Sorts the manifest lists by version number, higher is first in the list.
// Returns a list of errors corresponding to skipped plugin manifests (if any).
// If filterPluginPlatformID is not empty, filters to that platform ID.
func CollectPluginManifests(
	ctx context.Context,
	ws world.WorldState,
	filterPluginPlatformID string,
	objKeys ...string,
) (map[string][]*CollectedPluginManifest, []error, error) {
	pluginManifestObjKeys, err := ListPluginManifests(ctx, ws, objKeys...)
	if err != nil {
		return nil, nil, err
	}

	var manifestErrors []error
	manifestMap := make(map[string][]*CollectedPluginManifest)
	for _, objKey := range pluginManifestObjKeys {
		manifest, manifestRef, err := LookupPluginManifest(ctx, ws, objKey)
		if err != nil {
			manifestErrors = append(manifestErrors, errors.Wrapf(err, "plugin_manifests[%s]", objKey))
			continue
		}
		pluginID := manifest.GetMeta().GetPluginId()
		pluginPlatformID := manifest.GetMeta().GetPluginPlatformId()
		if filterPluginPlatformID != "" && filterPluginPlatformID != pluginPlatformID {
			continue
		}
		manifestList := append(manifestMap[pluginID], &CollectedPluginManifest{
			Manifest:    manifest,
			ManifestRef: manifestRef,
			ManifestKey: objKey,
		})
		sort.SliceStable(manifestList, func(i, j int) bool {
			return manifestList[i].GetRev() > manifestList[j].GetRev()
		})
		manifestMap[pluginID] = manifestList
	}

	return manifestMap, manifestErrors, nil
}

// CollectPluginManifestsForPluginID collects the list of PluginManifest for a specific plugin ID.
//
// Sorts the manifest lists by version number, higher is first in the list.
// Returns a list of errors corresponding to skipped plugin manifests (if any).
// If filterPluginPlatformID is not empty, filters to that platform ID.
func CollectPluginManifestsForPluginID(
	ctx context.Context,
	ws world.WorldState,
	pluginID string,
	filterPluginPlatformID string,
	objKeys ...string,
) ([]*CollectedPluginManifest, []error, error) {
	// TODO: https://github.com/cayleygraph/cayley/issues/977
	// - Use FilterContext to filter for label: empty string and/or plugin ID.
	// - Unsure how to implement this with cayley currently.
	// - For now, just filter after the fact.
	manifests, manifestErrs, err := CollectPluginManifests(ctx, ws, filterPluginPlatformID, objKeys...)
	if err != nil {
		return nil, manifestErrs, err
	}
	return manifests[pluginID], manifestErrs, nil
}

// LookupPluginManifestBundle looks up a PluginManifestBundle in the world.
func LookupPluginManifestBundle(ctx context.Context, ws world.WorldState, objKey string) (*bldr_plugin.PluginManifestBundle, *bucket.ObjectRef, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var manifest *bldr_plugin.PluginManifestBundle
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_plugin.UnmarshalPluginManifestBundle(bcs)
		return err
	})
	return manifest, ref, err
}

// ExtractPluginManifestBundle creates a PluginManifestBundle object in the world.
//
// Checks if the object exists already, and updates it if so.
// Extracts all plugin manifests from the bundle to the world, creating <plugin> links.
// Returns the bundle object state and list of manifest object keys.
func ExtractPluginManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, []*bldr_plugin.PluginManifest, []string, error) {
	manifestBundle, _, err := LookupPluginManifestBundle(ctx, ws, objKey)
	if err != nil {
		return nil, nil, nil, err
	}

	obj, objOk, err := ws.GetObject(objKey)
	if objOk {
		_, err = obj.SetRootRef(rootRef)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		obj, err = ws.CreateObject(objKey, rootRef)
		if err != nil {
			return nil, nil, nil, err
		}

		// create the <type> ref
		typesState := world_types.NewTypesState(ctx, ws)
		err = typesState.SetObjectType(objKey, PluginManifestBundleTypeID)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	// Extract all plugin manifests from the bundle to the world, creating <plugin> links
	manifestRefs := manifestBundle.GetPluginManifestRefs()
	manifests := make([]*bldr_plugin.PluginManifest, len(manifestRefs))
	manifestObjKeys := make([]string, len(manifestRefs))
	for i, manifestRef := range manifestRefs {
		if err := manifestRef.Validate(); err != nil {
			return nil, nil, nil, err
		}
		var manifest *bldr_plugin.PluginManifest
		_, err := world.AccessObject(ctx, ws.AccessWorldState, manifestRef.GetManifestRef(), func(bcs *block.Cursor) error {
			var err error
			manifest, err = bldr_plugin.UnmarshalPluginManifest(bcs)
			if err == nil {
				err = manifest.Validate()
			}
			return err
		})
		if err != nil {
			return nil, nil, nil, err
		}
		manifestObjKey, err := bldr_plugin.NewPluginManifestBundleEntryKey(objKey, manifest.GetMeta())
		if err != nil {
			return nil, nil, nil, err
		}
		_, err = SetPluginManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
		if err != nil {
			return nil, nil, nil, err
		}
		quad := NewPluginQuad(objKey, manifestObjKey, manifest.GetMeta().GetPluginId())
		if err := ws.SetGraphQuad(quad); err != nil {
			return nil, nil, nil, err
		}
		manifests[i] = manifest
		manifestObjKeys[i] = manifestObjKey
	}

	return obj, manifests, manifestObjKeys, nil
}

// CreatePluginManifestBundle creates the plugin manifest bundle at the block cursor.
// Aggregates together the given list of PluginManifest objects.
// Creates <plugin> links from the Bundle to the PluginManifest objects.
// The PluginManifest objects must be of type PluginManifest.
func CreatePluginManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	pluginManifestObjKeys []string,
	ts *timestamp.Timestamp,
) (*bldr_plugin.PluginManifestBundle, *bucket.ObjectRef, error) {
	bundle := &bldr_plugin.PluginManifestBundle{Timestamp: ts.Clone()}

	// copy the list of object keys, sort, make it unique.
	pluginManifestObjKeys = slices.Clone(pluginManifestObjKeys)
	sort.Strings(pluginManifestObjKeys)
	pluginManifestObjKeys = slices.Compact(pluginManifestObjKeys)

	// iterate over the manifests
	typesState := world_types.NewTypesState(ctx, ws)
	manifestPluginIDs := make([]string, len(pluginManifestObjKeys))
	for i, manifestObjKey := range pluginManifestObjKeys {
		if err := typesState.CheckObjectType(manifestObjKey, PluginManifestTypeID); err != nil {
			return nil, nil, err
		}
		manifest, manifestRef, err := LookupPluginManifest(ctx, ws, manifestObjKey)
		if err != nil {
			return nil, nil, err
		}
		manifestPluginIDs[i] = manifest.GetMeta().GetPluginId()
		if err := manifest.Validate(); err != nil {
			return nil, nil, errors.Wrapf(err, "invalid plugin manifest: %s", manifestPluginIDs[i])
		}
		bundle.PluginManifestRefs = append(
			bundle.PluginManifestRefs,
			bldr_plugin.NewPluginManifestRef(manifest.GetMeta(), manifestRef),
		)
	}

	// store the bundle to objKey
	_, objRef, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(bundle, true)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// create the links to the plugin manifests
	for i, manifestObjKey := range pluginManifestObjKeys {
		quad := NewPluginQuad(objKey, manifestObjKey, manifestPluginIDs[i])
		if err := ws.SetGraphQuad(quad); err != nil {
			return nil, nil, err
		}
	}

	return bundle, objRef, nil
}
