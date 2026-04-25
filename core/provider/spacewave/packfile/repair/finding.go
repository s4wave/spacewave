package repair

import "slices"

import packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"

// Reason identifies why a packfile entry needs metadata repair.
type Reason string

const (
	// ReasonMissingBloom indicates the entry has no bloom metadata.
	ReasonMissingBloom Reason = "missing-bloom"
	// ReasonMalformedBloom indicates bloom metadata cannot be decoded.
	ReasonMalformedBloom Reason = "malformed-bloom"
	// ReasonIncompatibleBloom indicates bloom parameters do not match policy.
	ReasonIncompatibleBloom Reason = "incompatible-bloom"
	// ReasonUnderCapacity indicates the recorded block count exceeds policy.
	ReasonUnderCapacity Reason = "under-capacity"
	// ReasonMissingBlockCount indicates the entry has no block count metadata.
	ReasonMissingBlockCount Reason = "missing-block-count"
	// ReasonMissingCreatedAt indicates the entry has no creation timestamp.
	ReasonMissingCreatedAt Reason = "missing-created-at"
	// ReasonMissingSize indicates the entry has no pack byte size.
	ReasonMissingSize Reason = "missing-size"
)

// Finding describes one packfile entry that needs metadata repair.
type Finding struct {
	Entry                   *packfile.PackfileEntry
	Reasons                 []Reason
	EstimatedFalsePositive  float64
	RepairedBlockCount      uint64
	RepairedBloomBytes      int
	PackSha256Hex           string
	VerifiedIndexedBlockCnt uint64
}

func (f *Finding) addReason(reason Reason) {
	if slices.Contains(f.Reasons, reason) {
		return
	}
	f.Reasons = append(f.Reasons, reason)
}
