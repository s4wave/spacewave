package bldr_manifest_world

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/quad"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
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
	_, hostExists, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if hostExists {
		return false, nil
	}

	// TODO: manifest store object contents ?
	_, err = ws.CreateObject(ctx, objKey, nil)
	if err != nil {
		return false, err
	}

	err = world_types.SetObjectType(ctx, ws, objKey, ManifestStoreTypeID)
	return true, err
}

// CreateManifestStoreInEngine creates a manifest store in an engine using a transaction.
//
// Discards the transaction if nothing done.
func CreateManifestStoreInEngine(ctx context.Context, eng world.Engine, objKey string) (created bool, err error) {
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	created, err = CreateManifestStore(ctx, tx, objKey)
	if created && err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	return created, nil
}

// CheckManifestStoreType checks the type graph quad for a ManifestStore.
func CheckManifestStoreType(ctx context.Context, ws world.WorldState, objKey string) error {
	return world_types.CheckObjectType(ctx, ws, objKey, ManifestStoreTypeID)
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
) (world.ObjectState, bool, error) {
	var changed bool
	obj, objOk, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, false, err
	}
	if objOk {
		var currRootRef *bucket.ObjectRef
		currRootRef, _, err = obj.GetRootRef(ctx)
		if err != nil {
			return nil, false, err
		}
		if !currRootRef.EqualVT(rootRef) {
			_, err = obj.SetRootRef(ctx, rootRef)
			changed = err == nil
		}
	} else {
		_, err = ws.CreateObject(ctx, objKey, rootRef)
		if err == nil {
			// create the <type> ref
			err = world_types.SetObjectType(ctx, ws, objKey, ManifestTypeID)
			changed = err == nil
		}
	}
	return nil, changed, err
}

// LookupManifest looks up a Manifest in the world.
func LookupManifest(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var manifest *bldr_manifest.Manifest
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_manifest.UnmarshalManifest(ctx, bcs)
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
// If filterPlatformIDs is not empty, filters to those platform IDs.
func CollectManifests(
	ctx context.Context,
	ws world.WorldState,
	filterPlatformIDs []string,
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
		if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, platformID) {
			continue
		}
		manifestList := append(manifestMap[manifestID], &CollectedManifest{
			Manifest:    manifest,
			ManifestRef: manifestRef,
			ManifestKey: objKey,
		})
		// sort by rev descending
		sort.SliceStable(manifestList, func(i, j int) bool {
			return manifestList[i].GetRev() > manifestList[j].GetRev()
		})
		manifestMap[manifestID] = manifestList
	}

	return manifestMap, manifestErrors, nil
}

// FilterCollectedManifestsMapByPlatformID filters the result of CollectManifests by a platform id list.
func FilterCollectedManifestsMapByPlatformID(cmanifests map[string][]*CollectedManifest, platformIDs []string) {
	filterPlatformIDs := make(map[string]struct{}, len(platformIDs))
	for _, platformID := range platformIDs {
		filterPlatformIDs[platformID] = struct{}{}
	}
	for k, manifestList := range cmanifests {
		for i := 0; i < len(manifestList); i++ {
			v := manifestList[i]
			vPlatformID := v.Manifest.GetMeta().GetPlatformId()
			if _, ok := filterPlatformIDs[vPlatformID]; !ok {
				manifestList = slices.Delete(manifestList, i, i+1)
				i--
			}
		}
		cmanifests[k] = manifestList
	}
}

// FilterCollectedManifestsByPlatformID filters a list of collected manifests by platform id.
// Maintains the sort order.
func FilterCollectedManifestsByPlatformID(manifestList []*CollectedManifest, platformIDs []string) []*CollectedManifest {
	filterPlatformIDs := make(map[string]struct{}, len(platformIDs))
	for _, platformID := range platformIDs {
		filterPlatformIDs[platformID] = struct{}{}
	}
	for i := 0; i < len(manifestList); i++ {
		v := manifestList[i]
		vPlatformID := v.Manifest.GetMeta().GetPlatformId()
		if _, ok := filterPlatformIDs[vPlatformID]; !ok {
			manifestList = slices.Delete(manifestList, i, i+1)
			i--
		}
	}
	return manifestList
}

