package bldr_manifest_world

import (
	"cmp"
	"context"
	stderrors "errors"
	"math"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/quad"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
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
		value = quad.IRI(manifestID).String()
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
	// An existing store is left untouched.
	hostExists, err := ws.HasObject(ctx, objKey)
	if err != nil {
		return false, err
	}
	if hostExists {
		return false, nil
	}

	// Create the object.
	createdObj, err := ws.CreateObject(ctx, objKey, nil)
	world.ReleaseObjectState(createdObj)
	if err != nil {
		return false, err
	}

	// Record its manifest store type.
	err = world_types.SetObjectType(ctx, ws, objKey, ManifestStoreTypeID)
	return true, err
}

// CreateManifestStoreInEngine creates a manifest store in an engine using a transaction.
//
// Discards the transaction if nothing done.
func CreateManifestStoreInEngine(ctx context.Context, eng world.Engine, objKey string) (created bool, err error) {
	// Loading a retained plugin can happen inside a World writer. Existing stores
	// need only a read snapshot, so initialization must not wait for that writer.
	reader, err := eng.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	exists, err := reader.HasObject(ctx, objKey)
	reader.Discard()
	if err != nil || exists {
		return false, err
	}

	// Recheck under the writer when this is the first store creation.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()

	// Create the store in the transaction and commit when created.
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

// ResetManifestStore deletes the manifest-store surface and recreates an empty store.
func ResetManifestStore(ctx context.Context, ws world.WorldState, objKey string) error {
	// Reject an empty object key.
	if objKey == "" {
		return world.ErrEmptyObjectKey
	}

	// Delete every unique linked candidate object.
	candidates, err := ListManifestCandidates(ctx, ws, objKey)
	if err != nil {
		return errors.Wrap(err, "list manifest store candidates")
	}
	seen := make(map[string]struct{}, len(candidates)+1)
	for _, candidate := range candidates {
		if candidate == "" || candidate == objKey {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if _, err := ws.DeleteObject(ctx, candidate); err != nil {
			return errors.Wrapf(err, "delete manifest candidate %s", candidate)
		}
	}

	// Delete the store object and recreate it empty.
	if _, err := ws.DeleteObject(ctx, objKey); err != nil {
		return errors.Wrapf(err, "delete manifest store %s", objKey)
	}
	if _, err := CreateManifestStore(ctx, ws, objKey); err != nil {
		return errors.Wrapf(err, "recreate manifest store %s", objKey)
	}
	return nil
}

// ResetManifestStores deletes and recreates manifest-store surfaces.
func ResetManifestStores(ctx context.Context, ws world.WorldState, objKeys ...string) error {
	// Reset each store in order.
	for _, objKey := range objKeys {
		if err := ResetManifestStore(ctx, ws, objKey); err != nil {
			return err
		}
	}
	return nil
}

// CollectManifestsResettingUnsupportedHash collects manifests and clears stale stores.
//
// The returned map is suitable for selecting more than one manifest ID from one
// graph traversal. A successful reset returns no manifests or manifest errors.
func CollectManifestsResettingUnsupportedHash(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	filterPlatformIDs []string,
	objKeys ...string,
) (map[string][]*CollectedManifest, []error, error) {
	// Return the collection unless an unsupported hash forced a reset.
	manifests, manifestErrs, err := CollectManifests(ctx, ws, filterPlatformIDs, objKeys...)
	if !hasUnsupportedHashError(err, manifestErrs) {
		return manifests, manifestErrs, err
	}

	// Log the reset, clear the stale stores, and return no manifests.
	logUnsupportedHashManifestStoreReset(le, objKeys, err, manifestErrs)
	if resetErr := ResetManifestStores(ctx, ws, objKeys...); resetErr != nil {
		return nil, manifestErrs, resetErr
	}
	return nil, nil, nil
}

// CollectManifestsForManifestIDResettingUnsupportedHash collects manifests for
// one ID and clears stale stores.
func CollectManifestsForManifestIDResettingUnsupportedHash(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) ([]*CollectedManifest, []error, error) {
	// Collect manifests and select the requested manifest ID.
	manifests, manifestErrs, err := CollectManifestsResettingUnsupportedHash(
		ctx,
		le,
		ws,
		filterPlatformIDs,
		objKeys...,
	)
	if err != nil {
		return nil, manifestErrs, err
	}
	return manifests[manifestID], manifestErrs, nil
}

// CollectStartupManifestsForManifestIDsResettingUnsupportedHash collects
// selected startup manifests and clears stale stores with unsupported hashes.
func CollectStartupManifestsForManifestIDsResettingUnsupportedHash(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	manifestIDs []string,
	filterPlatformIDs []string,
	objKeys ...string,
) (map[string][]*CollectedManifest, []error, error) {
	// Return the collection unless an unsupported hash forced a reset.
	manifests, manifestErrs, err := CollectStartupManifestsForManifestIDs(
		ctx,
		ws,
		manifestIDs,
		filterPlatformIDs,
		objKeys...,
	)
	if !hasUnsupportedHashError(err, manifestErrs) {
		return manifests, manifestErrs, err
	}

	// Log the reset, clear the stale stores, and return no manifests.
	logUnsupportedHashManifestStoreReset(le, objKeys, err, manifestErrs)
	if resetErr := ResetManifestStores(ctx, ws, objKeys...); resetErr != nil {
		return nil, manifestErrs, resetErr
	}
	return nil, nil, nil
}

func hasUnsupportedHashError(err error, manifestErrs []error) bool {
	// The top-level error or any manifest error may carry the unsupported hash.
	if stderrors.Is(err, hash.ErrHashTypeUnsupported) {
		return true
	}
	for _, manifestErr := range manifestErrs {
		if stderrors.Is(manifestErr, hash.ErrHashTypeUnsupported) {
			return true
		}
	}
	return false
}

func logUnsupportedHashManifestStoreReset(le *logrus.Entry, objKeys []string, err error, manifestErrs []error) {
	// A nil logger drops the message.
	if le == nil {
		return
	}

	// Fall back to the first unsupported-hash manifest error.
	logErr := err
	if logErr == nil {
		for _, manifestErr := range manifestErrs {
			if stderrors.Is(manifestErr, hash.ErrHashTypeUnsupported) {
				logErr = manifestErr
				break
			}
		}
	}
	le.WithError(logErr).
		WithField("object-keys", objKeys).
		Warn("unsupported hash in persisted manifest state; resetting")
}

// CanonicalizeManifestObjectRef returns a copy of a manifest object ref with
// the executable transform configuration encoded inline.
func CanonicalizeManifestObjectRef(
	ctx context.Context,
	access world.AccessWorldStateFunc,
	ref *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	// A nil ref canonicalizes to nil.
	if ref == nil {
		return nil, nil
	}
	out := ref.Clone()

	// An inline transform conf needs no pointer.
	if !out.GetTransformConf().GetEmpty() {
		out.TransformConfRef = nil
		return out, nil
	}
	if out.GetTransformConfRef().GetEmpty() {
		return out, nil
	}

	// Resolve the transform conf through the World access.
	if access == nil {
		return nil, errors.New("manifest object ref transform config requires world access")
	}
	err := access(ctx, ref, func(bls *bucket_lookup.Cursor) error {
		out.TransformConf = bls.GetTransformConf().Clone()
		out.TransformConfRef = nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ManifestObjectRefsSameExecutable reports whether two manifest object refs
// resolve to the same executable content. A manifest relocated between buckets
// can also re-encode its transform configuration, so executable identity is the
// root block alone. The scheduler uses this comparison to avoid restarting
// running content after a local copy. SetManifest also reports changes in
// locality because they affect whether a waiting candidate can execute.
//
// If either ref has an empty root block, the refs are compared with full
// equality including bucket id.
func ManifestObjectRefsSameExecutable(a, b *bucket.ObjectRef) bool {
	// Nil refs are equal only to each other.
	if a == nil || b == nil {
		return a == b
	}

	// Empty root refs fall back to full equality including bucket id.
	aRootRef := a.GetRootRef()
	bRootRef := b.GetRootRef()
	if aRootRef.GetEmpty() || bRootRef.GetEmpty() {
		return a.EqualVT(b)
	}
	return aRootRef.EqualsRef(bRootRef)
}

// SetManifest creates a Manifest object in the world.
//
// The returned change flag includes external-to-local copies so linked
// schedulers reconsider candidates that have become executable.
func SetManifest(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, bool, error) {
	// Fetch the existing manifest object, if any.
	var changed bool
	obj, objOk, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, false, err
	}

	// Update an existing object's root ref when it changes the manifest.
	if objOk {
		var replace bool
		replace, err = manifestRootRefReplaces(ctx, ws, obj, rootRef)
		if err != nil {
			return nil, false, err
		}
		if replace {
			_, err = obj.SetRootRef(ctx, rootRef)
			changed = err == nil
		}
	} else {
		// Create the object and its manifest type ref.
		created, createErr := ws.CreateObject(ctx, objKey, rootRef)
		world.ReleaseObjectState(created)
		err = createErr
		if err == nil {
			// create the <type> ref
			err = world_types.SetObjectType(ctx, ws, objKey, ManifestTypeID)
			changed = err == nil
		}
	}
	return nil, changed, err
}

// manifestRootRefReplaces reports whether storing rootRef replaces the root ref
// of the existing manifest object obj. A different executable replaces it. A
// same-executable ref replaces it only when it moves the manifest from an
// external bucket into the World bucket.
func manifestRootRefReplaces(
	ctx context.Context,
	ws world.WorldState,
	obj world.ObjectState,
	rootRef *bucket.ObjectRef,
) (bool, error) {
	// Compare the stored ref with rootRef.
	currRootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return false, err
	}
	if currRootRef.EqualVT(rootRef) {
		return false, nil
	}
	if !ManifestObjectRefsSameExecutable(currRootRef, rootRef) {
		return true, nil
	}

	// Replace a same-executable ref only to move it into the World bucket.
	var worldBucketID string
	err = ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		worldBucketID = bls.GetOpArgs().GetBucketId()
		return nil
	})
	if err != nil {
		return false, err
	}
	currLocal := currRootRef.GetBucketId() == "" || currRootRef.GetBucketId() == worldBucketID
	nextLocal := rootRef.GetBucketId() == "" || rootRef.GetBucketId() == worldBucketID
	return !currLocal && nextLocal, nil
}

// LookupManifest looks up a Manifest in the world.
func LookupManifest(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	// Fetch the object and unmarshal its manifest block.
	obj, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
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

// LookupManifestRef looks up a ManifestRef object in the world.
func LookupManifestRef(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.ManifestRef, *bucket.ObjectRef, error) {
	// Fetch the object and unmarshal its manifest ref block.
	obj, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, nil, err
	}
	var manifestRef *bldr_manifest.ManifestRef
	ref, _, err := world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		manifestRef, err = bldr_manifest.UnmarshalManifestRef(ctx, bcs)
		return err
	})
	return manifestRef, ref, err
}

