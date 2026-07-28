package bldr_manifest_world

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// StartupManifestEligibility is the retained startup candidate classification.
type StartupManifestEligibility string

const (
	// StartupManifestEligibilityEligible is a readable exact-label candidate.
	StartupManifestEligibilityEligible StartupManifestEligibility = "eligible"
	// StartupManifestEligibilityCompatibleLegacy is a readable legacy empty-label candidate.
	StartupManifestEligibilityCompatibleLegacy StartupManifestEligibility = "compatible-legacy"
	// StartupManifestEligibilityQuarantined is readable but incompatible with the requested manifest.
	StartupManifestEligibilityQuarantined StartupManifestEligibility = "quarantined"
	// StartupManifestEligibilityIgnored is not a selectable startup manifest for this request.
	StartupManifestEligibilityIgnored StartupManifestEligibility = "ignored"
	// StartupManifestEligibilityUnsafe cannot be read or validated safely.
	StartupManifestEligibilityUnsafe StartupManifestEligibility = "unsafe"
)

// StartupManifestCandidateEligibility describes one retained startup candidate.
type StartupManifestCandidateEligibility struct {
	// ObjectKey is the candidate object key.
	ObjectKey string
	// EdgeLabel is the manifest graph label that reached the candidate.
	EdgeLabel string
	// Eligibility is the candidate classification.
	Eligibility StartupManifestEligibility
	// Reason is a compact diagnostic reason.
	Reason string
	// ManifestID is the candidate manifest ID when known.
	ManifestID string
	// PlatformID is the candidate platform ID when known.
	PlatformID string
	// Rev is the candidate manifest revision when known.
	Rev uint64
	// Manifest is the decoded Manifest when the candidate is readable.
	Manifest *bldr_manifest.Manifest
	// ObjectRef is the candidate object's root reference when known.
	ObjectRef *bucket.ObjectRef
	// ManifestRef is the referenced manifest object when known.
	ManifestRef *bucket.ObjectRef
}

// collectStartupManifestEligibilityForManifestID classifies startup manifest
// candidates without mutating the Manifest graph or candidate objects.
func collectStartupManifestEligibilityForManifestID(
	ctx context.Context,
	ws world.WorldState,
	manifestID string,
	filterPlatformIDs []string,
	objKeys ...string,
) ([]*StartupManifestCandidateEligibility, error) {
	edges, candidates, err := collectStartupManifestGraph(ctx, ws, manifestID, objKeys...)
	if err != nil {
		return nil, err
	}
	edgeLabels := startupManifestCandidateEdgeLabels(edges, manifestID)
	out := make([]*StartupManifestCandidateEligibility, 0, len(candidates))
	for _, objKey := range candidates {
		candidate, err := classifyStartupManifestCandidateEligibility(
			ctx,
			ws,
			objKey,
			edgeLabels[objKey],
			manifestID,
			filterPlatformIDs,
		)
		if err != nil {
			return out, err
		}
		out = append(out, candidate)
	}
	return out, nil
}

// SummarizeStartupManifestEligibility builds a compact status string.
func SummarizeStartupManifestEligibility(candidates []*StartupManifestCandidateEligibility, maxItems int) string {
	if len(candidates) == 0 || maxItems == 0 {
		return ""
	}
	if maxItems < 0 || maxItems > len(candidates) {
		maxItems = len(candidates)
	}
	items := make([]string, 0, maxItems)
	for _, candidate := range candidates[:maxItems] {
		items = append(items, candidate.Summary())
	}
	summary := strings.Join(items, "; ")
	if remaining := len(candidates) - maxItems; remaining > 0 {
		summary += "; +" + strconv.Itoa(remaining) + " more"
	}
	return summary
}

// Summary returns one compact diagnostic item.
func (c *StartupManifestCandidateEligibility) Summary() string {
	if c == nil {
		return "<nil>"
	}
	parts := []string{
		c.ObjectKey,
		string(c.Eligibility),
		c.Reason,
	}
	if c.ManifestID != "" {
		parts = append(parts, "manifest_id="+c.ManifestID)
	}
	if c.PlatformID != "" {
		parts = append(parts, "platform_id="+c.PlatformID)
	}
	if c.Rev != 0 {
		parts = append(parts, "rev="+strconv.FormatUint(c.Rev, 10))
	}
	if ref := c.ManifestRef; ref != nil && !ref.GetEmpty() {
		if bucketID := ref.GetBucketId(); bucketID != "" {
			parts = append(parts, "bucket="+bucketID)
		}
		if rootRef := ref.GetRootRef(); rootRef != nil && !rootRef.GetEmpty() {
			parts = append(parts, "root="+rootRef.MarshalString())
		}
	}
	return strings.Join(parts, " ")
}

