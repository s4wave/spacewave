package sobject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanLeaveConformance compares consent, retained history and real owner publications.
func TestLeanLeaveConformance(t *testing.T) {
	oracle := leanOracle(t)
	peers := createMockPeers(t, 4)
	var cases []leanCase
	for seed := range uint64(30) {
		cases = append(cases, runLeanLeaveScenario(t, peers, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanLeave searches consent, admission continuity and owner publication boundaries.
func FuzzLeanLeave(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(11))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanLeaveScenario(t, createMockPeers(t, 4), seed))
	})
}

// runLeanLeaveScenario retains actual signatures and verifies complete returned history and state.
func runLeanLeaveScenario(t *testing.T, peers []peer.Peer, seed uint64) []leanCase {
	t.Helper()
	projection := &configChainScenario{t: t}
	a := &projection.arena
	keys := make([]crypto.PrivKey, len(peers))
	for i, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}
	base := createMockSOState(peers[:3], []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	base.Config.ConfigChainHash = bytes.Repeat([]byte{byte(seed%254 + 1)}, 32)
	base.Config.ConfigChainSeqno = seed%19 + 1
	base.Config.Participants[1].Role = SOParticipantRole(seed%3 + 1)
	base.Config.Participants[2].Role = SOParticipantRole(seed/3%3 + 1)
	signed := leanLeaveSignedHead(t, base.Config)
	base.Root = createMockSORoot(t, 1, peers[0])
	_, grants, _, err := RotateTransformKey(keys[0], mockSharedObjectID, base.Config.Participants, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	base.RootGrants = grants

	var cases []leanCase
	for variant := range 31 {
		previous := base.CloneVT()
		requestKeys := []crypto.PrivKey{keys[1]}
		signer := keys[0]
		lockOK, writeOK, watchOK, historyOK := true, true, true, true
		hostID := mockSharedObjectID
		switch variant {
		case 1:
			requestKeys = keys[1:3]
		case 2:
			requestKeys = keys[:1]
		case 3:
			requestKeys = keys[:3]
		case 4:
			requestKeys = keys[3:]
		case 18:
			writeOK = false
		case 19:
			lockOK = false
		case 20:
			watchOK = false
		case 21:
			signer = keys[1]
		case 22:
			signer = keys[3]
		case 23:
			previous.Config.ConfigChainSeqno = math.MaxUint64
		case 28:
			requestKeys = []crypto.PrivKey{keys[1], keys[3]}
		case 30:
			hostID = strings.Repeat("é", 120)
		}
		request, err := BuildSOLeaveRequest(hostID, previous.Config.ConfigChainHash, requestKeys...)
		if err != nil {
			t.Fatal(err)
		}
		switch variant {
		case 5:
			request.Signatures = nil
		case 6:
			request.Signatures = append(request.Signatures, request.Signatures[0].CloneVT())
		case 7:
			request.Signatures[0].SigData[0] ^= 1
		case 8:
			request.Signatures[0].PubKey = nil
		case 9:
			request.Signatures[0] = nil
		case 10:
			request.SharedObjectId = "another object"
		case 11:
			request.ConfigHash = request.ConfigHash[:31]
		case 12:
			request.SharedObjectId = strings.Repeat("é", 130)
		case 13:
			request.ConfigHash[0] ^= 1
			historyOK = false
		case 25:
			request = nil
		case 26:
			previous.Config.ConfigChainHash = nil
			historyOK = false
		case 29:
			for len(request.Signatures) <= MaxParticipants {
				request.Signatures = append(request.Signatures, request.Signatures[0].CloneVT())
			}
		}
		if variant >= 10 && variant <= 13 {
			unsigned := request.CloneVT()
			unsigned.Signatures = nil
			request.Signatures = nil
			for _, key := range requestKeys {
				signature, err := peer.NewSignature("sobject leave", key, hash.RecommendedHashType, mustMarshalVT(t, unsigned), true)
				if err != nil {
					t.Fatal(err)
				}
				request.Signatures = append(request.Signatures, signature)
			}
		}
		requestHash := sha256.Sum256(mustMarshalVT(t, request))
		var history []*SOConfigChange
		advance := func(next *SharedObjectConfig, kind SOConfigChangeType, consent []byte) {
			t.Helper()
			entry, err := BuildSOConfigChange(previous.Config, next, kind, keys[0], &SORevocationInfo{LeaveRequestHash: consent})
			if err != nil {
				t.Fatal(err)
			}
			config, err := VerifyConfigChange(previous.Config, entry)
			if err != nil {
				t.Fatal(err)
			}
			previous.Config = config
			history = append(history, entry)
			previous.RootGrants = slices.DeleteFunc(previous.RootGrants, func(g *SOGrant) bool {
				return !slices.ContainsFunc(config.Participants, func(p *SOParticipantConfig) bool {
					return p.PeerId == g.GetPeerId()
				})
			})
		}
		switch variant {
		case 14:
			advance(previous.Config, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, nil)
		case 15:
			next := previous.Config.CloneVT()
			next.Participants = append(next.Participants, &SOParticipantConfig{PeerId: peers[3].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_READER})
			advance(next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, nil)
		case 16, 17:
			next := previous.Config.CloneVT()
			next.Participants = slices.Delete(next.Participants, 1, 2)
			var consent []byte
			if variant == 17 {
				consent = requestHash[:]
			}
			advance(next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, consent)
			next = previous.Config.CloneVT()
			next.Participants = append(next.Participants, base.Config.Participants[1].CloneVT())
			advance(next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, nil)
		case 27:
			next := previous.Config.CloneVT()
			next.Participants = next.Participants[:2]
			advance(next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, nil)
			advance(previous.Config, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE, nil)
		}
		snapshot := previous.CloneVT()
		if variant == 24 {
			previous.Config.ConfigChainHash[0] ^= 1
		}
		projectedRequest := projectLeanLeaveRequest(t, a, request)
		var rawPeers []string
		for _, sig := range projectedRequest.GetArray("signatures") {
			rawPeers = append(rawPeers, string(sig.GetStringBytes("signer")))
		}
		next := snapshot.Config.CloneVT()
		next.Participants = slices.DeleteFunc(next.Participants, func(p *SOParticipantConfig) bool {
			return slices.Contains(rawPeers, p.GetPeerId())
		})
		entry, err := BuildSOConfigChange(snapshot.Config, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
			signer, &SORevocationInfo{LeaveRequestHash: requestHash[:]})
		if err != nil {
			t.Fatal(err)
		}
		projectedEntry := projection.entryJSON(entry)
		attempt := a.NewObject()
		attempt.Set("previous", projectLeanState(t, a, previous))
		attempt.Set("snapshot", a.NewNull())
		if watchOK {
			attempt.Set("snapshot", projectLeanConfig(snapshot.Config).json(a))
		}
		attempt.Set("latest", projectLeanConfig(snapshot.Config).json(a))
		attempt.Set("history", a.NewNull())
		attempt.Set("signed", a.NewNull())
		if historyOK {
			attempt.Set("history", projectLeanLeaveChanges(projection, history))
			attempt.Set("signed", projectLeanConfig(base.Config).json(a))
		}
		attempt.Set("sig", projectedEntry.Get("sig"))
		attempt.Set("hash", projectedEntry.Get("hash"))
		attempt.Set("buildOK", a.NewTrue())
		attempt.Set("lockOK", leanBool(a, lockOK))
		attempt.Set("writeOK", leanBool(a, writeOK))
		attempts := a.NewArray()
		attempts.SetArrayItem(0, attempt)
		store := &leanHostStore{state: previous.CloneVT(), lockOK: lockOK, writeOK: writeOK}
		retained := map[string]*SOConfigChange{string(base.Config.ConfigChainHash): signed}
		watch := ccontainer.NewCContainer[*SOState](snapshot)
		host := NewSOHost(t.Context(), func(context.Context, string, func()) (ccontainer.Watchable[*SOState], func(), error) {
			if !watchOK {
				return nil, nil, errors.New("injected watch failure")
			}
			return watch, func() {}, nil
		}, store.lock, hostID, &SOHostSyncFuncs{History: func(context.Context, string, []byte, []byte) ([]*SOConfigChange, error) {
			if !historyOK {
				return nil, errors.New("injected history failure")
			}
			return history, nil
		}, Entry: func(_ context.Context, _ string, hash []byte) (*SOConfigChange, error) {
			return retained[string(hash)], nil
		}})
		response, callErr := LeaveSOParticipants(t.Context(), host, signer, request)
		result := a.NewNull()
		if callErr == nil {
			result = a.NewObject()
			result.Set("retry", a.NewFalse())
			result.Set("changes", projectLeanLeaveChanges(projection, response.GetChanges()))
			outcome := a.NewObject()
			outcome.Set("state", projectLeanState(t, a, store.state))
			outcome.Set("revoked", a.NewFalse())
			outcome.Set("wrote", leanBool(a, store.writes != 0))
			result.Set("outcome", outcome)
		} else if !store.state.EqualVT(previous) || store.writes != 0 {
			t.Fatal("failed leave changed the held state")
		}
		trace := a.NewObject()
		trace.Set("pending", a.NewFalse())
		trace.Set("result", result)
		req := a.NewObject()
		req.Set("op", a.NewString("leaveTrace"))
		req.Set("request", projectedRequest)
		req.Set("hostId", a.NewString(hostID))
		req.Set("requestHash", a.NewString(hex.EncodeToString(requestHash[:])))
		req.Set("attempts", attempts)
		name := " seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		cases = append(cases, leanCase{
			name: "leaveTrace" + name, request: req.MarshalTo(nil),
			ok: callErr == nil, field: "leave", value: trace.MarshalTo(nil),
		})

		verified, verifyErr := request.Verify()
		identities := a.NewNull()
		if verifyErr == nil {
			identities = leanLeavePeers(a, verified)
		}
		req = a.NewObject()
		req.Set("op", a.NewString("verifyLeave"))
		req.Set("request", projectedRequest)
		cases = append(cases, leanCase{
			name: "verifyLeave" + name, request: req.MarshalTo(nil),
			ok: verifyErr == nil, field: "peers", value: identities.MarshalTo(nil),
		})

		buildKeys := slices.Clone(requestKeys)
		switch variant {
		case 5:
			buildKeys = nil
		case 6:
			buildKeys = append(buildKeys, buildKeys[0])
		}
		unsigned := &SOLeaveRequest{SharedObjectId: request.GetSharedObjectId(), ConfigHash: request.GetConfigHash()}
		unsignedData := mustMarshalVT(t, unsigned)
		for _, key := range buildKeys {
			signature, err := peer.NewSignature("sobject leave", key, hash.RecommendedHashType, unsignedData, true)
			if err != nil {
				t.Fatal(err)
			}
			unsigned.Signatures = append(unsigned.Signatures, signature)
		}
		built, buildErr := BuildSOLeaveRequest(request.GetSharedObjectId(), request.GetConfigHash(), buildKeys...)
		builtValue := a.NewNull()
		if buildErr == nil {
			builtValue = projectLeanLeaveRequest(t, a, built)
		}
		req = a.NewObject()
		req.Set("op", a.NewString("buildLeave"))
		req.Set("request", projectLeanLeaveRequest(t, a, unsigned))
		req.Set("signOK", a.NewTrue())
		cases = append(cases, leanCase{
			name: "buildLeave" + name, request: req.MarshalTo(nil),
			ok: buildErr == nil, field: "request", value: builtValue.MarshalTo(nil),
		})

		req = a.NewObject()
		req.Set("op", a.NewString("leaveProofsRemainCurrent"))
		req.Set("peers", leanLeavePeers(a, rawPeers))
		req.Set("signed", projectLeanConfig(base.Config).json(a))
		req.Set("changes", projectLeanLeaveChanges(projection, history))
		cases = append(cases, leanCase{
			name: "leaveProofsRemainCurrent" + name, request: req.MarshalTo(nil),
			ok: leaveProofsRemainCurrent(rawPeers, base.Config, history),
		})
	}
	cases = append(cases, runLeanLeaveRetry(t, projection, base.Config, signed, keys[0], keys[1], seed)...)
	return cases
}

