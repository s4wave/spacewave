package bldr_manifest_world

import (
	"context"
	"sort"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
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
	// ManifestStoreTypeID is the type identifier for a ManifestStore.
	ManifestStoreTypeID = "bldr/manifest-store"
	// ManifestTypeID is the type identifier for a Manifest.
	ManifestTypeID = "bldr/manifest"
	// ManifestBundleTypeID is the type identifier for a ManifestBundle.
	ManifestBundleTypeID = "bldr/manifest-bundle"

	// PredManifest is the predicate linking to a manifest.
	//
	// Example: bldr/manifest-bundle <manifest> -> Manifest <manifest-id>
	// Example: bldr/manifest-store <manifest> -> ManifestBundle
	//
	// The manifest ID is stored in the Value field.
	// The value may be empty if linking to a Bundle.
	PredManifest = quad.IRI("bldr/manifest")
)

// NewManifestQuad links to a manifest object or an object with links to other manifests.
//
// manifestID can be empty.
func NewManifestQuad(srcObjKey, destObjKey, manifestID string) world.GraphQuad {
	var value string
	if manifestID != "" {
		value = quad.IRI(value).String()
	}
	return world.NewGraphQuadWithKeys(
		srcObjKey,
		PredManifest.String(),
		destObjKey,
		value,
	)
}

// CreateManifestStore creates a ManifestStore object if it doesn't exist.
func CreateManifestStore(ctx context.Context, ws world.WorldState, objKey string) (created bool, err error) {
	_, hostExists, err := ws.GetObject(objKey)
	if err != nil {
		return false, err
	}
	if hostExists {
		return false, nil
	}

	// TODO: manifest store object contents ?
	_, err = ws.CreateObject(objKey, nil)
	if err != nil {
		return false, err
	}

	types := world_types.NewTypesState(ctx, ws)
	err = types.SetObjectType(objKey, ManifestStoreTypeID)
	return true, err
}

// CheckManifestStoreType checks the type graph quad for a ManifestStore.
func CheckManifestStoreType(typesState *world_types.TypesState, objKey string) error {
	manifestStoreType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if manifestStoreType != ManifestStoreTypeID {
		return errors.Errorf("expected object type %s but got %q", ManifestStoreTypeID, manifestStoreType)
	}
	return err
}

// SetManifest creates a Manifest object in the world.
//
// Checks if the object exists already, and updates it if so.
func SetManifest(
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
			err = typesState.SetObjectType(objKey, ManifestTypeID)
		}
	}
	return nil, err
}

// LookupManifest looks up a Manifest in the world.
func LookupManifest(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var manifest *bldr_manifest.Manifest
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_manifest.UnmarshalManifest(bcs)
		return err
	})
	return manifest, ref, err
}

// NewListManifestPath creates a Path that selects all Manifest
// recursively linked with <manifest>.
func NewListManifestPath(p *cayley.Path) *cayley.Path {
	return world_types.LimitNodesToTypes(
		p.FollowRecursive(PredManifest, 50, nil),
		ManifestTypeID,
	)
}

// ListManifests lists all manifests recursively linked to the given object(s).
func ListManifests(ctx context.Context, w world.WorldState, startObjKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		startObjKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			// Follow <manifest> references, collecting nodes.
			// Limit those objects to the ones that have type manifest.
			return NewListManifestPath(p), nil
		},
	)
}

// ListManifestsWithID lists all manifests recursively linked to the given object(s).
// Filters to the given manifest ID.
func ListManifestsWithID(ctx context.Context, w world.WorldState, startObjKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		startObjKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			// Follow <manifest> references, collecting nodes.
			// Limit those objects to the ones that have type manifest.
			return NewListManifestPath(p), nil
		},
	)
}

// CollectedManifest contains information from CollectManifest.
type CollectedManifest struct {
	// Manifest is the manifest object.
	Manifest *bldr_manifest.Manifest
	// ManifestRef is the reference to the manifest object.
	ManifestRef *bucket.ObjectRef
	// ManifestKey is the object key of the manifest.
	ManifestKey string
}

// GetRev returns the revision.
func (c *CollectedManifest) GetRev() uint64 {
	return c.Manifest.GetMeta().GetRev()
}

// CollectManifests collects all Manifest linked to by the given object(s).
//
// Maps the manifests by manifest ID.
// Sorts the manifest lists by version number, higher is first in the list.
// Returns a list of errors corresponding to skipped manifests (if any).
// If filterPlatformID is not empty, filters to that platform ID.
func CollectManifests(
	ctx context.Context,
	ws world.WorldState,
	filterPlatformID string,
	objKeys ...string,
) (map[string][]*CollectedManifest, []error, error) {
	manifestObjKeys, err := ListManifests(ctx, ws, objKeys...)
	if err != nil {
		return nil, nil, err
	}

	var manifestErrors []error
	manifestMap := make(map[string][]*CollectedManifest)
	for _, objKey := range manifestObjKeys {
		manifest, manifestRef, err := LookupManifest(ctx, ws, objKey)
		if err != nil {
			manifestErrors = append(manifestErrors, errors.Wrapf(err, "manifests[%s]", objKey))
			continue
		}
		manifestID := manifest.GetMeta().GetManifestId()
		platformID := manifest.GetMeta().GetPlatformId()
		if filterPlatformID != "" && filterPlatformID != platformID {
			continue
		}
		manifestList := append(manifestMap[manifestID], &CollectedManifest{
			Manifest:    manifest,
			ManifestRef: manifestRef,
			ManifestKey: objKey,
		})
		sort.SliceStable(manifestList, func(i, j int) bool {
			return manifestList[i].GetRev() > manifestList[j].GetRev()
		})
		manifestMap[manifestID] = manifestList
	}

	return manifestMap, manifestErrors, nil
}

