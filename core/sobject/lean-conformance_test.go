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

// leanOracleEnv names the environment variable holding the path of the Lean
// conformance oracle built from repos/spacewave/lean.
const leanOracleEnv = "SPACEWAVE_LEAN_ORACLE"

// leanScenarioSteps is the number of changes each generated scenario proposes.
const leanScenarioSteps = 30

// TestLeanConfigChainConformance checks that VerifyConfigChange,
// VerifyConfigChain and VerifyConfigChainSuffix make the same decisions as
// the Lean model in lean/Spacewave/SObject/ConfigChain.lean.
func TestLeanConfigChainConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 6)

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
	// field names an additional result field to compare as abstract JSON.
	field string
	// value is the projected Go result for field.
	value []byte
}

// checkLeanCases runs every case through one oracle process and compares its
// answers with the Go decisions.
func checkLeanCases(t *testing.T, oracle string, cases []leanCase) {
	t.Helper()

	// Send every request in one batch; the oracle answers line for line.
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
	// ids names each generated signer.
	ids []string
	// privs contains the matching signing keys.
	privs []crypto.PrivKey
	// arena owns projected JSON until the scenario finishes.
	arena fastjson.Arena
	// cases records Go's decisions for the oracle.
	cases []leanCase
}

// runConfigChainScenario generates the cases for one seed.
func runConfigChainScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
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

// run proposes a genesis and then leanScenarioSteps changes, advancing the
// held configuration through each change Go accepts.
func (s *configChainScenario) run() {
	// Start from a genesis entry; most scenarios produce a valid one.
	genesis := s.genesis()
	chain := []*SOConfigChange{genesis}
	if !s.checkChain(chain) {
		return
	}
	cur := configWithAppliedConfigChainHead(genesis.GetConfig(), 0, s.hash(genesis))
	heads := []*SharedObjectConfig{cur}

	// Propose changes, checking each alone, within the chain, and as a suffix.
	for i := range leanScenarioSteps {
		entry := s.change(cur)
		next, err := VerifyConfigChange(cur, entry)
		s.checkChange(i, cur, entry, next, err)
		s.checkChain(append(slices.Clone(chain), entry))
		if err == nil {
			chain = append(chain, entry)
			heads = append(heads, next)
			cur = next
		}
		s.checkSuffix(heads, chain)
	}
}

// checkChange records one VerifyConfigChange decision.
func (s *configChainScenario) checkChange(
	step int,
	cur *SharedObjectConfig,
	entry *SOConfigChange,
	next *SharedObjectConfig,
	err error,
) {
	req := s.arena.NewObject()
	req.Set("op", s.arena.NewString("verifyChange"))
	req.Set("current", projectLeanConfig(cur).json(&s.arena))
	req.Set("entry", s.entryJSON(entry))

	c := leanCase{name: "verifyChange step " + strconv.Itoa(step), request: req.MarshalTo(nil), ok: err == nil}
	if err == nil {
		projected := projectLeanConfig(next)
		c.config = &projected
	}
	s.cases = append(s.cases, c)
}

// checkChain records one VerifyConfigChain decision and returns it.
func (s *configChainScenario) checkChain(chain []*SOConfigChange) bool {
	req := s.arena.NewObject()
	req.Set("op", s.arena.NewString("verifyChain"))
	req.Set("entries", s.entriesJSON(chain))

	ok := VerifyConfigChain(chain) == nil
	name := "verifyChain length " + strconv.Itoa(len(chain))
	s.cases = append(s.cases, leanCase{name: name, request: req.MarshalTo(nil), ok: ok})
	return ok
}

// checkSuffix records one VerifyConfigChainSuffix decision from a random held
// checkpoint to the current head or a perturbed candidate.
func (s *configChainScenario) checkSuffix(heads []*SharedObjectConfig, chain []*SOConfigChange) {
	// Pick a checkpoint and the entries after it, sometimes dropping the last.
	from := s.rng.IntN(len(heads))
	entries := chain[from+1:]
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

	req := s.arena.NewObject()
	req.Set("op", s.arena.NewString("verifySuffix"))
	req.Set("current", projectLeanConfig(heads[from]).json(&s.arena))
	req.Set("candidate", projectLeanConfig(candidate).json(&s.arena))
	req.Set("entries", s.entriesJSON(entries))

	ok := VerifyConfigChainSuffix(heads[from], candidate, entries) == nil
	name := "verifySuffix from " + strconv.Itoa(from)
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
	entry := &SOConfigChange{Config: cfg, ChangeType: SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS}
	switch s.rng.IntN(12) {
	case 0:
		entry.ConfigSeqno = 1
	case 1:
		entry.PreviousHash = []byte{1}
	case 2:
		entry.ChangeType = SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT
	case 3:
		entry.Config = nil
	}

	// Sign as the owner, a random peer, or not at all, as cloud bootstrap does.
	switch s.rng.IntN(4) {
	case 0:
	case 1:
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
		SOConfigChangeType(s.rng.IntN(9)),
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
		ConfigSeqno:  cur.GetConfigChainSeqno() + 1,
		Config:       next,
		ChangeType:   kind,
		PreviousHash: cur.GetConfigChainHash(),
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
	}

	// Sign it, sometimes omitting or invalidating the signature.
	switch s.rng.IntN(16) {
	case 0:
	case 1:
		s.sign(entry, signer)
		entry.ConfigSeqno++
	default:
		s.sign(entry, signer)
	}
	return entry
}

