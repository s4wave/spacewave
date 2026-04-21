package link_solicit

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

func TestComputeSessionID(t *testing.T) {
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
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

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

	h5 := ComputeProtocolHash(sid, protocol.ID("test/echo"), []byte("bucket-b"))
	if bytes.Equal(h4, h5) {
		t.Fatal("different contexts should produce different hashes")
	}

	if len(h1) != HashSize {
		t.Fatalf("expected %d bytes, got %d", HashSize, len(h1))
	}
}

func TestComputeProtocolHashes(t *testing.T) {
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	entries := []SolicitEntry{
		{ProtocolID: protocol.ID("z-proto"), Context: nil},
		{ProtocolID: protocol.ID("a-proto"), Context: nil},
		{ProtocolID: protocol.ID("m-proto"), Context: []byte("ctx")},
	}
	hashes := ComputeProtocolHashes(sid, entries)
	if len(hashes) != 3 {
		t.Fatalf("expected 3 hashes, got %d", len(hashes))
	}

	// Verify sorted.
	for i := 1; i < len(hashes); i++ {
		if bytes.Compare(hashes[i-1], hashes[i]) >= 0 {
			t.Fatal("hashes should be sorted")
		}
	}
}

func TestFindMatchingHashes(t *testing.T) {
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	h1 := ComputeProtocolHash(sid, protocol.ID("shared"), nil)
	h2 := ComputeProtocolHash(sid, protocol.ID("only-local"), nil)
	h3 := ComputeProtocolHash(sid, protocol.ID("only-remote"), nil)
	h4 := ComputeProtocolHash(sid, protocol.ID("shared2"), nil)

	local := [][]byte{h1, h2, h4}
	remote := [][]byte{h1, h3, h4}
	SortHashes(local)
	SortHashes(remote)

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
	sid := ComputeSessionID(peer.ID("a"), peer.ID("b"))

	h1 := ComputeProtocolHash(sid, protocol.ID("local-only"), nil)
	h2 := ComputeProtocolHash(sid, protocol.ID("remote-only"), nil)

	local := [][]byte{h1}
	remote := [][]byte{h2}
	SortHashes(local)
	SortHashes(remote)

	matches := FindMatchingHashes(local, remote)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFindMatchingHashesEmpty(t *testing.T) {
	matches := FindMatchingHashes(nil, nil)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}

	h := [][]byte{{1, 2, 3}}
	matches = FindMatchingHashes(h, nil)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}

	matches = FindMatchingHashes(nil, h)
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}