// NewListManifestPath creates a Path that selects all Manifest
// recursively linked with <manifest>.
func NewListManifestPath(p *cayley.Path) *cayley.Path {
	// Limit the candidate path to typed Manifest objects.
	return world_types.LimitNodesToTypes(
		NewListManifestCandidatePath(p),
		ManifestTypeID,
	)
}

// NewListManifestCandidatePath creates a Path that selects all objects
// recursively linked with <manifest>.
func NewListManifestCandidatePath(p *cayley.Path) *cayley.Path {
	// Follow <manifest> edges recursively up to fifty levels.
	return p.FollowRecursive(PredManifest, 50, nil)
}

// ListManifests lists all manifests recursively linked to the given object(s).
func ListManifests(ctx context.Context, w world.WorldState, startObjKeys ...string) ([]string, error) {
	// Collect the keys reachable through the manifest path.
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

// ListManifestCandidates lists all objects recursively linked with <manifest>
// from the given object(s). Candidates may be Manifest objects, ManifestRef
// objects, or intermediate bundle/store objects.
func ListManifestCandidates(ctx context.Context, w world.WorldState, startObjKeys ...string) ([]string, error) {
	// Collect the keys reachable through the candidate path.
	return world.CollectPathWithKeys(
		ctx,
		w,
		startObjKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return NewListManifestCandidatePath(p), nil
		},
	)
}

