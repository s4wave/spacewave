package sobject

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// mergeChain builds signed config chains over three owners for merge tests.
type mergeChain struct {
	t       *testing.T
	keys    []crypto.PrivKey
	ids     []string
	genesis *SOConfigChange
	// root is the config genesis yields.
	root *SharedObjectConfig
	// history retains every record by hash.
	history map[string]*SOConfigChange
	// readers holds the reader peer IDs the test adds.
	readers map[string]string
}

// newMergeChain signs a genesis that makes three peers owners.
func newMergeChain(t *testing.T) *mergeChain {
	// Make each mock peer an owner.
	t.Helper()
	peers := createMockPeers(t, 3)
	c := &mergeChain{t: t, history: make(map[string]*SOConfigChange), readers: make(map[string]string)}
	initial := &SharedObjectConfig{}
	for _, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		c.keys = append(c.keys, key)
		c.ids = append(c.ids, p.GetPeerID().String())
		initial.Participants = append(initial.Participants, &SOParticipantConfig{PeerId: p.GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER})
	}

	// Sign the genesis and hold the config it yields.
	c.genesis = c.sign(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, 0)
	c.root = c.apply(initial, c.genesis)
	return c
}

// sign builds and retains a record from current to next signed by owner i.
func (c *mergeChain) sign(current, next *SharedObjectConfig, kind SOConfigChangeType, i int) *SOConfigChange {
	// Sign the record and retain it.
	c.t.Helper()
	entry, err := BuildSOConfigChange(mockSharedObjectID, current, next, kind, c.keys[i], nil)
	if err != nil {
		c.t.Fatal(err)
	}
	c.retain(entry)
	return entry
}

// retain stores entry by its hash.
func (c *mergeChain) retain(entry *SOConfigChange) {
	c.t.Helper()
	hash, err := HashSOConfigChange(entry)
	if err != nil {
		c.t.Fatal(err)
	}
	c.history[string(hash)] = entry
}

// apply verifies entry on current and returns the config it yields.
func (c *mergeChain) apply(current *SharedObjectConfig, entry *SOConfigChange) *SharedObjectConfig {
	c.t.Helper()
	next, err := VerifyConfigChange(mockSharedObjectID, current, entry)
	if err != nil {
		c.t.Fatal(err)
	}
	return next
}

// reader returns the peer ID of the reader called name.
func (c *mergeChain) reader(name string) string {
	c.t.Helper()
	if _, ok := c.readers[name]; !ok {
		c.readers[name] = createMockPeers(c.t, 1)[0].GetPeerID().String()
	}
	return c.readers[name]
}

// addReader appends a record by owner i adding the reader called name.
func (c *mergeChain) addReader(current *SharedObjectConfig, i int, name string) (*SOConfigChange, *SharedObjectConfig) {
	// Sign the config with the reader added and apply it.
	c.t.Helper()
	next := current.CloneVT()
	next.Participants = append(next.Participants, &SOParticipantConfig{PeerId: c.reader(name), Role: SOParticipantRole_SOParticipantRole_READER})
	entry := c.sign(current, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, i)
	return entry, c.apply(current, entry)
}

// merge signs, as owner i, a merge on head absorbing branch after fork.
func (c *mergeChain) merge(head *SharedObjectConfig, fork *SOConfigChange, branch []*SOConfigChange, i int) (*SOConfigChange, *SharedObjectConfig) {
	// Sign the merge, retain it and apply it.
	c.t.Helper()
	entry, err := BuildSOConfigMerge(mockSharedObjectID, head, fork, branch, c.keys[i])
	if err != nil {
		c.t.Fatal(err)
	}
	c.retain(entry)
	return entry, c.apply(head, entry)
}

// read returns a retained record, or nil.
func (c *mergeChain) read(_ context.Context, hash []byte) (*SOConfigChange, error) {
	return c.history[string(hash)], nil
}

// hasPeer reports whether cfg lists peerID.
func hasPeer(cfg *SharedObjectConfig, peerID string) bool {
	return slices.ContainsFunc(cfg.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == peerID })
}

