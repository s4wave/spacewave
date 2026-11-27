package link_solicit

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

func TestComputeSessionID(t *testing.T) {
	// Choose distinct peers for the canonical session comparison.
	peerA := peer.ID("peer-aaa")
	peerB := peer.ID("peer-bbb")

	// Same result regardless of argument order.
	s1 := ComputeSessionID(peerA, peerB)
	s2 := ComputeSessionID(peerB, peerA)
	if !bytes.Equal(s1, s2) {
		t.Fatal("session IDs should be equal regardless of peer order")
	}

	// Different peers produce different session ID.
	peerC := peer.ID("peer-ccc")
	s3 := ComputeSessionID(peerA, peerC)
	if bytes.Equal(s1, s3) {
		t.Fatal("different peers should produce different session IDs")
	}

	// Hash is always 32 bytes.
	if len(s1) != HashSize {
		t.Fatalf("expected %d bytes, got %d", HashSize, len(s1))
	}
}

func TestComputeProtocolHash(t *testing.T) {
	// Create a session shared by the protocol hash cases.
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	// Check that identical solicitation inputs produce identical hashes.
	h1 := ComputeProtocolHash(sid, protocol.ID("test/echo"), nil)
	h2 := ComputeProtocolHash(sid, protocol.ID("test/echo"), nil)
	if !bytes.Equal(h1, h2) {
		t.Fatal("same inputs should produce same hash")
	}

	// Different protocol ID.
	h3 := ComputeProtocolHash(sid, protocol.ID("test/other"), nil)
	if bytes.Equal(h1, h3) {
		t.Fatal("different protocol IDs should produce different hashes")
	}

	// Different context.
	h4 := ComputeProtocolHash(sid, protocol.ID("test/echo"), []byte("bucket-a"))
	if bytes.Equal(h1, h4) {
		t.Fatal("different contexts should produce different hashes")
	}

	// Check that two nonempty contexts produce different solicitation hashes.
	h5 := ComputeProtocolHash(sid, protocol.ID("test/echo"), []byte("bucket-b"))
	if bytes.Equal(h4, h5) {
		t.Fatal("different contexts should produce different hashes")
	}

	// Require the solicitation digest to have the protocol hash size.
	if len(h1) != HashSize {
		t.Fatalf("expected %d bytes, got %d", HashSize, len(h1))
	}
}

func TestComputeProtocolHashes(t *testing.T) {
	// Create a session for the protocol hash collection.
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	// Hash three protocol and context entries for the session.
	entries := []SolicitEntry{
		{ProtocolID: protocol.ID("z-proto"), Context: nil},
		{ProtocolID: protocol.ID("a-proto"), Context: nil},
		{ProtocolID: protocol.ID("m-proto"), Context: []byte("ctx")},
	}
	hashes := ComputeProtocolHashes(sid, entries)
	if len(hashes) != 3 {
		t.Fatalf("expected 3 hashes, got %d", len(hashes))
	}

	// Verify hashes are sorted.
	for i := 1; i < len(hashes); i++ {
		if bytes.Compare(hashes[i-1], hashes[i]) >= 0 {
			t.Fatal("hashes should be sorted")
		}
	}
}

func TestFindMatchingHashes(t *testing.T) {
	// Create a session for overlapping solicitation sets.
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	// Hash shared and endpoint-specific protocols for the matching cases.
	h1 := ComputeProtocolHash(sid, protocol.ID("shared"), nil)
	h2 := ComputeProtocolHash(sid, protocol.ID("only-local"), nil)
	h3 := ComputeProtocolHash(sid, protocol.ID("only-remote"), nil)
	h4 := ComputeProtocolHash(sid, protocol.ID("shared2"), nil)

	// Sort both endpoint hash sets for the intersection scan.
	local := [][]byte{h1, h2, h4}
	remote := [][]byte{h1, h3, h4}
	SortHashes(local)
	SortHashes(remote)

	// Require both shared solicitation hashes in the intersection.
	matches := FindMatchingHashes(local, remote)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Verify matches contain h1 and h4.
	found := make(map[string]bool)
	for _, m := range matches {
		found[string(m)] = true
	}
	if !found[string(h1)] || !found[string(h4)] {
		t.Fatal("matches should contain both shared hashes")
	}
}

func TestFindMatchingHashesDisjoint(t *testing.T) {
	// Create a session for disjoint solicitation sets.
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	// Hash a different protocol for each endpoint.
	h1 := ComputeProtocolHash(sid, protocol.ID("local-only"), nil)
	h2 := ComputeProtocolHash(sid, protocol.ID("remote-only"), nil)

	// Sort the disjoint endpoint hash sets for the intersection scan.
	local := [][]byte{h1}
	remote := [][]byte{h2}
	SortHashes(local)
	SortHashes(remote)

	// Require no matches between the disjoint solicitation sets.
	matches := FindMatchingHashes(local, remote)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFindMatchingHashesEmpty(t *testing.T) {
	// Require no matches when both solicitation sets are empty.
	matches := FindMatchingHashes(nil, nil)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}

	// Require no matches when only the remote solicitation set is empty.
	h := [][]byte{{1, 2, 3}}
	matches = FindMatchingHashes(h, nil)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}

	// Require no matches when only the local solicitation set is empty.
	matches = FindMatchingHashes(nil, h)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

// TestComputeHashesKnownAnswers checks the hashes against values computed by
// native Go. Both peers of a link must compute the same hashes, so a compiler
// that hashes differently, including the GoScript discrepancy for BLAKE3
// inputs over 64 bytes, fails here even though two peers it compiles still
// agree with each other.
func TestComputeHashesKnownAnswers(t *testing.T) {
	sid := ComputeSessionID(
		peer.ID("12D3KooWNrAtf3h7dZisrUer4VPRw5djhq2DyipePVooXo3cW9Mn"),
		peer.ID("12D3KooWSguLJqha8Lpt6qjMm85UiA7oFGzMgAaVqDzAtf78MDb8"),
	)
	if got, want := hex.EncodeToString(sid), "5fb4d834c723cc03265c034d59de74a20c41fcb74fc016fa34e24cddeeb217b4"; got != want {
		t.Fatalf("session ID = %s, want %s", got, want)
	}
	h := ComputeProtocolHash(sid, protocol.ID("alpha/so-sync/2"), []byte("01m5x0nce0etbcxc9sr7t82m92"))
	if got, want := hex.EncodeToString(h), "3fb9506638de97373bcc70874de3cd7229b71ba00bf125653b6f76823f4aa892"; got != want {
		t.Fatalf("protocol hash = %s, want %s", got, want)
	}
}
