package bldr_manifest_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// StartupManifestPruneProof carries the non-source proof gates required before
// a retained startup Manifest candidate may be pruned.
type StartupManifestPruneProof struct {
	// Reachability attests that no reachable graph path still references the
	// candidate.
	Reachability bool
	// Quarantine attests the candidate is quarantined.
	Quarantine bool
	// CopiedStateRelaunch attests a copied-state relaunch has been captured.
	CopiedStateRelaunch bool
}

// StartupManifestPruneResult describes a pruning attempt.
type StartupManifestPruneResult struct {
	// ObjectKey is the pruned candidate's object key.
	ObjectKey string
	// Pruned reports whether any pruning action was taken.
	Pruned bool
	// Reason explains why the candidate was skipped or pruned.
	Reason string
	// DeletedEdges is the number of deleted graph edges.
	DeletedEdges int
	// DeletedObject reports whether the object itself was deleted.
	DeletedObject bool
}

// PruneStartupManifestCandidate removes a retained derived startup Manifest
// candidate only after provenance, reachability, quarantine, and copied-state
// relaunch proof are all present. It deletes graph/object refs only; block
// bytes remain owned by normal world/volume GC.
func PruneStartupManifestCandidate(
	ctx context.Context,
	ws world.WorldState,
	candidate *StartupManifestCandidateEligibility,
	proof StartupManifestPruneProof,
	rootObjKeys ...string,
) (*StartupManifestPruneResult, error) {
	if candidate == nil {
		return nil, errors.New("startup manifest prune candidate is nil")
	}
	res := &StartupManifestPruneResult{ObjectKey: candidate.ObjectKey}
	if candidate.ObjectKey == "" {
		res.Reason = "empty-object-key"
		return res, nil
	}

	provenance := classifyStartupManifestGraphProvenance(ctx, ws, candidate.ObjectKey)
	if !provenance.derived || provenance.protected {
		res.Reason = "source-protected:" + provenance.source
		return res, nil
	}
	if !startupManifestPruneCandidateIsQuarantined(candidate) {
		res.Reason = "not-quarantined:" + string(candidate.Eligibility)
		return res, nil
	}
	if !proof.Quarantine {
		res.Reason = "missing-quarantine-proof"
		return res, nil
	}
	if !proof.Reachability {
		res.Reason = "missing-reachability-proof"
		return res, nil
	}
	if !proof.CopiedStateRelaunch {
		res.Reason = "missing-copied-state-relaunch-proof"
		return res, nil
	}

	inbound, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys("", PredManifest.String(), candidate.ObjectKey, ""),
		0,
	)
	if err != nil {
		return nil, err
	}
	if len(inbound) == 0 {
		res.Reason = "no-startup-graph-ref"
		return res, nil
	}

	rootSet := make(map[string]struct{}, len(rootObjKeys))
	for _, root := range rootObjKeys {
		if root != "" {
			rootSet[root] = struct{}{}
		}
	}
	if len(rootSet) == 0 {
		res.Reason = "missing-root-scope"
		return res, nil
	}
	for _, q := range inbound {
		subj, err := world.GraphValueToKey(q.GetSubject())
		if err != nil {
			return nil, err
		}
		if _, ok := rootSet[subj]; !ok {
			res.Reason = "reachable-from-other-root:" + subj
			return res, nil
		}
	}

	outbound, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(candidate.ObjectKey, PredManifest.String(), "", ""),
		1,
	)
	if err != nil {
		return nil, err
	}
	if len(outbound) != 0 {
		res.Reason = "candidate-has-manifest-children"
		return res, nil
	}

	for _, q := range inbound {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return nil, err
		}
		res.DeletedEdges++
	}
	deleted, err := ws.DeleteObject(ctx, candidate.ObjectKey)
	if err != nil {
		return nil, err
	}
	res.DeletedObject = deleted
	res.Pruned = res.DeletedEdges != 0 || deleted
	res.Reason = "pruned"
	return res, nil
}

func startupManifestPruneCandidateIsQuarantined(candidate *StartupManifestCandidateEligibility) bool {
	return candidate.Eligibility == StartupManifestEligibilityQuarantined
}