// CollectManifestsForManifestID collects the list of Manifest for a specific manifest ID.
//
// Sorts the manifest lists by version number, higher is first in the list.
// Returns a list of errors corresponding to skipped manifests (if any).
// If filterPlatformID is not empty, filters to that platform ID.
func CollectManifestsForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformID string,
	objKeys ...string,
) ([]*CollectedManifest, []error, error) {
	// TODO: https://github.com/cayleygraph/cayley/issues/977
	// - Use FilterContext to filter for label: empty string and/or manifest ID.
	// - Unsure how to implement this with cayley currently.
	// - For now, just filter after the fact.
	manifests, manifestErrs, err := CollectManifests(ctx, ws, filterPlatformID, objKeys...)
	if err != nil {
		return nil, manifestErrs, err
	}
	return manifests[manifestID], manifestErrs, nil
}

// LookupManifestBundle looks up a ManifestBundle in the world.
func LookupManifestBundle(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var manifest *bldr_manifest.ManifestBundle
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_manifest.UnmarshalManifestBundle(bcs)
		return err
	})
	return manifest, ref, err
}

// ExtractManifestBundle creates a ManifestBundle object in the world.
//
// Checks if the object exists already, and updates it if so.
// Extracts all manifests from the bundle to the world, creating <manifest> links.
// Returns the bundle object state and list of manifest object keys.
func ExtractManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, []*bldr_manifest.Manifest, []string, error) {
	manifestBundle, _, err := LookupManifestBundle(ctx, ws, objKey)
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
		err = typesState.SetObjectType(objKey, ManifestBundleTypeID)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	// Extract all manifests from the bundle to the world, creating <manifest> links
	manifestRefs := manifestBundle.GetManifestRefs()
	manifests := make([]*bldr_manifest.Manifest, len(manifestRefs))
	manifestObjKeys := make([]string, len(manifestRefs))
	for i, manifestRef := range manifestRefs {
		if err := manifestRef.Validate(); err != nil {
			return nil, nil, nil, err
		}
		var manifest *bldr_manifest.Manifest
		_, err := world.AccessObject(ctx, ws.AccessWorldState, manifestRef.GetManifestRef(), func(bcs *block.Cursor) error {
			var err error
			manifest, err = bldr_manifest.UnmarshalManifest(bcs)
			if err == nil {
				err = manifest.Validate()
			}
			return err
		})
		if err != nil {
			return nil, nil, nil, err
		}
		manifestObjKey, err := bldr_manifest.NewManifestBundleEntryKey(objKey, manifest.GetMeta())
		if err != nil {
			return nil, nil, nil, err
		}
		_, err = SetManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
		if err != nil {
			return nil, nil, nil, err
		}
		quad := NewManifestQuad(objKey, manifestObjKey, manifest.GetMeta().GetManifestId())
		if err := ws.SetGraphQuad(quad); err != nil {
			return nil, nil, nil, err
		}
		manifests[i] = manifest
		manifestObjKeys[i] = manifestObjKey
	}

	return obj, manifests, manifestObjKeys, nil
}

// CreateManifestBundle creates the manifest bundle at the block cursor.
// Aggregates together the given list of Manifest objects.
// Creates <manifest> links from the Bundle to the Manifest objects.
// The Manifest objects must be of type Manifest.
func CreateManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	manifestObjKeys []string,
	ts *timestamp.Timestamp,
) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	bundle := &bldr_manifest.ManifestBundle{Timestamp: ts.Clone()}

	// copy the list of object keys, sort, make it unique.
	manifestObjKeys = slices.Clone(manifestObjKeys)
	sort.Strings(manifestObjKeys)
	manifestObjKeys = slices.Compact(manifestObjKeys)

	// iterate over the manifests
	typesState := world_types.NewTypesState(ctx, ws)
	manifestIDs := make([]string, len(manifestObjKeys))
	for i, manifestObjKey := range manifestObjKeys {
		if err := typesState.CheckObjectType(manifestObjKey, ManifestTypeID); err != nil {
			return nil, nil, err
		}
		manifest, manifestRef, err := LookupManifest(ctx, ws, manifestObjKey)
		if err != nil {
			return nil, nil, err
		}
		manifestIDs[i] = manifest.GetMeta().GetManifestId()
		if err := manifest.Validate(); err != nil {
			return nil, nil, errors.Wrapf(err, "invalid manifest: %s", manifestIDs[i])
		}
		bundle.ManifestRefs = append(
			bundle.ManifestRefs,
			bldr_manifest.NewManifestRef(manifest.GetMeta(), manifestRef),
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

	// create the links to the manifests
	for i, manifestObjKey := range manifestObjKeys {
		quad := NewManifestQuad(objKey, manifestObjKey, manifestIDs[i])
		if err := ws.SetGraphQuad(quad); err != nil {
			return nil, nil, err
		}
	}

	return bundle, objRef, nil
}
