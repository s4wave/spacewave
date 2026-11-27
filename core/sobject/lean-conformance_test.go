package sobject

import (
	"bytes"
	"encoding/hex"
	"math/rand/v2"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// leanOracleEnv holds the environment variable name for the path of the Lean
// conformance oracle built from repos/spacewave/lean.
const leanOracleEnv = "SPACEWAVE_LEAN_ORACLE"

// leanScenarioSteps is the number of changes each generated scenario proposes.
const leanScenarioSteps = 30

// TestLeanConfigChainConformance checks that VerifyConfigChange,
// VerifyConfigChain and VerifyConfigChainSuffix make the same decisions as
// the Lean model in lean/Spacewave/SObject/ConfigChain.lean.
func TestLeanConfigChainConformance(t *testing.T) {
	// Share one oracle and peer set across the seeds.
	oracle := leanOracle(t)
	peers := createMockPeers(t, 6)

	// Generate every seed's cases and check them in one oracle run.
	var cases []leanCase
	for seed := range uint64(400) {
		cases = append(cases, runConfigChainScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanConfigChain searches scenario seeds for a disagreement between Go
// and the Lean configuration chain model.
func FuzzLeanConfigChain(f *testing.F) {
	f.Add(uint64(0))
	f.Fuzz(func(t *testing.T, seed uint64) {
		oracle := leanOracle(t)
		checkLeanCases(t, oracle, runConfigChainScenario(t, createMockPeers(t, 6), seed))
	})
}

// leanOracle returns the oracle path, skipping the test when it is unset.
func leanOracle(t *testing.T) string {
	t.Helper()
	oracle := os.Getenv(leanOracleEnv)
	if oracle == "" {
		t.Skipf("%s is not set; build it with bun run lean", leanOracleEnv)
	}
	return oracle
}

// leanCase is one oracle request and the decision Go made for it.
type leanCase struct {
	// name describes the case in failure messages.
	name string
	// request is the oracle JSON request line.
	request []byte
	// ok is whether Go accepted the input.
	ok bool
	// config is Go's resulting configuration for verifyChange, when accepted.
	config *leanConfig
	// field selects an additional result field to compare as abstract JSON.
	field string
	// value is the projected Go result for field.
	value []byte
}

// checkLeanCases runs every case through one oracle process and compares its
// answers with the Go decisions.
func checkLeanCases(t *testing.T, oracle string, cases []leanCase) {
	t.Helper()

	// Send every request in one batch. The oracle answers line for line.
	var input bytes.Buffer
	for _, c := range cases {
		input.Write(c.request)
		input.WriteByte('\n')
	}
	cmd := exec.CommandContext(t.Context(), oracle)
	cmd.Stdin = &input
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run lean oracle: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(cases) {
		t.Fatalf("lean oracle answered %d of %d requests", len(lines), len(cases))
	}

	// Compare acceptance, and the resulting configuration where Go returns one.
	var p fastjson.Parser
	accepted := make(map[string]int)
	for i, c := range cases {
		if c.ok {
			accepted[strings.Fields(c.name)[0]]++
		}
		v, err := p.Parse(lines[i])
		if err != nil {
			t.Fatalf("%s: parse oracle answer %q: %v", c.name, lines[i], err)
		}
		if msg := v.GetStringBytes("error"); msg != nil {
			t.Fatalf("%s: oracle error %s\nrequest: %s", c.name, msg, c.request)
		}
		if ok := v.GetBool("ok"); ok != c.ok {
			t.Fatalf("%s: go ok=%v lean ok=%v\nrequest: %s", c.name, c.ok, ok, c.request)
		}
		if c.field != "" {
			var expectedParser fastjson.Parser
			expected, err := expectedParser.ParseBytes(c.value)
			if err != nil {
				t.Fatal(err)
			}
			if !equalLeanJSON(v.Get(c.field), expected) {
				t.Fatalf("%s: %s differs\ngo: %s\nlean: %s\nrequest: %s", c.name, c.field, c.value, v.Get(c.field), c.request)
			}
		}
		if c.config == nil {
			continue
		}
		if got := parseLeanConfig(v.Get("config")); !got.equal(*c.config) {
			t.Fatalf("%s: go config %+v lean config %+v\nrequest: %s", c.name, *c.config, got, c.request)
		}
	}
	t.Logf("%d cases agree; accepted by op: %v", len(cases), accepted)
}

// configChainScenario walks one random configuration chain with real keys,
// recording each decision Go makes as a leanCase.
type configChainScenario struct {
	// t reports generation failures.
	t *testing.T
	// rng selects repeatable changes from the scenario seed.
	rng *rand.Rand
	// ids holds the ID of each generated signer.
	ids []string
	// privs contains the matching signing keys.
	privs []crypto.PrivKey
	// arena owns projected JSON until the scenario finishes.
	arena fastjson.Arena
	// cases records Go's decisions for the oracle.
	cases []leanCase
	// branch holds the branch the last proposed merge absorbs.
	branch absorbedBranch
	// absorbed lists the branches of merges the main chain accepted.
	absorbed []absorbedBranch
}

// absorbedBranch is a branch a merge absorbs: the heads its records yield and
// the main chain index of its fork.
type absorbedBranch struct {
	// heads are the configs after each branch record.
	heads []*SharedObjectConfig
	// forkAt indexes the fork in the main chain.
	forkAt int
}

// runConfigChainScenario generates the cases for one seed.
func runConfigChainScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	// Load each peer's signing key, then walk the scenario.
	t.Helper()
	s := &configChainScenario{t: t, rng: rand.New(rand.NewPCG(seed, 0x5eed))}
	for _, p := range peers {
		priv, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		s.ids = append(s.ids, p.GetPeerID().String())
		s.privs = append(s.privs, priv)
	}
	s.run()
	return s.cases
}

// run proposes a genesis and then leanScenarioSteps records, advancing the
// held configuration through each record Go accepts.
func (s *configChainScenario) run() {
	// Start from a genesis entry. Most scenarios produce a valid one.
	genesis := s.genesis()
	chain := []*SOConfigChange{genesis}
	if !s.checkChain(chain) {
		return
	}
	heads := []*SharedObjectConfig{configWithAppliedConfigChainHead(genesis.GetConfig(), 0, s.hash(genesis))}

	// Propose records, checking each alone, within the chain, and as a suffix.
	for i := range leanScenarioSteps {
		entry := s.propose(chain, heads, 0)
		var ok bool
		chain, heads, ok = s.extend("step "+strconv.Itoa(i), chain, heads, entry)
		if ok && entry.GetMerge() != nil {
			s.absorbed = append(s.absorbed, s.branch)
		}
		s.checkSuffix(heads, chain)
	}
}

// propose returns a record on the last head: usually a change, sometimes a
// merge absorbing a branch grown from an earlier head. depth bounds nesting.
func (s *configChainScenario) propose(chain []*SOConfigChange, heads []*SharedObjectConfig, depth int) *SOConfigChange {
	if depth < 2 && len(chain) > 1 && s.rng.IntN(4) == 0 {
		return s.merge(chain, heads, depth)
	}
	return s.change(heads[len(heads)-1])
}

// extend checks entry alone and within the chain, and returns the chain and
// heads extended by it when Go accepts it.
func (s *configChainScenario) extend(
	name string,
	chain []*SOConfigChange,
	heads []*SharedObjectConfig,
	entry *SOConfigChange,
) ([]*SOConfigChange, []*SharedObjectConfig, bool) {
	// Record Go's decisions on the entry, and keep it when accepted.
	cur := heads[len(heads)-1]
	next, err := VerifyConfigChange(mockSharedObjectID, cur, entry)
	s.checkChange(name, cur, entry, next, err)
	s.checkChain(append(slices.Clone(chain), entry))
	if err != nil {
		return chain, heads, false
	}
	return append(chain, entry), append(heads, next), true
}

// merge proposes a merge on the last head absorbing a branch of one to three
// records grown from an earlier head, sometimes with a rejected record, a
// config the branch does not yield, or a wrong kind, fork or signer.
func (s *configChainScenario) merge(chain []*SOConfigChange, heads []*SharedObjectConfig, depth int) *SOConfigChange {
	// Grow the branch from the fork, retrying rejected records and sometimes
	// keeping one.
	from := s.rng.IntN(len(heads) - 1)
	branchChain, branchHeads := slices.Clone(chain[:from+1]), slices.Clone(heads[:from+1])
	name := "branch at depth " + strconv.Itoa(depth+1)
	want := from + 2 + s.rng.IntN(3)
	for attempt := 0; attempt < 8 && len(branchChain) < want; attempt++ {
		entry := s.propose(branchChain, branchHeads, depth+1)
		var ok bool
		branchChain, branchHeads, ok = s.extend(name, branchChain, branchHeads, entry)
		if !ok && s.rng.IntN(8) == 0 {
			branchChain = append(branchChain, entry)
			break
		}
	}
	cur := heads[len(heads)-1]
	if len(branchChain) == from+1 {
		return s.change(cur)
	}
	s.branch = absorbedBranch{heads: branchHeads[from+1:], forkAt: from}

	// Carry the config the branch yields, sometimes perturbed.
	merge := &SOConfigMerge{Fork: chain[from], Branch: branchChain[from+1:]}
	next, err := mergeConfigBranch(mockSharedObjectID, cur, merge)
	if err != nil || s.rng.IntN(8) == 0 {
		next = cur.CloneVT()
		s.mutate(next)
	}
	entry := newSOConfigChange(mockSharedObjectID, cur, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_MERGE)
	entry.Merge = merge
	switch s.rng.IntN(16) {
	case 0:
		entry.ChangeType = SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT
	case 1:
		entry.Merge = nil
	case 2:
		merge.Fork = chain[s.rng.IntN(len(chain))]
	}

	// Sign as an owner of the head, sometimes as any peer.
	signer := s.rng.IntN(len(s.ids))
	if s.rng.IntN(4) != 0 {
		signer = s.peer(cur, func(p *SOParticipantConfig) bool { return p != nil && IsOwner(p.GetRole()) })
	}
	s.sign(entry, signer)
	return entry
}

// checkChange records one VerifyConfigChange decision.
func (s *configChainScenario) checkChange(
	name string,
	cur *SharedObjectConfig,
	entry *SOConfigChange,
	next *SharedObjectConfig,
	err error,
) {
	// Record the request.
	req := s.arena.NewObject()
	req.Set("op", s.arena.NewString("verifyChange"))
	req.Set("object", s.arena.NewString(mockSharedObjectID))
	req.Set("current", projectLeanConfig(cur).json(&s.arena))
	req.Set("entry", s.entryJSON(entry))

	// Record the Go decision and, when accepted, the next configuration.
	c := leanCase{name: "verifyChange " + name, request: req.MarshalTo(nil), ok: err == nil}
	if err == nil {
		projected := projectLeanConfig(next)
		c.config = &projected
	}
	s.cases = append(s.cases, c)
}

// checkChain records one VerifyConfigChain decision and returns it.
func (s *configChainScenario) checkChain(chain []*SOConfigChange) bool {
	// Record the request.
	req := s.arena.NewObject()
	req.Set("op", s.arena.NewString("verifyChain"))
	req.Set("object", s.arena.NewString(mockSharedObjectID))
	req.Set("entries", s.entriesJSON(chain))

	// Record the Go decision as the expected result.
	ok := VerifyConfigChain(mockSharedObjectID, chain) == nil
	name := "verifyChain length " + strconv.Itoa(len(chain))
	s.cases = append(s.cases, leanCase{name: name, request: req.MarshalTo(nil), ok: ok})
	return ok
}

// checkSuffix records one VerifyConfigChainSuffix decision from a random held
// checkpoint, on the chain or on a branch a merge absorbed, to the current head
// or a perturbed candidate.
func (s *configChainScenario) checkSuffix(heads []*SharedObjectConfig, chain []*SOConfigChange) {
	// Pick a checkpoint and the entries after it, sometimes dropping the last.
	from := s.rng.IntN(len(heads))
	held, entries := heads[from], chain[from+1:]
	if len(s.absorbed) != 0 && s.rng.IntN(3) == 0 {
		branch := s.absorbed[s.rng.IntN(len(s.absorbed))]
		held, entries = branch.heads[s.rng.IntN(len(branch.heads))], chain[branch.forkAt+1:]
	}
	if len(entries) != 0 && s.rng.IntN(8) == 0 {
		entries = entries[:len(entries)-1]
	}

	// Offer the current head, reordered, or an earlier head as the candidate.
	candidate := heads[len(heads)-1].CloneVT()
	switch s.rng.IntN(6) {
	case 0:
		s.rng.Shuffle(len(candidate.Participants), func(i, j int) {
			candidate.Participants[i], candidate.Participants[j] = candidate.Participants[j], candidate.Participants[i]
		})
	case 1:
		candidate = heads[s.rng.IntN(len(heads))].CloneVT()
	}

	// Record the request.
	req := s.arena.NewObject()
	req.Set("op", s.arena.NewString("verifySuffix"))
	req.Set("object", s.arena.NewString(mockSharedObjectID))
	req.Set("current", projectLeanConfig(held).json(&s.arena))
	req.Set("candidate", projectLeanConfig(candidate).json(&s.arena))
	req.Set("entries", s.entriesJSON(entries))

	// Record the Go decision as the expected result.
	ok := VerifyConfigChainSuffix(mockSharedObjectID, held, candidate, entries) == nil
	name := "verifySuffix from seqno " + strconv.FormatUint(held.GetConfigChainSeqno(), 10)
	s.cases = append(s.cases, leanCase{name: name, request: req.MarshalTo(nil), ok: ok})
}

// genesis builds a genesis entry, occasionally malformed or signed by a
// non-owner.
func (s *configChainScenario) genesis() *SOConfigChange {
	// Seed the configuration with an owner so most chains get past genesis.
	cfg := &SharedObjectConfig{Participants: []*SOParticipantConfig{{
		PeerId:   s.ids[0],
		Role:     SOParticipantRole_SOParticipantRole_OWNER,
		EntityId: "e0",
	}}}
	if s.rng.IntN(2) == 0 {
		s.mutate(cfg)
	}

	// Perturb the genesis fields that VerifyConfigChain checks.
	entry := &SOConfigChange{
		SharedObjectId: mockSharedObjectID,
		Config:         cfg,
		ChangeType:     SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
	}
	switch s.rng.IntN(12) {
	case 0:
		entry.ConfigSeqno = 1
	case 1:
		entry.PreviousHash = []byte{1}
	case 2:
		entry.ChangeType = SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT
	case 3:
		entry.Config = nil
	case 4:
		entry.SharedObjectId = "other object"
	}

	// Sign as the owner, a random peer, both, or not at all.
	switch s.rng.IntN(6) {
	case 0:
	case 1:
		s.sign(entry, s.rng.IntN(len(s.ids)))
	case 2:
		s.sign(entry, 0)
		s.sign(entry, s.rng.IntN(len(s.ids)))
	default:
		s.sign(entry, 0)
	}
	return entry
}

// change proposes a change to cur: usually well linked and signed by a
// participant, with a perturbed configuration.
func (s *configChainScenario) change(cur *SharedObjectConfig) *SOConfigChange {
	// Choose a change type, favoring membership changes and self-enrollment.
	kinds := []SOConfigChangeType{
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER,
		SOConfigChangeType(s.rng.IntN(13)),
	}
	kind := kinds[s.rng.IntN(len(kinds))]

	// Choose the signer: usually an owner, or a newcomer for self-enrollment.
	enroll := kind == SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER
	signer := s.rng.IntN(len(s.ids))
	if s.rng.IntN(4) != 0 {
		signer = s.peer(cur, func(p *SOParticipantConfig) bool {
			return p == nil && enroll || p != nil && !enroll && IsOwner(p.GetRole())
		})
	}

	// Build the proposed configuration for the change type.
	next := cur.CloneVT()
	switch {
	case enroll && s.rng.IntN(4) != 0:
		s.enroll(next, signer)
	default:
		s.mutate(next)
	}

	// Link the entry to the held head, sometimes wrongly.
	entry := &SOConfigChange{
		SharedObjectId: mockSharedObjectID,
		ConfigSeqno:    cur.GetConfigChainSeqno() + 1,
		Config:         next,
		ChangeType:     kind,
		PreviousHash:   cur.GetConfigChainHash(),
	}
	switch s.rng.IntN(16) {
	case 0:
		entry.ConfigSeqno--
	case 1:
		entry.ConfigSeqno++
	case 2:
		entry.PreviousHash = bytes.Repeat([]byte{7}, 32)
	case 3:
		entry.Config = nil
	case 4:
		entry.SharedObjectId = "other object"
	}

	// Sign it, sometimes omitting, invalidating or repeating the signature, or
	// adding an owner's.
	switch s.rng.IntN(16) {
	case 0:
	case 1:
		s.sign(entry, signer)
		entry.ConfigSeqno++
	case 2:
		s.sign(entry, signer)
		s.sign(entry, signer)
	case 3, 4:
		s.sign(entry, signer)
		s.sign(entry, s.peer(cur, func(p *SOParticipantConfig) bool {
			return p != nil && IsOwner(p.GetRole())
		}))
	default:
		s.sign(entry, signer)
	}
	return entry
}

// mutate applies one or two random membership edits to cfg.
func (s *configChainScenario) mutate(cfg *SharedObjectConfig) {
	for range 1 + s.rng.IntN(2) {
		parts := cfg.GetParticipants()
		switch op := s.rng.IntN(7); {
		case op < 3:
			// Add a participant, possibly already present or malformed.
			cfg.Participants = append(parts, s.participant(cfg))
		case op < 5 && len(parts) != 0:
			// Remove a participant, possibly the last owner.
			i := s.rng.IntN(len(parts))
			cfg.Participants = slices.Delete(parts, i, i+1)
		case op < 7 && len(parts) != 0:
			// Change a participant's role, including to values outside the enum.
			parts[s.rng.IntN(len(parts))].Role = s.role()
		}
	}
}

// enroll adds signer as a participant bound to an existing entity, as a
// well-formed self-enrollment does, sometimes with an escalated role.
func (s *configChainScenario) enroll(cfg *SharedObjectConfig, signer int) {
	// Bind to a random existing participant's entity and role.
	entity := "e0"
	role := SOParticipantRole_SOParticipantRole_READER
	if parts := cfg.GetParticipants(); len(parts) != 0 {
		p := parts[s.rng.IntN(len(parts))]
		entity, role = p.GetEntityId(), p.GetRole()
	}
	if s.rng.IntN(4) == 0 {
		role = s.role()
	}

	// Append the signer as the enrolled participant.
	cfg.Participants = append(cfg.Participants, &SOParticipantConfig{
		PeerId:   s.ids[signer],
		Role:     role,
		EntityId: entity,
	})
}

// peer returns the index of a random peer whose participant entry in cfg, nil
// when absent, satisfies match, or of any peer when none does.
func (s *configChainScenario) peer(cfg *SharedObjectConfig, match func(*SOParticipantConfig) bool) int {
	var found []int
	for i, id := range s.ids {
		idx := slices.IndexFunc(cfg.GetParticipants(), func(p *SOParticipantConfig) bool {
			return p.GetPeerId() == id
		})
		var p *SOParticipantConfig
		if idx >= 0 {
			p = cfg.GetParticipants()[idx]
		}
		if match(p) {
			found = append(found, i)
		}
	}
	if len(found) == 0 {
		return s.rng.IntN(len(s.ids))
	}
	return found[s.rng.IntN(len(found))]
}

// participant returns a random participant, usually a peer absent from cfg.
func (s *configChainScenario) participant(cfg *SharedObjectConfig) *SOParticipantConfig {
	// Pick the peer ID, usually absent from cfg, sometimes malformed.
	id := s.ids[s.rng.IntN(len(s.ids))]
	if s.rng.IntN(4) != 0 {
		id = s.ids[s.peer(cfg, func(p *SOParticipantConfig) bool { return p == nil })]
	}
	if s.rng.IntN(16) == 0 {
		id = "not-a-peer-id"
	}

	// Pick the entity, sometimes empty, and a random role.
	entity := "e" + strconv.Itoa(s.rng.IntN(3))
	if s.rng.IntN(8) == 0 {
		entity = ""
	}
	return &SOParticipantConfig{PeerId: id, Role: s.role(), EntityId: entity}
}

// role returns a random role code: usually a valid role, sometimes a code
// outside the valid range.
func (s *configChainScenario) role() SOParticipantRole {
	if s.rng.IntN(8) == 0 {
		return []SOParticipantRole{-1, 0, 5}[s.rng.IntN(3)]
	}
	return SOParticipantRole(1 + s.rng.IntN(3))
}

// sign adds a signature over entry by the key of peer index signer.
func (s *configChainScenario) sign(entry *SOConfigChange, signer int) {
	if err := signSOConfigChange(entry, s.privs[signer]); err != nil {
		s.t.Fatal(err)
	}
}

// hash returns the chain hash of entry.
func (s *configChainScenario) hash(entry *SOConfigChange) []byte {
	h, err := HashSOConfigChange(entry)
	if err != nil {
		s.t.Fatal(err)
	}
	return h
}

// entriesJSON projects a list of config change entries.
func (s *configChainScenario) entriesJSON(entries []*SOConfigChange) *fastjson.Value {
	arr := s.arena.NewArray()
	for i, entry := range entries {
		arr.SetArrayItem(i, s.entryJSON(entry))
	}
	return arr
}

// entryJSON projects a config change entry into the Lean Entry structure.
// Each signature becomes its signer and whether it verifies over the entry.
func (s *configChainScenario) entryJSON(entry *SOConfigChange) *fastjson.Value {
	// Project the binding, sequence and config.
	a := &s.arena
	v := a.NewObject()
	v.Set("object", a.NewString(entry.GetSharedObjectId()))
	v.Set("seqno", a.NewNumberString(strconv.FormatUint(entry.GetConfigSeqno(), 10)))
	v.Set("config", a.NewNull())
	if entry.GetConfig() != nil {
		v.Set("config", projectLeanConfig(entry.GetConfig()).json(a))
	}

	// Project each signature over the bytes verifyConfigChangeAuthority checks.
	data, err := configChangeSignedBody(entry)
	if err != nil {
		s.t.Fatal(err)
	}
	sigs := a.NewArray()
	for i, sig := range entry.GetSignatures() {
		sigs.SetArrayItem(i, projectLeanSig(a, sig, data, func(string) string {
			return soConfigChangeSignatureContext
		}))
	}
	v.Set("sigs", sigs)

	// Project the links and identity.
	v.Set("prev", a.NewString(hex.EncodeToString(entry.GetPreviousHash())))
	v.Set("kind", a.NewNumberInt(int(entry.GetChangeType())))
	v.Set("hash", a.NewString(hex.EncodeToString(s.hash(entry))))

	// Project the merge and its branch.
	v.Set("merge", a.NewNull())
	if merge := entry.GetMerge(); merge != nil {
		m := a.NewObject()
		m.Set("fork", s.entryJSON(merge.GetFork()))
		m.Set("branch", s.entriesJSON(merge.GetBranch()))
		v.Set("merge", m)
	}
	return v
}

// leanParticipant mirrors the Lean Participant structure.
type leanParticipant struct {
	// peer is the participant's serialized peer ID.
	peer string
	// role preserves the protobuf enum code, including unknown values.
	role int32
	// entity is the owning entity's ID.
	entity string
}

// leanConfig mirrors the Lean Config structure.
type leanConfig struct {
	// participants preserves participant order and duplicates.
	participants []leanParticipant
	// hash is the hexadecimal configuration chain head hash.
	hash string
	// seqno is the chain head's uint64 sequence number.
	seqno uint64
}

// projectLeanConfig projects a Go configuration. A peer ID that does not parse
// becomes the empty string, which the model treats as malformed.
func projectLeanConfig(cfg *SharedObjectConfig) leanConfig {
	c := leanConfig{
		hash:  hex.EncodeToString(cfg.GetConfigChainHash()),
		seqno: cfg.GetConfigChainSeqno(),
	}
	for _, p := range cfg.GetParticipants() {
		id := p.GetPeerId()
		if _, err := p.ParsePeerID(); err != nil {
			id = ""
		}
		c.participants = append(c.participants, leanParticipant{peer: id, role: int32(p.GetRole()), entity: p.GetEntityId()})
	}
	return c
}

// parseLeanConfig reads a Lean Config from an oracle answer.
func parseLeanConfig(v *fastjson.Value) leanConfig {
	c := leanConfig{
		hash:  string(v.GetStringBytes("hash")),
		seqno: v.GetUint64("seqno"),
	}
	for _, p := range v.GetArray("participants") {
		c.participants = append(c.participants, leanParticipant{
			peer:   string(p.GetStringBytes("peer")),
			role:   int32(p.GetInt("role")),
			entity: string(p.GetStringBytes("entity")),
		})
	}
	return c
}

// equal reports whether two projected configurations are identical, including
// participant order.
func (c leanConfig) equal(o leanConfig) bool {
	return c.hash == o.hash && c.seqno == o.seqno &&
		slices.Equal(c.participants, o.participants)
}

// json renders the configuration as a Lean Config.
func (c leanConfig) json(a *fastjson.Arena) *fastjson.Value {
	// Render the participants in order.
	parts := a.NewArray()
	for i, p := range c.participants {
		v := a.NewObject()
		v.Set("peer", a.NewString(p.peer))
		v.Set("role", a.NewNumberInt(int(p.role)))
		v.Set("entity", a.NewString(p.entity))
		parts.SetArrayItem(i, v)
	}

	// Render the config with its chain head.
	v := a.NewObject()
	v.Set("participants", parts)
	v.Set("hash", a.NewString(c.hash))
	v.Set("seqno", a.NewNumberString(strconv.FormatUint(c.seqno, 10)))
	return v
}

// leanBool returns the JSON boolean b.
func leanBool(a *fastjson.Arena, b bool) *fastjson.Value {
	if b {
		return a.NewTrue()
	}
	return a.NewFalse()
}

// equalLeanJSON compares JSON structurally without depending on object key order.
func equalLeanJSON(a, b *fastjson.Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case fastjson.TypeObject:
		x, y := a.GetObject(), b.GetObject()
		if x.Len() != y.Len() {
			return false
		}
		equal := true
		x.Visit(func(key []byte, value *fastjson.Value) {
			equal = equal && equalLeanJSON(value, y.Get(string(key)))
		})
		return equal
	case fastjson.TypeArray:
		return slices.EqualFunc(a.GetArray(), b.GetArray(), equalLeanJSON)
	default:
		return bytes.Equal(a.MarshalTo(nil), b.MarshalTo(nil))
	}
}