// FilterCollectedManifestsByFirst filters a list of collected manifests to the first for each platform id.
// The resulting slice will have zero or one manifest per platform ID.
// Usually this slice is sorted by revision (higher first) so this will return the latest manifest(s).
// Maintains the sort order.
func FilterCollectedManifestsByFirst(manifestList []*CollectedManifest) []*CollectedManifest {
	seenPlatformIDs := make(map[string]struct{})
	for i := 0; i < len(manifestList); i++ {
		v := manifestList[i]
		vPlatformID := v.Manifest.GetMeta().GetPlatformId()
		if _, ok := seenPlatformIDs[vPlatformID]; ok {
			manifestList = slices.Delete(manifestList, i, i+1)
			i--
		} else {
			seenPlatformIDs[vPlatformID] = struct{}{}
		}
	}
	return manifestList
}

// FilterCollectedManifestsByLatestRev filters a list of collected manifests to the latest revision for each manifest ID and platform ID combination.
// The resulting slice will have zero or one manifest per ManifestID+PlatformID combination with the highest revision.
// The resulting slice will be sorted by ManifestID, then by Rev (descending), then by PlatformID.
func FilterCollectedManifestsByLatestRev(manifestList []*CollectedManifest) []*CollectedManifest {
	// Group by ManifestID+PlatformID and find the latest revision for each combination
	type manifestPlatformKey struct {
		manifestID string
		platformID string
	}

	keyLatest := make(map[manifestPlatformKey]*CollectedManifest)
	for _, manifest := range manifestList {
		manifestID := manifest.Manifest.GetMeta().GetManifestId()
		platformID := manifest.Manifest.GetMeta().GetPlatformId()
		key := manifestPlatformKey{manifestID: manifestID, platformID: platformID}

		existing, exists := keyLatest[key]
		if !exists || manifest.GetRev() > existing.GetRev() {
			keyLatest[key] = manifest
		}
	}

	// Extract the latest manifests into a slice
	result := make([]*CollectedManifest, 0, len(keyLatest))
	for _, manifest := range keyLatest {
		result = append(result, manifest)
	}

	// Sort by ManifestID, then Rev (descending), then PlatformID
	slices.SortFunc(result, func(a, b *CollectedManifest) int {
		aManifestID := a.Manifest.GetMeta().GetManifestId()
		bManifestID := b.Manifest.GetMeta().GetManifestId()
		if cmp := strings.Compare(aManifestID, bManifestID); cmp != 0 {
			return cmp
		}

		aRev := a.GetRev()
		bRev := b.GetRev()
		if aRev != bRev {
			// Sort by rev descending (higher rev first)
			if aRev > bRev {
				return -1
			}
			return 1
		}

		aPlatformID := a.Manifest.GetMeta().GetPlatformId()
		bPlatformID := b.Manifest.GetMeta().GetPlatformId()
		return strings.Compare(aPlatformID, bPlatformID)
	})

	return result
}

// FilterCollectedManifestsByBuildType filters a list of collected manifests by build type.
// Maintains the sort order.
func FilterCollectedManifestsByBuildType(manifestList []*CollectedManifest, buildType bldr_manifest.BuildType) []*CollectedManifest {
	for i := 0; i < len(manifestList); i++ {
		v := manifestList[i]
		vBuildType := v.Manifest.GetMeta().GetBuildType()
		if vBuildType != string(buildType) {
			manifestList = slices.Delete(manifestList, i, i+1)
			i--
		}
	}
	return manifestList
}

