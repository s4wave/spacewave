package link_solicit

import (
	"bytes"
	"slices"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/zeebo/blake3"
)

// HashSize is the size of a BLAKE3 hash in bytes.
const HashSize = 32

// SolicitEntry is a (protocolID, context) pair for hash computation.
type SolicitEntry struct {
	ProtocolID protocol.ID
	Context    []byte
}

// ComputeSessionID returns BLAKE3(lower_peer || higher_peer).
// Peers are lexicographically sorted to produce a canonical ordering.
// Both sides of a link compute the same session ID since the peer IDs
// are the same regardless of which side you're on.
func ComputeSessionID(peerA, peerB peer.ID) []byte {
	lower, higher := peerA, peerB
	if lower > higher {
		lower, higher = higher, lower
	}

	h := blake3.New()
	h.Write([]byte(lower))
	h.Write([]byte(higher))

	sum := h.Sum(nil)
	return sum[:HashSize]
}

// ComputeProtocolHash returns BLAKE3(session_id || protocol_id || context).
func ComputeProtocolHash(sessionID []byte, protocolID protocol.ID, context []byte) []byte {
	h := blake3.New()
	h.Write(sessionID)
	h.Write([]byte(protocolID))
	h.Write(context)

	sum := h.Sum(nil)
	return sum[:HashSize]
}

// ComputeProtocolHashes returns sorted hashes for a set of (protocolID, context) pairs.
func ComputeProtocolHashes(sessionID []byte, entries []SolicitEntry) [][]byte {
	hashes := make([][]byte, len(entries))
	for i, e := range entries {
		hashes[i] = ComputeProtocolHash(sessionID, e.ProtocolID, e.Context)
	}
	SortHashes(hashes)
	return hashes
}

// SortHashes sorts a slice of hashes lexicographically.
func SortHashes(hashes [][]byte) {
	slices.SortFunc(hashes, bytes.Compare)
}

// FindMatchingHashes returns the hashes present in both sorted sets.
// Both input slices must be sorted.
func FindMatchingHashes(local, remote [][]byte) [][]byte {
	var matches [][]byte
	i, j := 0, 0
	for i < len(local) && j < len(remote) {
		cmp := bytes.Compare(local[i], remote[j])
		if cmp == 0 {
			matches = append(matches, slices.Clone(local[i]))
			i++
			j++
		} else if cmp < 0 {
			i++
		} else {
			j++
		}
	}
	return matches
}
