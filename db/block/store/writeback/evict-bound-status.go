package block_store_writeback

// Labels of the entries a bounded pass refuses, a closed set.
const (
	// EvictRefusedMarked labels an entry whose block awaits its upload.
	EvictRefusedMarked = "marked"
	// EvictRefusedUnlocated labels an entry the local index places in no
	// pack.
	EvictRefusedUnlocated = "unlocated"
	// EvictRefusedAbsent labels an entry whose located pack verified but
	// does not hold it.
	EvictRefusedAbsent = "absent"
	// EvictRefusedOther labels a verifier reason outside the set.
	EvictRefusedOther = "other"
)

// EvictRefusalLabels are every label of EvictBoundStatus.Refused: the
// refusals above and the pack verify failures.
var EvictRefusalLabels = []string{
	EvictRefusedMarked, EvictRefusedUnlocated, EvictRefusedAbsent,
	"missing", "oversize", "short", "index", "hash", "fetch", "entry",
	"unregistered", EvictRefusedOther,
}

// evictLastingRefusals are the verify failures that cannot heal while the
// bucket holds the same pack and entry objects: each is a fact about those
// bytes, or about the pack's recorded size and sum, which never change for a
// pack id. A pass keeps such a refusal with the pack's identity and does not
// fetch the pack again until the identity changes. A fetch failure, a pack
// out of the plain registered state and an unknown reason heal on their own
// and are tried again on the next pass.
var evictLastingRefusals = map[string]bool{
	"missing": true, "oversize": true, "short": true, "index": true, "hash": true, "entry": true,
}

// refusalLabel returns reason when it is one of EvictRefusalLabels, else
// EvictRefusedOther.
func refusalLabel(reason string) string {
	for _, label := range EvictRefusalLabels {
		if label == reason {
			return label
		}
	}
	return EvictRefusedOther
}

// EvictBoundStatus reports the bounded passes since the store opened.
type EvictBoundStatus struct {
	// BudgetKnown reports a pass read a budget's bytes left.
	BudgetKnown bool
	// BudgetLeft is, once BudgetKnown, the persisted bytes left of the last
	// budget read, without the reservation of a batch in flight.
	BudgetLeft int64
	// VerifiedBytes is the size of the values of the packs fetched and
	// verified whole.
	VerifiedBytes int64
	// Refused counts the queue entries refused, by EvictRefusalLabels. The
	// map is replaced, never changed, so a copied status keeps its counts.
	Refused map[string]int64
	// DroppedAhead is how many blocks bounded passes dropped with their pack
	// ahead of the walk since the store opened, each deleting its queue entry
	// in the same commit.
	DroppedAhead int64
}

// addEvictBoundStatus projects a bounded batch's committed changes.
func (s *Store) addEvictBoundStatus(left int64, leftKnown bool, verified int64, refused map[string]int64, droppedAhead int64) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Replace the refusal map rather than change it, so copies keep
		// their counts.
		b := &s.status.Bound
		if leftKnown {
			b.BudgetKnown, b.BudgetLeft = true, left
		}
		b.VerifiedBytes += verified
		if len(refused) != 0 {
			next := make(map[string]int64, len(b.Refused)+len(refused))
			for k, v := range b.Refused {
				next[k] = v
			}
			for k, v := range refused {
				next[k] += v
			}
			b.Refused = next
		}
		b.DroppedAhead += droppedAhead
		broadcast()
	})
}
