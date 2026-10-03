package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// operationLogVectorsPath holds the cases shared with the TypeScript core.
// Regenerate it with SPACEWAVE_WRITE_VECTORS=1.
var operationLogVectorsPath = filepath.Join("testdata", "operation-log-vectors.json")

// vectorObjectID is the shared object every vector is verified under.
const vectorObjectID = "vector-object"

// operationLogVectors are the operation and control-record cases shared by the
// Go and TypeScript cores.
type operationLogVectors struct {
	SharedObjectID string
	Operations     []operationVector
	OperationSets  []operationSetVector
	ConfigChains   []configChainVector
	ConfigChanges  []configChangeVector
	Checkpoints    []checkpointVector
}

// operationVector is one operation and its verification under the object.
type operationVector struct {
	Name      string
	Operation []byte
	// Hash is the operation hash in hex, empty when verification fails.
	Hash string
}

// operationSetVector is the set built from named operations, in any order.
type operationSetVector struct {
	Name          string
	Operations    []string
	Heads         []string
	Equivocations []equivocationVector
}

// equivocationVector is one author sequence holding several operations.
type equivocationVector struct {
	PeerID string
	Nonce  uint64
	Hashes []string
}

// configChainVector is a control chain from genesis.
type configChainVector struct {
	Name    string
	Entries [][]byte
	// HeadHash is the resulting head in hex, empty when the chain is rejected.
	HeadHash string
	// Kind is the TypeScript rejection kind, empty when the chain is valid.
	Kind string
}

// configChangeVector is one control record applied to a held configuration.
type configChangeVector struct {
	Name    string
	Current []byte
	Entry   []byte
	// NextHash is the resulting head in hex, empty when the record is rejected.
	NextHash string
	// Kind is the TypeScript rejection kind, empty when the record is valid.
	Kind string
}

// checkpointVector is one checkpoint verified under the object and checked
// against the participants of a configuration.
type checkpointVector struct {
	Name       string
	Checkpoint []byte
	Config     []byte
	// Hash is the checkpoint hash in hex, empty when verification fails.
	Hash string
	// Authority reports whether an owner under Config signed the checkpoint.
	Authority bool
}

// marshalJSON encodes the vectors as the JSON the TypeScript tests import.
// Byte fields are standard base64.
func (v *operationLogVectors) marshalJSON() []byte {
	// Encode the operation cases.
	var a fastjson.Arena
	ops := jsonArray(&a, len(v.Operations), func(i int) *fastjson.Value {
		c := v.Operations[i]
		return jsonObject(&a, "name", a.NewString(c.Name), "operation", jsonBytes(&a, c.Operation), "hash", a.NewString(c.Hash))
	})
	sets := jsonArray(&a, len(v.OperationSets), func(i int) *fastjson.Value {
		c := v.OperationSets[i]
		equivocations := jsonArray(&a, len(c.Equivocations), func(j int) *fastjson.Value {
			e := c.Equivocations[j]
			return jsonObject(&a, "peerId", a.NewString(e.PeerID), "nonce", a.NewNumberString(strconv.FormatUint(e.Nonce, 10)), "hashes", jsonStrings(&a, e.Hashes))
		})
		return jsonObject(&a, "name", a.NewString(c.Name), "operations", jsonStrings(&a, c.Operations), "heads", jsonStrings(&a, c.Heads), "equivocations", equivocations)
	})

	// Encode the control-record cases.
	chains := jsonArray(&a, len(v.ConfigChains), func(i int) *fastjson.Value {
		c := v.ConfigChains[i]
		entries := jsonArray(&a, len(c.Entries), func(j int) *fastjson.Value { return jsonBytes(&a, c.Entries[j]) })
		return jsonObject(&a, "name", a.NewString(c.Name), "entries", entries, "headHash", a.NewString(c.HeadHash), "kind", a.NewString(c.Kind))
	})
	changes := jsonArray(&a, len(v.ConfigChanges), func(i int) *fastjson.Value {
		c := v.ConfigChanges[i]
		return jsonObject(&a, "name", a.NewString(c.Name), "current", jsonBytes(&a, c.Current), "entry", jsonBytes(&a, c.Entry), "nextHash", a.NewString(c.NextHash), "kind", a.NewString(c.Kind))
	})

	// Encode the checkpoint cases.
	checkpoints := jsonArray(&a, len(v.Checkpoints), func(i int) *fastjson.Value {
		c := v.Checkpoints[i]
		authority := a.NewFalse()
		if c.Authority {
			authority = a.NewTrue()
		}
		return jsonObject(&a, "name", a.NewString(c.Name), "checkpoint", jsonBytes(&a, c.Checkpoint), "config", jsonBytes(&a, c.Config), "hash", a.NewString(c.Hash), "authority", authority)
	})

	// Join them under the object ID.
	out := jsonObject(&a, "sharedObjectId", a.NewString(v.SharedObjectID), "operations", ops, "operationSets", sets, "configChains", chains, "configChanges", changes, "checkpoints", checkpoints)
	return append(out.MarshalTo(nil), '\n')
}

