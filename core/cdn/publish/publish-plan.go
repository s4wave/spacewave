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

// BuildPublishPlan computes whether the destination root pointer must be
// updated. Pack IDs are resource-scoped, so source IDs cannot be compared
// directly to destination IDs; when the root differs, all source packs are
// copied and destination-side v1 IDs make retries idempotent.
func BuildPublishPlan(
	srcPacks []*packfile.PackfileEntry,
	_ []*packfile.PackfileEntry,
	srcHeadRef *bucket.ObjectRef,
	dstHeadRef *bucket.ObjectRef,
) *PublishPlan {
	needRootPost := true
	if srcHeadRef == nil && dstHeadRef == nil {
		needRootPost = false
	} else if srcHeadRef != nil && dstHeadRef != nil && srcHeadRef.EqualVT(dstHeadRef) {
		needRootPost = false
	}
	missingPackIDs := make([]string, 0, len(srcPacks))
	if needRootPost {
		for _, entry := range srcPacks {
			missingPackIDs = append(missingPackIDs, entry.GetId())
		}
	}
	return &PublishPlan{
		MissingPackIDs: missingPackIDs,
		NeedRootPost:   needRootPost,
	}
}