// ListStartupManifestCandidatesWithID lists startup manifest candidates for a manifest ID.
//
// Exact-label edges are followed before legacy empty-label edges at each
// recursion step. Empty-label edges remain eligible for retained worlds written
// before manifest labels were persisted.
func ListStartupManifestCandidatesWithID(ctx context.Context, w world.WorldState, manifestID string, startObjKeys ...string) ([]string, error) {
	// An empty id falls back to the legacy full traversal.
	if manifestID == "" {
		return ListManifestCandidates(ctx, w, startObjKeys...)
	}
	return listManifestCandidatesByLabels(ctx, w, []string{
		quad.IRI(manifestID).String(),
		"",
	}, startObjKeys...)
}

type startupManifestSelectionCandidate struct {
	// objectKey is a selected or legacy manifest candidate.
	objectKey string
	// exactManifestIDs are the selected IDs named by concrete graph labels.
	exactManifestIDs []string
	// legacy reports an empty graph label on a path to this candidate.
	legacy bool
}

// listStartupManifestCandidatesForManifestIDs follows selected manifest labels
// and retained empty-label paths. Empty labels can name a direct legacy
// candidate or a store/bundle frontier, so every empty edge remains traversable.
func listStartupManifestCandidatesForManifestIDs(
	ctx context.Context,
	ws world.WorldState,
	manifestIDs []string,
	startObjKeys ...string,
) ([]startupManifestSelectionCandidate, error) {
	// Nothing to collect without start object keys.
	if len(startObjKeys) == 0 {
		return nil, nil
	}

	// Map each graph label IRI to its manifest id.
	labels := make(map[string]string, len(manifestIDs))
	for _, manifestID := range manifestIDs {
		labels[quad.IRI(manifestID).String()] = manifestID
	}

	// Seed the breadth-first queue with the unique start keys.
	seen := make(map[string]struct{}, len(startObjKeys))
	frontier := make([]string, 0, len(startObjKeys))
	for _, objKey := range startObjKeys {
		if objKey == "" {
			continue
		}
		if _, ok := seen[objKey]; ok {
			continue
		}
		seen[objKey] = struct{}{}
		frontier = append(frontier, objKey)
	}
	slices.Sort(frontier)

	// Walk the graph breadth-first up to fifty levels deep.
	candidates := make(map[string]*startupManifestSelectionCandidate)
	for depth := 0; depth < 50 && len(frontier) != 0; depth++ {
		// Look up the <manifest> edges for every frontier node at once.
		filters := make([]world.GraphQuad, len(frontier))
		for i, objKey := range frontier {
			filters[i] = world.NewGraphQuadWithKeys(objKey, PredManifest.String(), "", "")
		}
		results, err := lookupManifestEdgesBatch(ctx, ws, filters)
		if err != nil {
			return nil, err
		}

		// Record exact and legacy candidates and queue unseen keys.
		next := make([]string, 0)
		for _, quads := range results {
			for _, q := range quads {
				manifestID, exact := labels[q.GetLabel()]
				legacy := q.GetLabel() == ""
				if !exact && !legacy {
					continue
				}
				objKey, err := world.GraphValueToKey(q.GetObj())
				if err != nil {
					return nil, err
				}
				candidate := candidates[objKey]
				if candidate == nil {
					candidate = &startupManifestSelectionCandidate{objectKey: objKey}
					candidates[objKey] = candidate
				}
				if exact && !slices.Contains(candidate.exactManifestIDs, manifestID) {
					candidate.exactManifestIDs = append(candidate.exactManifestIDs, manifestID)
				}
				candidate.legacy = candidate.legacy || legacy
				if _, ok := seen[objKey]; ok {
					continue
				}
				seen[objKey] = struct{}{}
				next = append(next, objKey)
			}
		}
		slices.Sort(next)
		frontier = next
	}

	// Return the candidates sorted by object key.
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]startupManifestSelectionCandidate, 0, len(keys))
	for _, key := range keys {
		candidate := candidates[key]
		slices.Sort(candidate.exactManifestIDs)
		out = append(out, *candidate)
	}
	return out, nil
}