// jsonObject builds an object from alternating keys and values.
func jsonObject(a *fastjson.Arena, kv ...any) *fastjson.Value {
	obj := a.NewObject()
	for i := 0; i < len(kv); i += 2 {
		obj.Set(kv[i].(string), kv[i+1].(*fastjson.Value))
	}
	return obj
}

// jsonArray builds an array of n items.
func jsonArray(a *fastjson.Arena, n int, item func(i int) *fastjson.Value) *fastjson.Value {
	arr := a.NewArray()
	for i := range n {
		arr.SetArrayItem(i, item(i))
	}
	return arr
}

// jsonStrings builds an array of strings.
func jsonStrings(a *fastjson.Arena, ss []string) *fastjson.Value {
	return jsonArray(a, len(ss), func(i int) *fastjson.Value { return a.NewString(ss[i]) })
}

// jsonBytes encodes data as a standard base64 string.
func jsonBytes(a *fastjson.Arena, data []byte) *fastjson.Value {
	return a.NewString(base64.StdEncoding.EncodeToString(data))
}

// TestOperationLogVectors checks that the Go core produces and accepts exactly
// the shared vectors, so the TypeScript core is tested against Go behavior.
func TestOperationLogVectors(t *testing.T) {
	// Build the vectors from deterministic keys and signatures.
	data := buildOperationLogVectors(t).marshalJSON()

	// Regenerate on request; otherwise the committed file must match.
	if os.Getenv("SPACEWAVE_WRITE_VECTORS") != "" {
		if err := os.WriteFile(operationLogVectorsPath, data, 0o644); err != nil { //nolint:gosec
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(operationLogVectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, data) {
		t.Fatal("operation log vectors are stale; regenerate with SPACEWAVE_WRITE_VECTORS=1")
	}
}

// vectorKey derives a deterministic Ed25519 key and peer ID from name.
func vectorKey(t *testing.T, name string) (crypto.PrivKey, string) {
	// Seed the key from the name.
	t.Helper()
	seed := sha256.Sum256([]byte("spacewave operation log vector " + name))
	priv, _, err := crypto.GenerateEd25519Key(bytes.NewReader(seed[:]))
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, peerID.String()
}

// signVectorInner signs inner as written without validating it.
func signVectorInner(t *testing.T, priv crypto.PrivKey, inner *SOOperationInner) *SOOperation {
	// Sign the encoded body as written.
	t.Helper()
	data := mustMarshalVT(t, inner)
	sig, err := peer.NewSignature(SOOperationSignatureContext, priv, hash.RecommendedHashType, data, true)
	if err != nil {
		t.Fatal(err)
	}
	return &SOOperation{Inner: data, Signature: sig}
}

// buildOperationLogVectors builds every shared case and checks it in Go.
func buildOperationLogVectors(t *testing.T) *operationLogVectors {
	// Build the operation cases and the sets made from them.
	t.Helper()
	vectors := &operationLogVectors{SharedObjectID: vectorObjectID}
	ops := buildOperationVectors(t, vectors)
	vectors.OperationSets = buildOperationSetVectors(t, ops)

	// Build the control-record and checkpoint cases.
	buildConfigVectors(t, vectors)
	vectors.Checkpoints = buildCheckpointVectors(t)
	return vectors
}

// buildCheckpointVectors returns the checkpoint cases, each checked in Go.
func buildCheckpointVectors(t *testing.T) []checkpointVector {
	// Owner A and owner C govern the object with writer B.
	t.Helper()
	privA, peerA := vectorKey(t, "owner-a")
	privB, peerB := vectorKey(t, "member-b")
	privC, peerC := vectorKey(t, "owner-c")
	config := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peerA, Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peerB, Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: peerC, Role: SOParticipantRole_SOParticipantRole_OWNER},
	}}
	configHash := bytes.Repeat([]byte{0xc1}, 32)
	sign := func(inner *SOCheckpointInner, privs ...crypto.PrivKey) *SOCheckpoint {
		checkpoint := &SOCheckpoint{Inner: mustMarshalVT(t, inner)}
		for _, priv := range privs {
			if err := checkpoint.CoSign(priv); err != nil {
				t.Fatal(err)
			}
		}
		return checkpoint
	}

	// Build a genesis.
	genesisInner := &SOCheckpointInner{SharedObjectId: vectorObjectID, ConfigHash: configHash, StateData: []byte("genesis"), ReplayVersion: SOReplayVersion}
	genesis := sign(genesisInner, privA)
	authors := []*SOOperationPosition{
		{PeerId: peerA, Nonce: 3, OpHash: bytes.Repeat([]byte{0xa3}, 32)},
		{PeerId: peerB, Nonce: 1, OpHash: bytes.Repeat([]byte{0xb1}, 32)},
	}
	slices.SortFunc(authors, func(x, y *SOOperationPosition) int { return strings.Compare(x.GetPeerId(), y.GetPeerId()) })

	// Build its successor covering one operation of each author.
	nextInner := &SOCheckpointInner{
		SharedObjectId: vectorObjectID, Height: 1, PrevCheckpointHash: genesis.Hash(), ConfigHash: configHash,
		StateData:     []byte("next"),
		ReplayVersion: SOReplayVersion, KeyEpoch: 1, Authors: authors,
	}

	// Derive the invalid variants.
	unsorted := nextInner.CloneVT()
	slices.Reverse(unsorted.Authors)
	genesisWithAuthors := genesisInner.CloneVT()
	genesisWithAuthors.Authors = nextInner.GetAuthors()
	otherObject := genesisInner.CloneVT()
	otherObject.SharedObjectId = "other-object"
	forged := sign(genesisInner, privA)
	forged.Inner = mustMarshalVT(t, nextInner)

	// Record each case with its Go result.
	cases := []struct {
		name       string
		checkpoint *SOCheckpoint
	}{
		{"genesis-by-owner", genesis},
		{"next-co-signed-by-owners", sign(nextInner, privA, privC)},
		{"signed-by-writer", sign(nextInner, privB)},
		{"writer-and-owner", sign(nextInner, privB, privC)},
		{"duplicate-signer", sign(nextInner, privA, privA)},
		{"unsigned", sign(nextInner)},
		{"signature-over-other-body", forged},
		{"bound-to-other-object", sign(otherObject, privA)},
		{"authors-unsorted", sign(unsorted, privA)},
		{"genesis-names-authors", sign(genesisWithAuthors, privA)},
	}
	out := make([]checkpointVector, 0, len(cases))
	for _, c := range cases {
		v := checkpointVector{Name: c.name, Checkpoint: mustMarshalVT(t, c.checkpoint), Config: mustMarshalVT(t, config)}
		if _, _, err := c.checkpoint.Verify(vectorObjectID); err == nil {
			v.Hash = hex.EncodeToString(c.checkpoint.Hash())
		}
		_, err := c.checkpoint.ValidateAuthority(vectorObjectID, config.GetParticipants())
		v.Authority = err == nil
		out = append(out, v)
	}
	return out
}