// SelectableStartupManifests returns eligible and compatible legacy candidates
// as CollectedManifest values for scheduler selection.
func SelectableStartupManifests(candidates []*StartupManifestCandidateEligibility) []*CollectedManifest {
	out := make([]*CollectedManifest, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.Manifest == nil || candidate.ManifestRef == nil {
			continue
		}
		switch candidate.Eligibility {
		case StartupManifestEligibilityEligible, StartupManifestEligibilityCompatibleLegacy:
		default:
			continue
		}
		out = append(out, &CollectedManifest{
			Manifest:    candidate.Manifest,
			ManifestRef: candidate.ManifestRef,
			ManifestKey: candidate.ObjectKey,
		})
	}
	slices.SortStableFunc(out, func(a, b *CollectedManifest) int {
		if a.GetRev() > b.GetRev() {
			return -1
		}
		if a.GetRev() < b.GetRev() {
			return 1
		}
		return strings.Compare(a.ManifestRef.String(), b.ManifestRef.String())
	})
	return out
}

func startupManifestCandidateEdgeLabels(edges []startupManifestGraphEdge, manifestID string) map[string]string {
	labels := make(map[string]string)
	exactLabel := ""
	if manifestID != "" {
		exactLabel = quad.IRI(manifestID).String()
	}
	for _, edge := range edges {
		current, ok := labels[edge.to]
		if !ok || (exactLabel != "" && current != exactLabel && edge.label == exactLabel) {
			labels[edge.to] = edge.label
		}
	}
	return labels
}

func classifyStartupManifestCandidateEligibility(
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	edgeLabel string,
	expectedManifestID string,
	filterPlatformIDs []string,
) (*StartupManifestCandidateEligibility, error) {
	candidate := &StartupManifestCandidateEligibility{
		ObjectKey: objKey,
		EdgeLabel: edgeLabel,
	}

	objType, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		if ctxErr := startupContextError(err); ctxErr != nil {
			return nil, ctxErr
		}
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "object-type:" + err.Error()
		return candidate, nil
	}

	switch objType {
	case ManifestStoreTypeID, ManifestBundleTypeID:
		candidate.Eligibility = StartupManifestEligibilityIgnored
		candidate.Reason = "intermediate:" + objType
		return candidate, nil
	case ManifestTypeID:
		return classifyDirectStartupManifestEligibility(ctx, ws, candidate, expectedManifestID, filterPlatformIDs)
	default:
		return classifyManifestRefStartupEligibility(ctx, ws, candidate, objType, expectedManifestID, filterPlatformIDs)
	}
}

func classifyDirectStartupManifestEligibility(
	ctx context.Context,
	ws world.WorldState,
	candidate *StartupManifestCandidateEligibility,
	expectedManifestID string,
	filterPlatformIDs []string,
) (*StartupManifestCandidateEligibility, error) {
	manifest, manifestRef, err := lookupStartupManifestObject(ctx, ws, candidate.ObjectKey)
	if err != nil {
		if ctxErr := startupContextError(err); ctxErr != nil {
			return nil, ctxErr
		}
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-read:" + err.Error()
		return candidate, nil
	}
	candidate.Manifest = manifest
	candidate.ManifestRef = manifestRef.Clone()
	if err := manifest.Validate(); err != nil {
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-validate:" + err.Error()
		return candidate, nil
	}
	return classifyReadableStartupManifest(candidate, manifest.GetMeta(), expectedManifestID, filterPlatformIDs), nil
}