// projectLeanLeaveRequest projects individual raw signature verification without consent admission.
func projectLeanLeaveRequest(t *testing.T, a *fastjson.Arena, request *SOLeaveRequest) *fastjson.Value {
	t.Helper()
	unsigned := request.CloneVT()
	if unsigned != nil {
		unsigned.Signatures = nil
	}
	data := mustMarshalVT(t, unsigned)
	signatures := a.NewArray()
	for i, signature := range request.GetSignatures() {
		signatures.SetArrayItem(i, projectLeanSig(a, signature, data, func(string) string { return "sobject leave" }))
	}
	result := a.NewObject()
	result.Set("object", a.NewString(request.GetSharedObjectId()))
	result.Set("configHash", a.NewString(hex.EncodeToString(request.GetConfigHash())))
	result.Set("signatures", signatures)
	return result
}

// leanLeaveSignedHead addresses a seeded configuration by a retained entry, as
// provider history does for every head a participant can sign.
func leanLeaveSignedHead(t *testing.T, config *SharedObjectConfig) *SOConfigChange {
	t.Helper()
	entry := &SOConfigChange{
		ConfigSeqno: config.GetConfigChainSeqno(),
		Config:      config.CloneVT(),
		ChangeType:  SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE,
	}
	entry.Config.ConfigChainHash = nil
	hash, err := HashSOConfigChange(entry)
	if err != nil {
		t.Fatal(err)
	}
	config.ConfigChainHash = hash
	return entry
}