// buildOperationVectors adds the single-operation cases and returns the
// valid operations by name.
func buildOperationVectors(t *testing.T, vectors *operationLogVectors) map[string]*SOOperation {
	// Two authors write under one config.
	t.Helper()
	privA, peerA := vectorKey(t, "author-a")
	privB, _ := vectorKey(t, "author-b")
	configHash := bytes.Repeat([]byte{0xc1}, 32)
	build := func(priv crypto.PrivKey, objectID string, data string, link *SOOperationLink, localID string) *SOOperation {
		op, err := BuildSOOperation(objectID, priv, []byte(data), link, localID)
		if err != nil {
			t.Fatal(err)
		}
		return op
	}

	// Each author starts a chain; A extends its own under B's head, and A
	// signs a second operation at its first sequence.
	a1 := build(privA, vectorObjectID, "a1", &SOOperationLink{Nonce: 1, ConfigHash: configHash}, "01k6h000000000000000000001")
	b1 := build(privB, vectorObjectID, "b1", &SOOperationLink{Nonce: 1, ConfigHash: configHash}, "01k6h000000000000000000002")
	a2 := build(privA, vectorObjectID, "a2", &SOOperationLink{
		Nonce: 2, PrevOpHash: a1.Hash(), Parents: []*SOOperationPosition{opPosition(t, b1)}, ConfigHash: configHash,
	}, "01k6h000000000000000000003")
	a1Fork := build(privA, vectorObjectID, "a1-fork", &SOOperationLink{Nonce: 1, ConfigHash: configHash}, "01k6h000000000000000000004")

	// B acknowledges A's head with an operation carrying no data.
	b2Ack := build(privB, vectorObjectID, "", &SOOperationLink{
		Nonce: 2, PrevOpHash: b1.Hash(), Parents: []*SOOperationPosition{opPosition(t, a2)}, ConfigHash: configHash,
	}, "01k6h000000000000000000008")
	valid := map[string]*SOOperation{"a1": a1, "b1": b1, "a2": a2, "a1-fork": a1Fork, "b2-ack": b2Ack}

	// Malformed operations are signed as written.
	otherObject := build(privA, "other-object", "a1", &SOOperationLink{Nonce: 1, ConfigHash: configHash}, "01k6h000000000000000000001")
	impostor := signVectorInner(t, privB, &SOOperationInner{
		PeerId: peerA, LocalId: "01k6h000000000000000000005", Nonce: 1, OpData: []byte("x"),
		SharedObjectId: vectorObjectID, ProtocolVersion: SOOperationProtocolVersion, ConfigHash: configHash,
	})
	noPrev := signVectorInner(t, privA, &SOOperationInner{
		PeerId: peerA, LocalId: "01k6h000000000000000000006", Nonce: 2, OpData: []byte("x"),
		SharedObjectId: vectorObjectID, ProtocolVersion: SOOperationProtocolVersion, ConfigHash: configHash,
	})
	unsorted := signVectorInner(t, privA, &SOOperationInner{
		PeerId: peerA, LocalId: "01k6h000000000000000000007", Nonce: 2, OpData: []byte("x"),
		SharedObjectId: vectorObjectID, ProtocolVersion: SOOperationProtocolVersion, PrevOpHash: a1.Hash(),
		Parents: []*SOOperationPosition{
			{PeerId: peerA, Nonce: 1, OpHash: bytes.Repeat([]byte{0xff}, 32)},
			{PeerId: peerA, Nonce: 1, OpHash: bytes.Repeat([]byte{0x01}, 32)},
		},
		ConfigHash: configHash,
	})

	// A tampered body no longer matches its signature.
	tampered := a1.CloneVT()
	tampered.Inner = bytes.Clone(a2.GetInner())

	// Record each case with its Go verification result.
	cases := []struct {
		name string
		op   *SOOperation
	}{
		{"a1", a1}, {"b1", b1}, {"a2", a2}, {"a1-fork", a1Fork}, {"b2-ack", b2Ack},
		{"replayed-from-other-object", otherObject},
		{"signed-by-another-author", impostor},
		{"missing-prev-op-hash", noPrev},
		{"unsorted-parents", unsorted},
		{"tampered-body", tampered},
	}
	for _, c := range cases {
		v := operationVector{Name: c.name, Operation: mustMarshalVT(t, c.op)}
		if _, err := c.op.Verify(vectorObjectID); err == nil {
			v.Hash = hex.EncodeToString(c.op.Hash())
		}
		if _, ok := valid[c.name]; ok != (v.Hash != "") {
			t.Fatalf("%s: unexpected verification result", c.name)
		}
		vectors.Operations = append(vectors.Operations, v)
	}
	return valid
}