// manifestEdgeLookupLimit is the first per-filter limit for a batched
// <manifest> edge lookup. Remote World states reject an unbounded batch.
const manifestEdgeLookupLimit = 64

// lookupManifestEdgesBatch returns every quad matching each filter through
// bounded batch lookups. A filter whose result fills the limit may have more
// matches, so it is looked up again with twice the limit until it comes back
// short. The batch API has no cursor, so each retry reads from the start.
func lookupManifestEdgesBatch(
	ctx context.Context,
	ws world.WorldState,
	filters []world.GraphQuad,
) ([][]world.GraphQuad, error) {
	// Every filter starts pending at the first limit.
	results := make([][]world.GraphQuad, len(filters))
	pending := make([]int, len(filters))
	for i := range pending {
		pending[i] = i
	}
	limit := uint32(manifestEdgeLookupLimit)

	// Repeat until every filter has a result shorter than its limit.
	for len(pending) != 0 {
		// Look up the pending filters with the current limit.
		batch := make([]world.GraphQuad, len(pending))
		for i, idx := range pending {
			batch[i] = filters[idx]
		}
		found, err := ws.LookupGraphQuadsBatch(ctx, batch, limit)
		if err != nil {
			return nil, err
		}
		if len(found) != len(batch) {
			return nil, errors.Errorf("manifest graph lookup returned %d results for %d filters", len(found), len(batch))
		}

		// Keep each short result and queue the full ones for another round.
		next := pending[:0]
		for i, idx := range pending {
			results[idx] = found[i]
			if uint64(len(found[i])) >= uint64(limit) {
				next = append(next, idx)
			}
		}
		pending = next

		// Double the limit for the next round.
		if len(pending) != 0 && limit > math.MaxUint32/2 {
			return nil, errors.Errorf("manifest graph lookup exceeded %d edges for one object", limit)
		}
		limit *= 2
	}
	return results, nil
}

func listManifestCandidatesByLabels(
	ctx context.Context,
	w world.WorldState,
	labels []string,
	startObjKeys ...string,
) ([]string, error) {
	// Nothing to collect without start object keys.
	if len(startObjKeys) == 0 {
		return nil, nil
	}

	// Seed the breadth-first queue with the unique start keys.
	queued := make(map[string]struct{}, len(startObjKeys))
	frontier := make([]string, 0, len(startObjKeys))
	for _, objKey := range startObjKeys {
		if objKey == "" {
			continue
		}
		if _, ok := queued[objKey]; ok {
			continue
		}
		queued[objKey] = struct{}{}
		frontier = append(frontier, objKey)
	}

	// Walk the graph breadth-first up to fifty levels deep.
	var output []string
	outputSeen := make(map[string]struct{})
	for depth := 0; depth < 50 && len(frontier) != 0; depth++ {
		var next []string
		for _, objKey := range frontier {
			// Record every linked key matching a label.
			for _, label := range labels {
				linkedKeys, err := listManifestCandidateEdgesWithLabel(ctx, w, objKey, label)
				if err != nil {
					return nil, err
				}
				for _, linkedKey := range linkedKeys {
					if _, ok := outputSeen[linkedKey]; !ok {
						outputSeen[linkedKey] = struct{}{}
						output = append(output, linkedKey)
					}
					if _, ok := queued[linkedKey]; ok {
						continue
					}
					queued[linkedKey] = struct{}{}
					next = append(next, linkedKey)
				}
			}
		}
		frontier = next
	}
	return output, nil
}

func listManifestCandidateEdgesWithLabel(
	ctx context.Context,
	w world.WorldState,
	objKey string,
	label string,
) ([]string, error) {
	// Look up the object's <manifest> edges.
	quads, err := w.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(objKey, PredManifest.String(), "", ""),
		0,
	)
	if err != nil {
		return nil, err
	}

	// Collect the linked keys matching the label, sorted.
	linkedKeys := make([]string, 0, len(quads))
	for _, q := range quads {
		if q.GetLabel() != label {
			continue
		}
		linkedKey, err := world.GraphValueToKey(q.GetObj())
		if err != nil {
			return nil, err
		}
		linkedKeys = append(linkedKeys, linkedKey)
	}
	slices.Sort(linkedKeys)
	return linkedKeys, nil
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

// StartupManifestSkipError describes a startup manifest candidate that was skipped.
type StartupManifestSkipError struct {
	// ObjectKey is the skipped candidate object key.
	ObjectKey string
	// ObjectRef is the object or manifest ref involved in the skip, if known.
	ObjectRef *bucket.ObjectRef
	// Err is the underlying availability error.
	Err error
}