// FilterCollectedManifestsByBuildTypes filters a list of collected manifests by build types.
// Maintains the sort order.
// If len(buildTypes) is zero, returns the original list.
func FilterCollectedManifestsByBuildTypes(manifestList []*CollectedManifest, buildTypes []bldr_manifest.BuildType) []*CollectedManifest {
	if len(buildTypes) == 0 {
		return manifestList
	}

	for i := 0; i < len(manifestList); i++ {
		v := manifestList[i]
		vBuildType := bldr_manifest.BuildType(v.Manifest.GetMeta().GetBuildType())
		if !slices.Contains(buildTypes, vBuildType) {
			manifestList = slices.Delete(manifestList, i, i+1)
			i--
		}
	}
	return manifestList
}

// FilterCollectedManifestsByMinRev filters a list of collected manifests by minimum revision.
// Maintains the sort order.
// If minRev is zero, returns the original list.
func FilterCollectedManifestsByMinRev(manifestList []*CollectedManifest, minRev uint64) []*CollectedManifest {
	if minRev == 0 {
		return manifestList
	}

	for i := 0; i < len(manifestList); i++ {
		v := manifestList[i]
		if v.GetRev() < minRev {
			manifestList = slices.Delete(manifestList, i, i+1)
			i--
		}
	}
	return manifestList
}

// CollectManifestsForManifestID collects the list of Manifest for a specific manifest ID.
//
// Sorts the manifest lists by version number, higher is first in the list.
// Returns a list of errors corresponding to skipped manifests (if any).
// If filterPlatformIDs is not empty, filters to those platform IDs.
func CollectManifestsForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) ([]*CollectedManifest, []error, error) {
	// TODO: How do we filter properly for a label?
	// - Use FilterContext to filter for label: empty string and/or manifest ID.
	// - Unsure how to implement this with cayley currently.
	// - For now, just filter after the fact.
	manifests, manifestErrs, err := CollectManifests(ctx, ws, filterPlatformIDs, objKeys...)
	if err != nil {
		return nil, manifestErrs, err
	}
	return manifests[manifestID], manifestErrs, nil
}

// LookupManifestBundle looks up a ManifestBundle in the world.
func LookupManifestBundle(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return nil, nil, err
	}
	var manifest *bldr_manifest.ManifestBundle
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifest, err = bldr_manifest.UnmarshalManifestBundle(ctx, bcs)
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

	obj, objOk, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, nil, nil, err
	}

	if objOk {
		_, err = obj.SetRootRef(ctx, rootRef)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		obj, err = ws.CreateObject(ctx, objKey, rootRef)
		if err != nil {
			return nil, nil, nil, err
		}

		// create the <type> ref
		err = world_types.SetObjectType(ctx, ws, objKey, ManifestBundleTypeID)
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
			manifest, err = bldr_manifest.UnmarshalManifest(ctx, bcs)
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
		_, _, err = SetManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
		if err != nil {
			return nil, nil, nil, err
		}
		quad := NewManifestQuad(objKey, manifestObjKey, manifest.GetMeta().GetManifestId())
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
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
// If the object already exists, collects manifests already linked to it as well.
func CreateManifestBundle(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	manifestObjKeys []string,
	ts *timestamp.Timestamp,
) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	manifestObjKeys = slices.Clone(manifestObjKeys)
	bundle := &bldr_manifest.ManifestBundle{Timestamp: ts.CloneVT()}

	// check for existing linked object keys
	existingManifests, _, err := CollectManifests(ctx, ws, nil, objKey)
	if err != nil {
		return nil, nil, err
	}
	for _, manifestSet := range existingManifests {
		for _, manifest := range manifestSet {
			manifestObjKeys = append(manifestObjKeys, manifest.ManifestKey)
		}
	}

	// sort & duplicate list of keys
	slices.Sort(manifestObjKeys)
	manifestObjKeys = slices.Compact(manifestObjKeys)

	// iterate over the manifests
	manifestIDs := make([]string, len(manifestObjKeys))
	for i, manifestObjKey := range manifestObjKeys {
		if err := world_types.CheckObjectType(ctx, ws, manifestObjKey, ManifestTypeID); err != nil {
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
	objRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
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
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			return nil, nil, err
		}
	}

	return bundle, objRef, nil
}
