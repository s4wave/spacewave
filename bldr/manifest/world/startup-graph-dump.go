package bldr_manifest_world

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

type startupManifestGraphEdge struct {
	from  string
	to    string
	label string
}
type startupManifestGraphResult struct {
	edges         []startupManifestGraphEdge
	candidates    []string
	dequeuedNodes int
	edgesFound    int
	depthReached  int
}

// dumpStartupManifestGraphForManifestID builds a non-mutating diagnostic dump
// of the retained manifest graph followed during startup selection.
func dumpStartupManifestGraphForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) (string, error) {
	edges, candidates, err := collectStartupManifestGraph(ctx, ws, manifestID, objKeys...)
	if err != nil {
		return "", err
	}
	worldBucketID, err := startupManifestGraphWorldBucketID(ctx, ws)
	if err != nil {
		return "", err
	}

	var lines []string
	header := "startup manifest graph manifest_id=" + manifestID + " platform_ids=" + strings.Join(filterPlatformIDs, ",")
	if worldBucketID != "" {
		header += " world_bucket=" + worldBucketID
	}
	lines = append(lines, header)
	for _, objKey := range objKeys {
		if objKey == "" {
			continue
		}
		lines = append(lines, describeStartupManifestGraphRoot(ctx, ws, objKey, worldBucketID))
	}
	for _, edge := range edges {
		lines = append(lines, "edge "+edge.from+" -> "+edge.to+" label="+startupManifestGraphLabel(edge.label))
	}
	for _, objKey := range candidates {
		lines = append(lines, describeStartupManifestGraphCandidate(ctx, ws, objKey, manifestID, filterPlatformIDs, worldBucketID))
	}
	return strings.Join(lines, "\n"), nil
}

func startupManifestGraphWorldBucketID(ctx context.Context, ws world.WorldState) (string, error) {
	var bucketID string
	err := ws.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		bucketID = root.GetOpArgs().GetBucketId()
		return nil
	})
	return bucketID, err
}