// Error returns the startup manifest skip message.
func (e *StartupManifestSkipError) Error() string {
	// A nil error has no message.
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return "startup manifest candidate[" + e.ObjectKey + "] unavailable"
	}

	// Describe the candidate with its ref, bucket, and root.
	msg := "startup manifest candidate[" + e.ObjectKey + "]"
	if e.ObjectRef != nil {
		if !e.ObjectRef.GetEmpty() {
			msg += " ref=" + e.ObjectRef.MarshalString()
		}
		if bucketID := e.ObjectRef.GetBucketId(); bucketID != "" {
			msg += " bucket=" + bucketID
		}
		if rootRef := e.ObjectRef.GetRootRef(); !rootRef.GetEmpty() {
			msg += " root=" + rootRef.MarshalString()
		}
	}
	return msg + ": " + e.Err.Error()
}

// Unwrap returns the underlying skip cause.
func (e *StartupManifestSkipError) Unwrap() error {
	// A nil error unwraps to nil.
	if e == nil {
		return nil
	}
	return e.Err
}

func newStartupManifestSkipError(objKey string, objRef *bucket.ObjectRef, err error) *StartupManifestSkipError {
	return &StartupManifestSkipError{
		ObjectKey: objKey,
		ObjectRef: objRef.Clone(),
		Err:       err,
	}
}

func startupContextError(err error) error {
	// Report cancellation and deadline errors as themselves.
	cause := errors.Cause(err)
	if cause == context.Canceled || stderrors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if cause == context.DeadlineExceeded || stderrors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
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
	// List the manifest object keys reachable from the start objects.
	manifestObjKeys, err := ListManifests(ctx, ws, objKeys...)
	if err != nil {
		return nil, nil, err
	}

	// Collect one entry per readable manifest, grouped by manifest id.
	var manifestErrors []error
	manifestMap := make(map[string][]*CollectedManifest)

	// Look up each manifest object key in turn.
	for _, objKey := range manifestObjKeys {
		// Context errors stop the collection; others are skip errors.
		manifest, manifestRef, err := LookupManifest(ctx, ws, objKey)
		if err != nil {
			cause := errors.Cause(err)
			if cause == context.Canceled || cause == context.DeadlineExceeded {
				return nil, manifestErrors, cause
			}
			manifestErrors = append(manifestErrors, errors.Wrapf(err, "manifests[%s]", objKey))
			continue
		}

		// Filter by platform id when the caller supplied filters.
		manifestID := manifest.GetMeta().GetManifestId()
		platformID := manifest.GetMeta().GetPlatformId()
		if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, platformID) {
			continue
		}

		// sort by rev descending
		manifestList := append(manifestMap[manifestID], &CollectedManifest{
			Manifest:    manifest,
			ManifestRef: manifestRef,
			ManifestKey: objKey,
		})
		slices.SortStableFunc(manifestList, func(a, b *CollectedManifest) int {
			return cmp.Compare(b.GetRev(), a.GetRev())
		})
		manifestMap[manifestID] = manifestList
	}

	return manifestMap, manifestErrors, nil
}

// CollectStartupManifests collects readable startup manifest candidates linked
// to the given object(s).
//
// Unlike CollectManifests, this accepts ManifestRef candidate objects and
// treats optional candidate lookup failures as skip errors.
func CollectStartupManifests(
	ctx context.Context,
	ws world.WorldState,
	filterPlatformIDs []string,
	objKeys ...string,
) (map[string][]*CollectedManifest, []error, error) {
	// List the candidate keys and treat each as a legacy candidate.
	manifestObjKeys, err := ListManifestCandidates(ctx, ws, objKeys...)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]startupManifestSelectionCandidate, 0, len(manifestObjKeys))
	for _, objKey := range manifestObjKeys {
		candidates = append(candidates, startupManifestSelectionCandidate{
			objectKey: objKey,
			legacy:    true,
		})
	}
	any := func(startupManifestSelectionCandidate, string) bool { return true }
	return collectStartupManifests(ctx, ws, candidates, any, filterPlatformIDs)
}

// CollectStartupManifestsForManifestIDs collects startup manifests for selected
// IDs. An empty ID list retains the full traversal used by catalog callers.
func CollectStartupManifestsForManifestIDs(
	ctx context.Context,
	ws world.WorldState,
	manifestIDs []string,
	filterPlatformIDs []string,
	objKeys ...string,
) (map[string][]*CollectedManifest, []error, error) {
	// Dedupe and sort the requested manifest ids.
	manifestIDs = slices.Clone(manifestIDs)
	for i := 0; i < len(manifestIDs); i++ {
		if manifestIDs[i] == "" {
			manifestIDs = slices.Delete(manifestIDs, i, i+1)
			i--
		}
	}
	slices.Sort(manifestIDs)
	manifestIDs = slices.Compact(manifestIDs)
	if len(manifestIDs) == 0 {
		return CollectStartupManifests(ctx, ws, filterPlatformIDs, objKeys...)
	}

	// Follow the selected labels and collect the matching candidates.
	candidates, err := listStartupManifestCandidatesForManifestIDs(ctx, ws, manifestIDs, objKeys...)
	if err != nil {
		return nil, nil, err
	}
	selected := make(map[string]struct{}, len(manifestIDs))
	for _, manifestID := range manifestIDs {
		selected[manifestID] = struct{}{}
	}
	matches := func(candidate startupManifestSelectionCandidate, manifestID string) bool {
		return candidate.matches(manifestID, selected)
	}
	return collectStartupManifests(ctx, ws, candidates, matches, filterPlatformIDs)
}