// TestConfigMergeDropsRecordOfFormerOwner keeps a branch's change only while
// its signer still owns the merged config, and rejects a merge whose config
// differs from the one its branch yields.
func TestConfigMergeDropsRecordOfFormerOwner(t *testing.T) {
	// Owner 0 adds a reader while owner 1 demotes owner 0.
	c := newMergeChain(t)
	added, _ := c.addReader(c.root, 0, "reader-of-demoted-owner")
	demoted := c.root.CloneVT()
	demoted.Participants[0].Role = SOParticipantRole_SOParticipantRole_READER
	demotion := c.sign(c.root, demoted, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, 1)
	head := c.apply(c.root, demotion)

	// Owner 1 merges, so the former owner's record is dropped.
	merge, merged := c.merge(head, c.genesis, []*SOConfigChange{added}, 1)
	if hasPeer(merged, c.reader("reader-of-demoted-owner")) || len(merged.GetParticipants()) != 3 {
		t.Fatal("merge applied a record whose signer no longer owns")
	}
	if merge.GetConfigSeqno() != 2 || len(merge.GetMerge().GetBranch()) != 1 {
		t.Fatal("merge does not extend the head with its branch")
	}

	// A merge carrying a config its branch does not yield fails verification.
	forged := merge.CloneVT()
	forged.Signatures = nil
	forged.Config.Participants = append(forged.Config.Participants, &SOParticipantConfig{PeerId: c.reader("reader-of-demoted-owner"), Role: SOParticipantRole_SOParticipantRole_READER})
	if err := signSOConfigChange(forged, c.keys[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyConfigChange(mockSharedObjectID, head, forged); err == nil {
		t.Fatal("forged merge config verified")
	}
}

// TestConfigMergeNestedSuffix reads and verifies, from a head absorbed two
// merges deep, the suffix to the final merge.
func TestConfigMergeNestedSuffix(t *testing.T) {
	// Three owners extend genesis apart: one, two and four records.
	c := newMergeChain(t)
	a1, aHead := c.addReader(c.root, 0, "reader-a1")
	b1, b1Config := c.addReader(c.root, 1, "reader-b1")
	b2, bHead := c.addReader(b1Config, 1, "reader-b2")
	cHead := c.root
	var cBranch []*SOConfigChange
	for _, reader := range []string{"reader-c1", "reader-c2", "reader-c3", "reader-c4"} {
		var entry *SOConfigChange
		entry, cHead = c.addReader(cHead, 2, reader)
		cBranch = append(cBranch, entry)
	}

	// Owner 1 merges A's branch, then owner 2 merges that branch onto C's head.
	inner, _ := c.merge(bHead, c.genesis, []*SOConfigChange{a1}, 1)
	outer, final := c.merge(cHead, c.genesis, []*SOConfigChange{b1, b2, inner}, 2)
	for _, reader := range []string{"reader-a1", "reader-b1", "reader-b2", "reader-c1", "reader-c4"} {
		if !hasPeer(final, c.reader(reader)) {
			t.Fatalf("final merge lacks %s", reader)
		}
	}

	// A replica holding only A's head reads back to the fork and verifies.
	aHash := aHead.GetConfigChainHash()
	suffix, err := ReadConfigSuffix(t.Context(), aHash, final.GetConfigChainHash(), c.read)
	if err != nil {
		t.Fatal(err)
	}
	want := append(slices.Clone(cBranch), outer)
	if len(suffix) != len(want) || !bytes.Equal(suffix[0].GetPreviousHash(), c.root.GetConfigChainHash()) {
		t.Fatalf("suffix has %d records from the wrong fork", len(suffix))
	}
	if err := VerifyConfigChainSuffix(mockSharedObjectID, aHead, final, suffix); err != nil {
		t.Fatal(err)
	}

	// Every absorbed record is retained, and a head no merge absorbs fails.
	if got := len(AbsorbedConfigChanges([]*SOConfigChange{outer})); got != 4 {
		t.Fatalf("absorbed %d records, want 4", got)
	}
	if _, err := ConfigSuffixBase(cHead, []*SOConfigChange{b2}); !errors.Is(err, ErrConfigChainHeadMismatch) {
		t.Fatalf("unabsorbed head base error = %v", err)
	}
}