// buildOperationSetVectors builds sets from the valid operations.
func buildOperationSetVectors(t *testing.T, ops map[string]*SOOperation) []operationSetVector {
	// Each set is built in the listed order; TypeScript also builds it reversed.
	t.Helper()
	sets := [][]string{
		{"a1", "b1"},
		{"a1", "b1", "a2"},
		{"a2", "b1", "a1", "a1-fork", "a1"},
		{"a1", "b1", "a2", "b2-ack"},
	}
	out := make([]operationSetVector, 0, len(sets))
	for _, names := range sets {
		// Add each operation, keeping duplicates out of the set.
		set := NewSOOperationSet(vectorObjectID, nil)
		for _, name := range names {
			if _, err := set.Add(ops[name]); err != nil {
				t.Fatal(err)
			}
		}

		// Record the heads and evidence in hex.
		v := operationSetVector{Operations: names, Heads: hexHashes(headHashes(set.Heads())), Equivocations: []equivocationVector{}}
		for i, name := range names {
			v.Name += map[bool]string{true: "+", false: ""}[i > 0] + name
		}
		for _, eq := range set.Equivocations() {
			v.Equivocations = append(v.Equivocations, equivocationVector{PeerID: eq.PeerID, Nonce: eq.Nonce, Hashes: hexHashes(eq.Hashes)})
		}
		out = append(out, v)
	}
	return out
}