// collectStartupManifests walks the candidates, decoding and filtering each
// one, and returns collected manifests grouped by manifest id.
//
// matches reports whether a candidate's manifest id passes the caller's
// selection; it is applied to both the ref metadata and the decoded
// manifest.
func collectStartupManifests(
	ctx context.Context,
	ws world.WorldState,
	candidates []startupManifestSelectionCandidate,
	matches func(candidate startupManifestSelectionCandidate, manifestID string) bool,
	filterPlatformIDs []string,
) (map[string][]*CollectedManifest, []error, error) {
	// Collect one entry per selected candidate, grouped by manifest id.
	var manifestErrors []error
	manifestMap := make(map[string][]*CollectedManifest)

	for _, candidate := range candidates {
		// Intermediates are skipped; other type errors are skip errors.
		objType, err := world_types.GetObjectType(ctx, ws, candidate.objectKey)
		if err != nil {
			if ctxErr := startupContextError(err); ctxErr != nil {
				return nil, manifestErrors, ctxErr
			}
			manifestErrors = append(manifestErrors, newStartupManifestSkipError(candidate.objectKey, nil, err))
			continue
		}
		if objType == ManifestStoreTypeID || objType == ManifestBundleTypeID {
			continue
		}

		// Decode the candidate, skipping unselected or filtered ones.
		manifest, manifestRef, skip, err := collectStartupManifestCandidate(
			ctx,
			ws,
			candidate,
			objType,
			matches,
			filterPlatformIDs,
		)
		if skip {
			continue
		}
		if err != nil {
			if ctxErr := startupContextError(err); ctxErr != nil {
				return nil, manifestErrors, ctxErr
			}
			manifestErrors = append(manifestErrors, newStartupManifestSkipError(candidate.objectKey, manifestRef, err))
			continue
		}

		// Validate the manifest and apply the id and platform filters.
		if err := manifest.Validate(); err != nil {
			manifestErrors = append(manifestErrors, newStartupManifestSkipError(candidate.objectKey, manifestRef, err))
			continue
		}
		manifestID := manifest.GetMeta().GetManifestId()
		if !matches(candidate, manifestID) {
			continue
		}
		platformID := manifest.GetMeta().GetPlatformId()
		if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, platformID) {
			continue
		}

		// Sort each manifest list by rev descending.
		manifestList := append(manifestMap[manifestID], &CollectedManifest{
			Manifest:    manifest,
			ManifestRef: manifestRef,
			ManifestKey: candidate.objectKey,
		})
		slices.SortStableFunc(manifestList, func(a, b *CollectedManifest) int {
			return cmp.Compare(b.GetRev(), a.GetRev())
		})
		manifestMap[manifestID] = manifestList
	}

	return manifestMap, manifestErrors, nil
}

// matches reports whether the candidate's manifest id passes selection:
// exact graph labels match directly, and legacy candidates match any id in
// the selected set.
func (c startupManifestSelectionCandidate) matches(
	manifestID string,
	selected map[string]struct{},
) bool {
	// Exact graph labels match directly.
	if slices.Contains(c.exactManifestIDs, manifestID) {
		return true
	}

	// Legacy candidates match any id in the selected set.
	if !c.legacy {
		return false
	}
	_, ok := selected[manifestID]
	return ok
}

// collectStartupManifestCandidate decodes one startup candidate. It follows
// a manifest ref when present, falling back to direct manifest or bundle
// lookups for untyped objects. Returns skip=true when the candidate is not
// selected under matches or is filtered by platform.
func collectStartupManifestCandidate(
	ctx context.Context,
	ws world.WorldState,
	candidate startupManifestSelectionCandidate,
	objType string,
	matches func(candidate startupManifestSelectionCandidate, manifestID string) bool,
	filterPlatformIDs []string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, bool, error) {
	// A typed Manifest object is looked up directly.
	if objType == ManifestTypeID {
		manifest, manifestRef, err := LookupManifest(ctx, ws, candidate.objectKey)
		return manifest, manifestRef, false, err
	}

	// Other candidates resolve through their manifest ref object.
	manifestRef, candidateRef, err := LookupManifestRef(ctx, ws, candidate.objectKey)
	if err != nil {
		// Untyped objects may be a direct manifest or a bundle.
		if objType == "" {
			manifest, directRef, manifestErr := LookupManifest(ctx, ws, candidate.objectKey)
			if manifestErr == nil && manifest != nil && manifest.Validate() == nil {
				return manifest, directRef, false, nil
			}
			if _, _, bundleErr := LookupManifestBundle(ctx, ws, candidate.objectKey); bundleErr == nil {
				return nil, nil, true, nil
			}
		}
		return nil, candidateRef, false, err
	}

	// Validate the ref and apply the id and platform filters.
	manifestObjRef := manifestRef.GetManifestRef()
	if err := manifestRef.Validate(); err != nil {
		return nil, manifestObjRef, false, err
	}
	refMeta := manifestRef.GetMeta()
	if !matches(candidate, refMeta.GetManifestId()) {
		return nil, manifestObjRef, true, nil
	}
	if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, refMeta.GetPlatformId()) {
		return nil, manifestObjRef, true, nil
	}

	// The referenced manifest's meta must match the ref meta.
	manifest, err := lookupStartupManifestObjectRefLocal(ctx, ws, manifestObjRef)
	if err != nil {
		return nil, manifestObjRef, false, err
	}
	if !manifest.GetMeta().EqualVT(manifestRef.GetMeta()) {
		return nil, manifestObjRef, false, errors.New("manifest ref meta does not match manifest meta")
	}
	return manifest, manifestObjRef, false, nil
}

