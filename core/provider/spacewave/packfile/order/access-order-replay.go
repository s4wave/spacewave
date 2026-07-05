package order

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ReplayAccessOrderRecord orders refs by record file access, then graph fallback.
func ReplayAccessOrderRecord(
	ctx context.Context,
	graph RefGraph,
	identity AccessOrderManifestIdentity,
	record *AccessOrderRecord,
	refs []*block.BlockRef,
	resolver AccessOrderPathResolver,
) (*AccessOrderReplayResult, error) {
	fallback, err := BlockRefs(ctx, graph, refs)
	if err != nil {
		return nil, err
	}
	res := &AccessOrderReplayResult{}
	if record == nil || !identity.MatchesRecord(record) {
		res.Refs = fallback
		res.StaleRecord = true
		return res, nil
	}

	candidates := make(map[string]*block.BlockRef, len(refs))
	for _, ref := range refs {
		key := refKey(ref)
		if key == "" {
			continue
		}
		candidates[key] = ref
	}

	seen := make(map[string]struct{}, len(refs))
	ordered := make([]*block.BlockRef, 0, len(refs))
	for _, entry := range record.GetEntries() {
		entryRefs, found, err := accessOrderEntryRefs(ctx, entry, resolver)
		if err != nil {
			return nil, err
		}
		if !found {
			res.StaleRecord = true
			res.MissingPaths = append(res.MissingPaths, entry.GetPath())
			continue
		}
		used := false
		for _, ref := range entryRefs {
			key := refKey(ref)
			if key == "" {
				continue
			}
			candidate, ok := candidates[key]
			if !ok {
				res.StaleRecord = true
				res.MissingRefs = append(res.MissingRefs, ref)
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ordered = append(ordered, candidate)
			used = true
		}
		if used {
			res.UsedEntries++
		}
	}

	for _, ref := range fallback {
		key := refKey(ref)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		ordered = append(ordered, ref)
	}
	res.Refs = ordered
	return res, nil
}

func accessOrderEntryRefs(ctx context.Context, entry *AccessOrderEntry, resolver AccessOrderPathResolver) ([]*block.BlockRef, bool, error) {
	if entry == nil {
		return nil, false, nil
	}
	if resolver != nil {
		refs, found, err := resolver.ResolveAccessOrderPath(ctx, entry.GetFilesystem(), entry.GetPath())
		if err != nil {
			return nil, false, errors.Wrap(err, "resolve access order path")
		}
		return refs, found, nil
	}
	refs := entry.GetResolvedRefs()
	return refs, len(refs) != 0, nil
}
