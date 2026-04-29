package publish

import (
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/db/bucket"
)

// PublishPlan describes the pack and root delta between a source and destination Space.
type PublishPlan struct {
	MissingPackIDs []string
	NeedRootPost   bool
}

// BuildPublishPlan computes which source packs are absent from the destination
// and whether the destination root pointer must be updated.
func BuildPublishPlan(
	srcPacks []*packfile.PackfileEntry,
	dstPacks []*packfile.PackfileEntry,
	srcHeadRef *bucket.ObjectRef,
	dstHeadRef *bucket.ObjectRef,
) *PublishPlan {
	dstPackSet := make(map[string]struct{}, len(dstPacks))
	for _, entry := range dstPacks {
		dstPackSet[entry.GetId()] = struct{}{}
	}
	missingPackIDs := make([]string, 0)
	for _, entry := range srcPacks {
		if _, ok := dstPackSet[entry.GetId()]; !ok {
			missingPackIDs = append(missingPackIDs, entry.GetId())
		}
	}
	needRootPost := true
	if srcHeadRef == nil && dstHeadRef == nil {
		needRootPost = false
	} else if srcHeadRef != nil && dstHeadRef != nil && srcHeadRef.EqualVT(dstHeadRef) {
		needRootPost = false
	}
	return &PublishPlan{
		MissingPackIDs: missingPackIDs,
		NeedRootPost:   needRootPost,
	}
}