func lookupStartupManifestObject(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	// Fetch the object and look up its manifest on demand.
	obj, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, nil, err
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := lookupStartupManifestObjectRefDemand(ctx, ws, ref)
	return manifest, ref, err
}

func lookupStartupManifestObjectLocal(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	// Fetch the object and look up its manifest locally.
	obj, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, nil, err
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := lookupStartupManifestObjectRefLocal(ctx, ws, ref)
	return manifest, ref, err
}

func lookupStartupManifestObjectRefLocal(
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
) (*bldr_manifest.Manifest, error) {
	// Reject an empty object ref.
	if ref == nil || ref.GetEmpty() {
		return nil, errors.New("manifest object ref is empty")
	}

	// Build the storage cursor and follow the ref locally.
	storageCursor, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}
	defer storageCursor.Release()

	// Follow the ref to the manifest cursor.
	manifestCursor, err := followStartupManifestRef(ctx, storageCursor, ref)
	if err != nil {
		return nil, err
	}
	defer manifestCursor.Release()

	// Restrict the manifest cursor to local reads.
	localManifestCursor := manifestCursor.CloneWithLocalOnlyReads()
	defer localManifestCursor.Release()

	// Unmarshal and validate the manifest.
	_, bcs := localManifestCursor.BuildTransaction(nil)
	manifest, err := bldr_manifest.UnmarshalManifest(ctx, bcs)
	if err != nil {
		return nil, err
	}
	return manifest, validateStartupManifest(ctx, manifest)
}

func lookupStartupManifestObjectRefDemand(
	ctx context.Context,
	ws world.WorldState,
	ref *bucket.ObjectRef,
) (*bldr_manifest.Manifest, error) {
	// Reject an empty object ref.
	if ref == nil || ref.GetEmpty() {
		return nil, errors.New("manifest object ref is empty")
	}

	// Build the storage cursor and follow the ref on demand.
	storageCursor, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}
	defer storageCursor.Release()

	// Follow the ref to the manifest cursor.
	manifestCursor, err := followManifestRefForStartupDemand(ctx, storageCursor, ref)
	if err != nil {
		return nil, err
	}
	defer manifestCursor.Release()

	// Unmarshal and validate the manifest.
	_, bcs := manifestCursor.BuildTransaction(nil)
	manifest, err := bldr_manifest.UnmarshalManifest(ctx, bcs)
	if err != nil {
		return nil, err
	}
	return manifest, validateStartupManifest(ctx, manifest)
}

// opArgsForRef derives the follow op args for a ref: a ref pointing at an
// external bucket retargets the bucket and drops the source volume binding.
func opArgsForRef(root *bucket_lookup.Cursor, ref *bucket.ObjectRef) *bucket.BucketOpArgs {
	// Retarget the bucket to the ref's bucket when set.
	opArgs := root.GetOpArgs()
	if refBucketID := ref.GetBucketId(); refBucketID != "" {
		opArgs.BucketId = refBucketID
	}

	// Drop the source volume binding when the bucket changed.
	if opArgs.GetBucketId() != root.GetOpArgs().GetBucketId() {
		opArgs.VolumeId = ""
	}
	return opArgs
}

func followManifestRefForStartupDemand(
	ctx context.Context,
	root *bucket_lookup.Cursor,
	ref *bucket.ObjectRef,
) (*bucket_lookup.Cursor, error) {
	return root.FollowRefWithOpArgs(ctx, ref, opArgsForRef(root, ref))
}

func followStartupManifestRef(
	ctx context.Context,
	root *bucket_lookup.Cursor,
	ref *bucket.ObjectRef,
) (*bucket_lookup.Cursor, error) {
	return root.FollowRefWithOpArgsReadOnly(ctx, ref, opArgsForRef(root, ref), true)
}

// FollowObjectRefReadOnly follows an object reference through a lookup-only
// bucket handle, clearing the source volume when switching buckets so the
// lookup may resolve an external bucket.
func FollowObjectRefReadOnly(
	ctx context.Context,
	root *bucket_lookup.Cursor,
	ref *bucket.ObjectRef,
) (*bucket_lookup.Cursor, error) {
	// Validate the root cursor and object ref.
	if root == nil {
		return nil, errors.New("root cursor is nil")
	}
	if ref == nil || ref.GetEmpty() {
		return nil, errors.New("object ref is empty")
	}
	return root.FollowRefWithOpArgsReadOnly(ctx, ref, opArgsForRef(root, ref), false)
}