func collectStartupManifestGraphCore(
	ctx context.Context,
	w world.WorldState,
	manifestID string,
	startObjKeys ...string,
) (startupManifestGraphResult, error) {
	if len(startObjKeys) == 0 {
		return startupManifestGraphResult{}, nil
	}

	labels := startupManifestGraphLabels(manifestID)
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

	result := startupManifestGraphResult{}
	outputSeen := make(map[string]struct{})
	for depth := 0; depth < 50 && len(frontier) != 0; depth++ {
		if depth+1 > result.depthReached {
			result.depthReached = depth + 1
		}
		quadsByNode, err := lookupStartupManifestGraphQuadsBatch(ctx, w, frontier)
		if err != nil {
			return startupManifestGraphResult{}, err
		}
		var next []string
		for i, objKey := range frontier {
			result.dequeuedNodes++
			quads := quadsByNode[i]
			sortStartupManifestGraphQuads(quads)
			for _, label := range labels {
				for _, q := range quads {
					if q.GetLabel() != label {
						continue
					}
					linkedKey, err := world.GraphValueToKey(q.GetObj())
					if err != nil {
						return startupManifestGraphResult{}, err
					}
					result.edgesFound++
					result.edges = append(result.edges, startupManifestGraphEdge{
						from:  objKey,
						to:    linkedKey,
						label: q.GetLabel(),
					})
					if _, ok := outputSeen[linkedKey]; !ok {
						outputSeen[linkedKey] = struct{}{}
						result.candidates = append(result.candidates, linkedKey)
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
	return result, nil
}

func startupManifestGraphLabels(manifestID string) []string {
	if manifestID == "" {
		return []string{""}
	}
	return []string{
		quad.IRI(manifestID).String(),
		"",
	}
}

func sortStartupManifestGraphQuads(quads []world.GraphQuad) {
	slices.SortFunc(quads, func(a, b world.GraphQuad) int {
		if cmp := strings.Compare(a.GetLabel(), b.GetLabel()); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.GetObj(), b.GetObj())
	})
}

func describeStartupManifestGraphRoot(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	defaultBucketID string,
) string {
	return describeStartupManifestGraphObject(ctx, ws, "root", objKey, defaultBucketID)
}

func describeStartupManifestGraphCandidate(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	expectedManifestID string,
	filterPlatformIDs []string,
	defaultBucketID string,
) string {
	parts := describeStartupManifestGraphObjectParts(ctx, ws, "candidate", objKey, defaultBucketID)
	parts = append(parts, startupManifestGraphProvenanceParts(ctx, ws, objKey)...)
	if startupManifestGraphPartHasPrefix(parts, "skip=") {
		return strings.Join(parts, " ")
	}

	objType := startupManifestGraphPartValue(parts, "type=")
	if objType == ManifestStoreTypeID || objType == ManifestBundleTypeID {
		parts = append(parts, "intermediate=true")
		return strings.Join(parts, " ")
	}

	if objType == ManifestTypeID {
		manifest, _, err := lookupStartupManifestObjectLocal(ctx, ws, objKey)
		if err != nil {
			parts = append(parts, "skip="+err.Error())
			return strings.Join(parts, " ")
		}
		parts = append(parts, "manifest_meta="+startupManifestGraphMeta(manifest.GetMeta()))
		if err := manifest.Validate(); err != nil {
			parts = append(parts, "skip="+err.Error())
		}
		return strings.Join(parts, " ")
	}

	manifestRef, _, err := LookupManifestRef(ctx, ws, objKey)
	if err != nil {
		manifest, _, manifestErr := lookupStartupManifestObjectLocal(ctx, ws, objKey)
		if manifestErr == nil && manifest != nil {
			parts = append(parts, "manifest_meta="+startupManifestGraphMeta(manifest.GetMeta()))
			if err := manifest.Validate(); err != nil {
				parts = append(parts, "skip="+err.Error())
			}
			return strings.Join(parts, " ")
		}
		if _, _, bundleErr := LookupManifestBundle(ctx, ws, objKey); bundleErr == nil {
			parts = append(parts, "intermediate=true")
			return strings.Join(parts, " ")
		}
		parts = append(parts, "skip="+err.Error())
		return strings.Join(parts, " ")
	}

	refMeta := manifestRef.GetMeta()
	manifestObjRef := manifestRef.GetManifestRef()
	parts = append(parts, "ref_meta="+startupManifestGraphMeta(refMeta))
	parts = append(parts, startupManifestGraphObjectRefParts("manifest", manifestObjRef, defaultBucketID)...)
	if err := manifestRef.Validate(); err != nil {
		parts = append(parts, "skip="+err.Error())
		return strings.Join(parts, " ")
	}
	if expectedManifestID != "" && refMeta.GetManifestId() != expectedManifestID {
		parts = append(parts, "filtered=manifest-id")
		return strings.Join(parts, " ")
	}
	if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, refMeta.GetPlatformId()) {
		parts = append(parts, "filtered=platform-id")
		return strings.Join(parts, " ")
	}

	manifest, err := lookupStartupManifestObjectRefLocal(ctx, ws, manifestObjRef)
	if err != nil {
		parts = append(parts, "skip="+err.Error())
		return strings.Join(parts, " ")
	}
	parts = append(parts, "manifest_meta="+startupManifestGraphMeta(manifest.GetMeta()))
	if !manifest.GetMeta().EqualVT(manifestRef.GetMeta()) {
		parts = append(parts, "skip=manifest ref meta does not match manifest meta")
		return strings.Join(parts, " ")
	}
	if err := manifest.Validate(); err != nil {
		parts = append(parts, "skip="+err.Error())
	}
	return strings.Join(parts, " ")
}

func describeStartupManifestGraphObject(
	ctx context.Context,
	ws world.WorldState,
	prefix string,
	objKey string,
	defaultBucketID string,
) string {
	return strings.Join(describeStartupManifestGraphObjectParts(ctx, ws, prefix, objKey, defaultBucketID), " ")
}

func describeStartupManifestGraphObjectParts(
	ctx context.Context,
	ws world.WorldState,
	prefix string,
	objKey string,
	defaultBucketID string,
) []string {
	parts := []string{prefix + " " + objKey}
	objType, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		if ctxErr := startupContextError(err); ctxErr != nil {
			err = ctxErr
		}
		parts = append(parts, "skip="+err.Error())
		return parts
	}
	typePart := "type=" + objType
	if objType == "" {
		typePart = "type=<unknown>"
	}
	parts = append(parts, typePart)

	obj, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		parts = append(parts, "skip="+err.Error())
		return parts
	}
	if !found {
		parts = append(parts, "skip="+world.ErrObjectNotFound.Error())
		return parts
	}
	objRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		parts = append(parts, "skip="+err.Error())
		return parts
	}
	parts = append(parts, startupManifestGraphObjectRefParts("object", objRef, defaultBucketID)...)
	return parts
}