func classifyManifestRefStartupEligibility(
	ctx context.Context,
	ws world.WorldState,
	candidate *StartupManifestCandidateEligibility,
	objType string,
	expectedManifestID string,
	filterPlatformIDs []string,
) (*StartupManifestCandidateEligibility, error) {
	manifestRef, objectRef, err := LookupManifestRef(ctx, ws, candidate.ObjectKey)
	if err != nil {
		if ctxErr := startupContextError(err); ctxErr != nil {
			return nil, ctxErr
		}
		if objType == "" {
			return classifyUnknownTypedStartupCandidate(ctx, ws, candidate, expectedManifestID, filterPlatformIDs, err)
		}
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-ref-read:" + err.Error()
		return candidate, nil
	}
	candidate.ObjectRef = objectRef.Clone()
	candidate.ManifestRef = manifestRef.GetManifestRef().Clone()
	if err := manifestRef.Validate(); err != nil {
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-ref-validate:" + err.Error()
		return candidate, nil
	}

	meta := manifestRef.GetMeta()
	if expectedManifestID != "" && meta.GetManifestId() != expectedManifestID {
		candidate.ManifestID = meta.GetManifestId()
		candidate.PlatformID = meta.GetPlatformId()
		candidate.Rev = meta.GetRev()
		candidate.Eligibility = StartupManifestEligibilityQuarantined
		candidate.Reason = "manifest-id-mismatch:" + meta.GetManifestId()
		return candidate, nil
	}
	if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, meta.GetPlatformId()) {
		candidate.ManifestID = meta.GetManifestId()
		candidate.PlatformID = meta.GetPlatformId()
		candidate.Rev = meta.GetRev()
		candidate.Eligibility = StartupManifestEligibilityIgnored
		candidate.Reason = "platform-filtered:" + meta.GetPlatformId()
		return candidate, nil
	}

	manifest, err := lookupStartupManifestObjectRefLocal(ctx, ws, manifestRef.GetManifestRef())
	if err != nil {
		if ctxErr := startupContextError(err); ctxErr != nil {
			return nil, ctxErr
		}
		candidate.ManifestID = meta.GetManifestId()
		candidate.PlatformID = meta.GetPlatformId()
		candidate.Rev = meta.GetRev()
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-ref-unreadable:" + err.Error()
		return candidate, nil
	}
	candidate.Manifest = manifest
	if !manifest.GetMeta().EqualVT(meta) {
		candidate.ManifestID = meta.GetManifestId()
		candidate.PlatformID = meta.GetPlatformId()
		candidate.Rev = meta.GetRev()
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-meta-mismatch"
		return candidate, nil
	}
	if err := manifest.Validate(); err != nil {
		candidate.ManifestID = meta.GetManifestId()
		candidate.PlatformID = meta.GetPlatformId()
		candidate.Rev = meta.GetRev()
		candidate.Eligibility = StartupManifestEligibilityUnsafe
		candidate.Reason = "manifest-validate:" + err.Error()
		return candidate, nil
	}
	return classifyReadableStartupManifest(candidate, meta, expectedManifestID, filterPlatformIDs), nil
}

func classifyUnknownTypedStartupCandidate(
	ctx context.Context,
	ws world.WorldState,
	candidate *StartupManifestCandidateEligibility,
	expectedManifestID string,
	filterPlatformIDs []string,
	refErr error,
) (*StartupManifestCandidateEligibility, error) {
	manifest, manifestRef, manifestErr := lookupStartupManifestObject(ctx, ws, candidate.ObjectKey)
	if manifestErr == nil && manifest != nil {
		candidate.Manifest = manifest
		candidate.ManifestRef = manifestRef.Clone()
		if err := manifest.Validate(); err != nil {
			candidate.Eligibility = StartupManifestEligibilityUnsafe
			candidate.Reason = "manifest-validate:" + err.Error()
			return candidate, nil
		}
		return classifyReadableStartupManifest(candidate, manifest.GetMeta(), expectedManifestID, filterPlatformIDs), nil
	}
	if _, _, bundleErr := LookupManifestBundle(ctx, ws, candidate.ObjectKey); bundleErr == nil {
		candidate.Eligibility = StartupManifestEligibilityIgnored
		candidate.Reason = "intermediate:" + ManifestBundleTypeID
		return candidate, nil
	}
	candidate.Eligibility = StartupManifestEligibilityUnsafe
	candidate.Reason = "manifest-ref-read:" + errors.Cause(refErr).Error()
	return candidate, nil
}

func classifyReadableStartupManifest(
	candidate *StartupManifestCandidateEligibility,
	meta *bldr_manifest.ManifestMeta,
	expectedManifestID string,
	filterPlatformIDs []string,
) *StartupManifestCandidateEligibility {
	candidate.ManifestID = meta.GetManifestId()
	candidate.PlatformID = meta.GetPlatformId()
	candidate.Rev = meta.GetRev()
	if expectedManifestID != "" && candidate.ManifestID != expectedManifestID {
		candidate.Eligibility = StartupManifestEligibilityQuarantined
		candidate.Reason = "manifest-id-mismatch:" + candidate.ManifestID
		return candidate
	}
	if len(filterPlatformIDs) != 0 && !slices.Contains(filterPlatformIDs, candidate.PlatformID) {
		candidate.Eligibility = StartupManifestEligibilityIgnored
		candidate.Reason = "platform-filtered:" + candidate.PlatformID
		return candidate
	}
	if candidate.EdgeLabel == "" && expectedManifestID != "" {
		candidate.Eligibility = StartupManifestEligibilityCompatibleLegacy
		candidate.Reason = "legacy-empty-label"
		return candidate
	}
	candidate.Eligibility = StartupManifestEligibilityEligible
	candidate.Reason = "exact-label"
	return candidate
}
