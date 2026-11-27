package sobject

import (
	"slices"
	"strings"
)

// EqualSOConfigs compares every configuration field, ignoring participant order.
// Membership is keyed by peer ID. Its projection order carries no authority.
// Inputs are neither mutated nor validated. Signed encodings and chain hashes
// retain their original order and must be verified separately.
func EqualSOConfigs(a, b *SharedObjectConfig) bool {
	// Preserve absence without treating a missing configuration as an empty one.
	if a == nil || b == nil {
		return a == b
	}

	// Normalize independent copies so signed data retains its exact encoding.
	left, right := a.CloneVT(), b.CloneVT()
	for _, config := range []*SharedObjectConfig{left, right} {
		slices.SortFunc(config.Participants, func(a, b *SOParticipantConfig) int {
			return strings.Compare(a.GetPeerId(), b.GetPeerId())
		})
	}
	return left.EqualVT(right)
}