// projectLeanLeaveChanges retains each signed entry and the exact consent-request identity.
func projectLeanLeaveChanges(projection *configChainScenario, changes []*SOConfigChange) *fastjson.Value {
	a := &projection.arena
	result := a.NewArray()
	for i, change := range changes {
		entry := a.NewObject()
		entry.Set("entry", projection.entryJSON(change))
		entry.Set("requestHash", a.NewString(hex.EncodeToString(change.GetRevocationInfo().GetLeaveRequestHash())))
		result.SetArrayItem(i, entry)
	}
	return result
}

// leanLeavePeers preserves the order and multiplicity of consent identities.
func leanLeavePeers(a *fastjson.Arena, peers []string) *fastjson.Value {
	result := a.NewArray()
	for i, id := range peers {
		result.SetArrayItem(i, a.NewString(id))
	}
	return result
}

// runLeanLeaveRetry forces one real optimistic conflict and retains both observed attempts.
func runLeanLeaveRetry(t *testing.T, projection *configChainScenario, config *SharedObjectConfig,
	signed *SOConfigChange, owner, departing crypto.PrivKey, seed uint64,
) []leanCase {
	t.Helper()
	a := &projection.arena
	host, state := newLeaveTestHost(t, config, signed)
	request, err := BuildSOLeaveRequest(mockSharedObjectID, config.GetConfigChainHash(), departing)
	if err != nil {
		t.Fatal(err)
	}
	requestHash := sha256.Sum256(mustMarshalVT(t, request))
	advance, err := BuildSOConfigChange(config, config, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	originalLock := host.lockFn
	first := true
	calls := 0
	var advanced *SOState
	host.lockFn = func(ctx context.Context, objectID string) (SOStateLock, error) {
		calls++
		if first {
			first = false
			if err := host.ApplyConfigChange(ctx, advance, nil); err != nil {
				t.Fatal(err)
			}
			advanced = (*state).CloneVT()
		}
		return originalLock(ctx, objectID)
	}
	response, err := LeaveSOParticipants(t.Context(), host, owner, request)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || advanced == nil || len(response.GetChanges()) != 2 {
		t.Fatalf("leave did not retry the observed conflict: locks=%d changes=%d", calls, len(response.GetChanges()))
	}
	departingID, err := peer.IDFromPrivateKey(departing)
	if err != nil {
		t.Fatal(err)
	}
	attempts := a.NewArray()
	for i, snapshot := range []*SharedObjectConfig{config, advanced.Config} {
		next := snapshot.CloneVT()
		next.Participants = slices.DeleteFunc(next.Participants, func(p *SOParticipantConfig) bool {
			return p.GetPeerId() == departingID.String()
		})
		entry, err := BuildSOConfigChange(snapshot, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
			owner, &SORevocationInfo{LeaveRequestHash: requestHash[:]})
		if err != nil {
			t.Fatal(err)
		}
		projected := projection.entryJSON(entry)
		attempt := a.NewObject()
		attempt.Set("previous", projectLeanState(t, a, advanced))
		attempt.Set("snapshot", projectLeanConfig(snapshot).json(a))
		attempt.Set("latest", projectLeanConfig(advanced.Config).json(a))
		attempt.Set("history", projectLeanLeaveChanges(projection, []*SOConfigChange{advance}))
		attempt.Set("signed", projectLeanConfig(config).json(a))
		attempt.Set("sig", projected.Get("sig"))
		attempt.Set("hash", projected.Get("hash"))
		attempt.Set("buildOK", a.NewTrue())
		attempt.Set("lockOK", a.NewTrue())
		attempt.Set("writeOK", a.NewTrue())
		attempts.SetArrayItem(i, attempt)
	}
	result := a.NewObject()
	result.Set("retry", a.NewFalse())
	result.Set("changes", projectLeanLeaveChanges(projection, response.GetChanges()))
	outcome := a.NewObject()
	outcome.Set("state", projectLeanState(t, a, *state))
	outcome.Set("revoked", a.NewFalse())
	outcome.Set("wrote", a.NewTrue())
	result.Set("outcome", outcome)
	trace := a.NewObject()
	trace.Set("pending", a.NewFalse())
	trace.Set("result", result)
	req := a.NewObject()
	req.Set("op", a.NewString("leaveTrace"))
	req.Set("request", projectLeanLeaveRequest(t, a, request))
	req.Set("hostId", a.NewString(mockSharedObjectID))
	req.Set("requestHash", a.NewString(hex.EncodeToString(requestHash[:])))
	req.Set("attempts", attempts)
	cases := []leanCase{{
		name: "leaveTrace retry seed " + strconv.FormatUint(seed, 10), request: req.MarshalTo(nil),
		ok: true, field: "leave", value: trace.MarshalTo(nil),
	}}

	// The observed first conflict returned to the loop rather than terminating.
	partial := a.NewArray()
	partial.SetArrayItem(0, attempts.GetArray()[0])
	req.Set("attempts", partial)
	trace.Set("pending", a.NewTrue())
	trace.Set("result", a.NewNull())
	cases = append(cases, leanCase{
		name:    "leaveTrace pending seed " + strconv.FormatUint(seed, 10),
		request: req.MarshalTo(nil), ok: false, field: "leave", value: trace.MarshalTo(nil),
	})
	return cases
}