func startupManifestGraphObjectRefParts(prefix string, ref *bucket.ObjectRef, defaultBucketID string) []string {
	if ref == nil || ref.GetEmpty() {
		return []string{prefix + "_ref=<empty>"}
	}
	parts := []string{prefix + "_ref=" + ref.MarshalString()}
	if bucketID := ref.GetBucketId(); bucketID != "" {
		parts = append(parts, prefix+"_bucket="+bucketID)
		return appendStartupManifestGraphRootRefPart(parts, prefix, ref)
	}
	if defaultBucketID != "" {
		parts = append(parts, prefix+"_bucket="+defaultBucketID)
	}
	return appendStartupManifestGraphRootRefPart(parts, prefix, ref)
}

func appendStartupManifestGraphRootRefPart(parts []string, prefix string, ref *bucket.ObjectRef) []string {
	if rootRef := ref.GetRootRef(); rootRef != nil && !rootRef.GetEmpty() {
		parts = append(parts, prefix+"_root="+rootRef.MarshalString())
	}
	return parts
}

func startupManifestGraphMeta(meta *bldr_manifest.ManifestMeta) string {
	return "manifest_id=" + meta.GetManifestId() +
		",build_type=" + meta.GetBuildType() +
		",platform_id=" + meta.GetPlatformId() +
		",rev=" + strconv.FormatUint(meta.GetRev(), 10)
}

func startupManifestGraphLabel(label string) string {
	if label == "" {
		return "<empty>"
	}
	return label
}

func startupManifestGraphProvenanceParts(ctx context.Context, ws world.WorldState, objKey string) []string {
	provenance := classifyStartupManifestGraphProvenance(ctx, ws, objKey)
	return []string{
		"provenance=" + provenance.source,
		"derived=" + strconv.FormatBool(provenance.derived),
		"protected=" + strconv.FormatBool(provenance.protected),
	}
}

type startupManifestGraphProvenance struct {
	source    string
	derived   bool
	protected bool
}

func classifyStartupManifestGraphProvenance(ctx context.Context, ws world.WorldState, objKey string) startupManifestGraphProvenance {
	provenance := startupManifestGraphProvenance{
		source:    "unknown",
		protected: true,
	}
	if startupManifestGraphObjectKeyIsReleaseSource(objKey) {
		provenance.source = "global-release"
		provenance.derived = true
		provenance.protected = false
	} else if startupManifestGraphHasBuildResult(ctx, ws, objKey) {
		provenance.source = "project-build"
		provenance.derived = true
		provenance.protected = false
	} else if startupManifestGraphObjectKeyIsSpaceLocalOrEphemeral(objKey) {
		provenance.source = "space-local-or-ephemeral"
	}
	return provenance
}

func startupManifestGraphObjectKeyIsReleaseSource(objKey string) bool {
	return strings.HasPrefix(objKey, "release/manifests/") ||
		strings.HasPrefix(objKey, "spacewave/release/manifests/")
}

func startupManifestGraphObjectKeyIsSpaceLocalOrEphemeral(objKey string) bool {
	return strings.HasPrefix(objKey, "spaces/") ||
		strings.HasPrefix(objKey, "space/") ||
		strings.HasPrefix(objKey, "shared-object/") ||
		strings.HasPrefix(objKey, "ephemeral/") ||
		strings.Contains(objKey, "/ephemeral/")
}

func startupManifestGraphHasBuildResult(ctx context.Context, ws world.WorldState, objKey string) bool {
	objType, err := world_types.GetObjectType(ctx, ws, objKey+"/build-result")
	return err == nil && objType == "bldr/manifest-build-result"
}

func startupManifestGraphPartHasPrefix(parts []string, prefix string) bool {
	for _, part := range parts {
		if strings.HasPrefix(part, prefix) {
			return true
		}
	}
	return false
}

func startupManifestGraphPartValue(parts []string, prefix string) string {
	for _, part := range parts {
		if after, ok := strings.CutPrefix(part, prefix); ok {
			return after
		}
	}
	return ""
}