// mutate applies one or two random membership and metadata edits to cfg.
func (s *configChainScenario) mutate(cfg *SharedObjectConfig) {
	for range 1 + s.rng.IntN(2) {
		parts := cfg.GetParticipants()
		switch op := s.rng.IntN(8); {
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
		default:
			// Change metadata a self-enrollment must preserve.
			cfg.ConsensusMode = SOConsensusMode(s.rng.IntN(2))
		}
	}
}

// enroll adds signer as a participant bound to an existing entity, as a
// well-formed self-enrollment does, sometimes with an escalated role.
func (s *configChainScenario) enroll(cfg *SharedObjectConfig, signer int) {
	entity := "e0"
	role := SOParticipantRole_SOParticipantRole_READER
	if parts := cfg.GetParticipants(); len(parts) != 0 {
		p := parts[s.rng.IntN(len(parts))]
		entity, role = p.GetEntityId(), p.GetRole()
	}
	if s.rng.IntN(4) == 0 {
		role = s.role()
	}
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
	id := s.ids[s.rng.IntN(len(s.ids))]
	if s.rng.IntN(4) != 0 {
		id = s.ids[s.peer(cfg, func(p *SOParticipantConfig) bool { return p == nil })]
	}
	if s.rng.IntN(16) == 0 {
		id = "not-a-peer-id"
	}
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
	return SOParticipantRole(1 + s.rng.IntN(4))
}

// sign signs entry with the key of peer index signer.
func (s *configChainScenario) sign(entry *SOConfigChange, signer int) {
	signConfigChange(s.t, entry, s.privs[signer])
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

// entryJSON projects a config change entry into the Lean Entry structure. The
// signature becomes its signer and whether it verifies over the entry.
func (s *configChainScenario) entryJSON(entry *SOConfigChange) *fastjson.Value {
	a := &s.arena
	v := a.NewObject()
	v.Set("seqno", a.NewNumberString(strconv.FormatUint(entry.GetConfigSeqno(), 10)))
	v.Set("config", a.NewNull())
	if entry.GetConfig() != nil {
		v.Set("config", projectLeanConfig(entry.GetConfig()).json(a))
	}
	v.Set("sig", a.NewNull())
	if entry.GetSignature() != nil {
		signer, valid := projectLeanSignature(entry)
		sig := a.NewObject()
		sig.Set("signer", a.NewString(signer))
		sig.Set("valid", leanBool(a, valid))
		v.Set("sig", sig)
	}
	v.Set("prev", a.NewString(hex.EncodeToString(entry.GetPreviousHash())))
	v.Set("kind", a.NewNumberInt(int(entry.GetChangeType())))
	v.Set("hash", a.NewString(hex.EncodeToString(s.hash(entry))))
	return v
}

// projectLeanSignature returns the peer that produced the entry signature and
// whether it verifies over the entry without its signature field.
func projectLeanSignature(entry *SOConfigChange) (string, bool) {
	sig := entry.GetSignature()
	pub, err := sig.ParsePubKey()
	if err != nil || pub == nil {
		return "", false
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return "", false
	}

	// Verify over the same bytes verifyConfigChangeSignature checks.
	clone := entry.CloneVT()
	clone.Signature = nil
	data, err := clone.MarshalVT()
	if err != nil {
		return "", false
	}
	valid, err := sig.VerifyWithPublic("sobject config change", pub, data)
	return id.String(), err == nil && valid
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
	// mode preserves the consensus enum code.
	mode int32
	// hash is the hexadecimal configuration chain head hash.
	hash string
	// seqno is the chain head's uint64 sequence number.
	seqno uint64
}

// projectLeanConfig projects a Go configuration. A peer ID that does not parse
// becomes the empty string, which the model treats as malformed.
func projectLeanConfig(cfg *SharedObjectConfig) leanConfig {
	c := leanConfig{
		mode:  int32(cfg.GetConsensusMode()),
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
		mode:  int32(v.GetInt("mode")),
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
	return c.mode == o.mode && c.hash == o.hash && c.seqno == o.seqno &&
		slices.Equal(c.participants, o.participants)
}

// json renders the configuration as a Lean Config.
func (c leanConfig) json(a *fastjson.Arena) *fastjson.Value {
	parts := a.NewArray()
	for i, p := range c.participants {
		v := a.NewObject()
		v.Set("peer", a.NewString(p.peer))
		v.Set("role", a.NewNumberInt(int(p.role)))
		v.Set("entity", a.NewString(p.entity))
		parts.SetArrayItem(i, v)
	}

	v := a.NewObject()
	v.Set("participants", parts)
	v.Set("mode", a.NewNumberInt(int(c.mode)))
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