// FilterCollectedManifestsMapByPlatformID filters the result of CollectManifests by a platform id list.
func FilterCollectedManifestsMapByPlatformID(cmanifests map[string][]*CollectedManifest, platformIDs []string) {
	// Index the allowed platform ids.
	filterPlatformIDs := make(map[string]struct{}, len(platformIDs))
	for _, platformID := range platformIDs {
		filterPlatformIDs[platformID] = struct{}{}
	}

	// Drop each manifest whose platform id is not allowed.
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
	// Index the allowed platform ids.
	filterPlatformIDs := make(map[string]struct{}, len(platformIDs))
	for _, platformID := range platformIDs {
		filterPlatformIDs[platformID] = struct{}{}
	}

	// Drop each manifest whose platform id is not allowed.
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
	// Keep the first manifest per platform id in sort order.
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

	// Keep the highest-revision manifest per ManifestID+PlatformID.
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
		// Compare by manifest id first.
		aManifestID := a.Manifest.GetMeta().GetManifestId()
		bManifestID := b.Manifest.GetMeta().GetManifestId()
		if cmp := strings.Compare(aManifestID, bManifestID); cmp != 0 {
			return cmp
		}

		// Sort by rev descending (higher rev first)
		aRev := a.GetRev()
		bRev := b.GetRev()
		if aRev != bRev {
			if aRev > bRev {
				return -1
			}
			return 1
		}

		// Compare equal revisions by platform id.
		aPlatformID := a.Manifest.GetMeta().GetPlatformId()
		bPlatformID := b.Manifest.GetMeta().GetPlatformId()
		return strings.Compare(aPlatformID, bPlatformID)
	})

	return result
}

// FilterCollectedManifestsByBuildType filters a list of collected manifests by build type.
// Maintains the sort order.
func FilterCollectedManifestsByBuildType(manifestList []*CollectedManifest, buildType bldr_manifest.BuildType) []*CollectedManifest {
	// Drop each manifest whose build type does not match.
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
	// An empty filter list keeps the original list.
	if len(buildTypes) == 0 {
		return manifestList
	}

	// Drop each manifest whose build type is not allowed.
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
	// A zero minimum keeps the original list.
	if minRev == 0 {
		return manifestList
	}

	// Drop each manifest below the minimum revision.
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
	// CollectManifests traverses every reachable manifest before this lookup
	// selects one manifest ID from its result map.
	manifests, manifestErrs, err := CollectManifests(ctx, ws, filterPlatformIDs, objKeys...)
	if err != nil {
		return nil, manifestErrs, err
	}
	return manifests[manifestID], manifestErrs, nil
}

// CollectStartupManifestsForManifestID collects startup Manifest candidates for a specific manifest ID.
func CollectStartupManifestsForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) ([]*CollectedManifest, []error, error) {
	// Collect the startup manifests and select the requested manifest ID.
	manifests, manifestErrs, err := CollectStartupManifestsForManifestIDs(
		ctx,
		ws,
		[]string{manifestID},
		filterPlatformIDs,
		objKeys...,
	)
	if err != nil {
		return nil, manifestErrs, err
	}
	return manifests[manifestID], manifestErrs, nil
}

// LookupManifestBundle looks up a ManifestBundle in the world.
func LookupManifestBundle(ctx context.Context, ws world.WorldState, objKey string) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	// Fetch the object and unmarshal its bundle block.
	obj, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
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
	// Look up the bundle block before touching the bundle object.
	manifestBundle, _, err := LookupManifestBundle(ctx, ws, objKey)
	if err != nil {
		return nil, nil, nil, err
	}

	// Fetch the existing bundle object, if any.
	obj, objOk, err := ws.GetObject(ctx, objKey)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, nil, nil, err
	}

	// Update the existing object's root ref or create the object.
	if objOk {
		_, err = obj.SetRootRef(ctx, rootRef)
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, nil, nil, err
		}
	} else {
		obj, err = ws.CreateObject(ctx, objKey, rootRef)
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, nil, nil, err
		}

		// create the <type> ref
		err = world_types.SetObjectType(ctx, ws, objKey, ManifestBundleTypeID)
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, nil, nil, err
		}
	}

	// Extract all manifests from the bundle to the world, creating <manifest> links
	manifestRefs := manifestBundle.GetManifestRefs()
	manifests := make([]*bldr_manifest.Manifest, len(manifestRefs))
	manifestObjKeys := make([]string, len(manifestRefs))
	for i, manifestRef := range manifestRefs {
		// Validate the ref and decode its manifest block.
		if err := manifestRef.Validate(); err != nil {
			world.ReleaseObjectState(obj)
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
			world.ReleaseObjectState(obj)
			return nil, nil, nil, err
		}

		// Register the manifest under its bundle entry key.
		manifestObjKey, err := bldr_manifest.NewManifestBundleEntryKey(objKey, manifest.GetMeta())
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, nil, nil, err
		}
		_, _, err = SetManifest(ctx, ws, sender, manifestObjKey, manifestRef.GetManifestRef())
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, nil, nil, err
		}
		quad := NewManifestQuad(objKey, manifestObjKey, manifest.GetMeta().GetManifestId())
		if err := ws.SetGraphQuad(ctx, quad); err != nil {
			world.ReleaseObjectState(obj)
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
	// Clone the manifest keys and start the bundle with its timestamp.
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

	// Collect each manifest's ref into the bundle.
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