// hexHashes encodes hashes in hex, in order.
func hexHashes(hashes [][]byte) []string {
	out := make([]string, len(hashes))
	for i, h := range hashes {
		out[i] = hex.EncodeToString(h)
	}
	return out
}

// buildConfigVectors adds the control chain and single transition cases.
func buildConfigVectors(t *testing.T, vectors *operationLogVectors) {
	// Owner A creates the object; B joins as a writer and C as an owner.
	t.Helper()
	privA, peerA := vectorKey(t, "owner-a")
	privB, peerB := vectorKey(t, "member-b")
	privC, peerC := vectorKey(t, "owner-c")
	privB2, peerB2 := vectorKey(t, "member-b-second-device")
	owner := SOParticipantRole_SOParticipantRole_OWNER
	writer := SOParticipantRole_SOParticipantRole_WRITER
	member := &SOParticipantConfig{PeerId: peerB, Role: writer, EntityId: "account-b", Username: "bob"}

	// Build the main chain: genesis, add B and C, then remove C.
	cfg0 := &SharedObjectConfig{Participants: []*SOParticipantConfig{{PeerId: peerA, Role: owner}}}
	genesis := buildVectorChange(t, vectorObjectID, &SharedObjectConfig{}, cfg0, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, privA)
	cur0 := vectorHead(t, cfg0, genesis)
	add := buildVectorChange(t, vectorObjectID, cur0, withParticipants(cur0, cfg0.Participants[0], member, &SOParticipantConfig{PeerId: peerC, Role: owner}),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, privA)
	cur1 := vectorHead(t, add.GetConfig(), add)
	remove := buildVectorChange(t, vectorObjectID, cur1, withParticipants(cur1, cfg0.Participants[0], member),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, privA)
	cur2 := vectorHead(t, remove.GetConfig(), remove)

	// Build the competing, unauthorized and co-signed records.
	competing := buildVectorChange(t, vectorObjectID, cur0, withParticipants(cur0, cfg0.Participants[0], member),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, privA)
	byRemoved := buildVectorChange(t, vectorObjectID, cur2, withParticipants(cur2, cfg0.Participants[0]),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, privC)
	coSigned := buildVectorChange(t, vectorObjectID, cur1, withParticipants(cur1, cfg0.Participants[0], &SOParticipantConfig{PeerId: peerC, Role: owner}),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, privA)
	if err := signSOConfigChange(coSigned, privC); err != nil {
		t.Fatal(err)
	}
	doubleSigned := remove.CloneVT()
	doubleSigned.Signatures = append(doubleSigned.Signatures, remove.GetSignatures()[0].CloneVT())

	// Build the genesis records that must fail.
	genesisByNonOwner := buildVectorChange(t, vectorObjectID, &SharedObjectConfig{}, cfg0, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, privB)
	genesisOtherObject := buildVectorChange(t, "other-object", &SharedObjectConfig{}, cfg0, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, privA)

	// Record each chain with its Go result.
	chains := []struct {
		name    string
		kind    string
		entries []*SOConfigChange
	}{
		{"genesis", "", []*SOConfigChange{genesis}},
		{"add-then-remove", "", []*SOConfigChange{genesis, add, remove}},
		{"competing-record", "", []*SOConfigChange{genesis, competing}},
		{"competing-records-both-applied", "stale", []*SOConfigChange{genesis, add, competing}},
		{"record-from-removed-member", "unauthorized", []*SOConfigChange{genesis, add, remove, byRemoved}},
		{"co-signed-by-two-owners", "", []*SOConfigChange{genesis, add, coSigned}},
		{"duplicate-signer", "unauthorized", []*SOConfigChange{genesis, add, doubleSigned}},
		{"genesis-by-non-owner", "unauthorized", []*SOConfigChange{genesisByNonOwner}},
		{"genesis-for-other-object", "invalid", []*SOConfigChange{genesisOtherObject}},
	}
	for _, c := range chains {
		v := configChainVector{Name: c.name, Kind: c.kind}
		for _, entry := range c.entries {
			v.Entries = append(v.Entries, mustMarshalVT(t, entry))
		}
		err := VerifyConfigChain(vectorObjectID, c.entries)
		if (err == nil) != (c.kind == "") {
			t.Fatalf("%s: unexpected chain result: %v", c.name, err)
		}
		if err == nil {
			v.HeadHash = hex.EncodeToString(mustHashConfigChange(t, c.entries[len(c.entries)-1]))
		}
		vectors.ConfigChains = append(vectors.ConfigChains, v)
	}

	// B's second device enrolls itself under B's entity.
	device := &SOParticipantConfig{PeerId: peerB2, Role: writer, EntityId: "account-b", Username: "bob"}
	enroll := buildVectorChange(t, vectorObjectID, cur2, withParticipants(cur2, append(cur2.GetParticipants(), device)...),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER, privB2)
	staleEnroll := buildVectorChange(t, vectorObjectID, cur1, withParticipants(cur1, append(cur1.GetParticipants(), device)...),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER, privB2)
	escalated := &SOParticipantConfig{PeerId: peerB2, Role: owner, EntityId: "account-b", Username: "bob"}
	escalate := buildVectorChange(t, vectorObjectID, cur2, withParticipants(cur2, append(cur2.GetParticipants(), escalated)...),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER, privB2)
	byWriter := buildVectorChange(t, vectorObjectID, cur2, withParticipants(cur2, cfg0.Participants[0]),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, privB)
	otherObject := buildVectorChange(t, "other-object", cur2, withParticipants(cur2, cfg0.Participants[0]),
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, privA)

	// Record each transition with its Go result.
	changes := []struct {
		name    string
		kind    string
		current *SharedObjectConfig
		entry   *SOConfigChange
	}{
		{"owner-removes-participant", "", cur1, remove},
		{"self-enroll", "", cur2, enroll},
		{"self-enroll-from-stale-head", "stale", cur2, staleEnroll},
		{"self-enroll-role-escalation", "unauthorized", cur2, escalate},
		{"writer-removes-participant", "unauthorized", cur2, byWriter},
		{"record-for-other-object", "invalid", cur2, otherObject},
	}
	for _, c := range changes {
		v := configChangeVector{Name: c.name, Kind: c.kind, Current: mustMarshalVT(t, c.current), Entry: mustMarshalVT(t, c.entry)}
		next, err := VerifyConfigChange(vectorObjectID, c.current, c.entry)
		if (err == nil) != (c.kind == "") {
			t.Fatalf("%s: unexpected change result: %v", c.name, err)
		}
		if err == nil {
			v.NextHash = hex.EncodeToString(next.GetConfigChainHash())
		}
		vectors.ConfigChanges = append(vectors.ConfigChanges, v)
	}
}

// buildVectorChange signs a record moving current to next.
func buildVectorChange(
	t *testing.T,
	objectID string,
	current, next *SharedObjectConfig,
	kind SOConfigChangeType,
	priv crypto.PrivKey,
) *SOConfigChange {
	t.Helper()
	entry, err := BuildSOConfigChange(objectID, current, next, kind, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

// vectorHead returns cfg carrying entry as its chain head.
func vectorHead(t *testing.T, cfg *SharedObjectConfig, entry *SOConfigChange) *SharedObjectConfig {
	t.Helper()
	return configWithAppliedConfigChainHead(cfg, entry.GetConfigSeqno(), mustHashConfigChange(t, entry))
}

// withParticipants returns a copy of cfg with its head and the given participants.
func withParticipants(cfg *SharedObjectConfig, participants ...*SOParticipantConfig) *SharedObjectConfig {
	next := cfg.CloneVT()
	next.Participants = nil
	for _, p := range participants {
		next.Participants = append(next.Participants, p.CloneVT())
	}
	return next
}

// mustHashConfigChange returns the hash of a control record.
func mustHashConfigChange(t *testing.T, entry *SOConfigChange) []byte {
	t.Helper()
	h, err := HashSOConfigChange(entry)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
