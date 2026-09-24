package sobject_sync

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// TestLeanSyncCatchupConformance compares real serialized pages and response mutations.
func TestLeanSyncCatchupConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(6) {
		cases = append(cases, leanSyncCatchupCases(t, seed)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncCatchup varies authenticated history and the paging boundaries around it.
func FuzzLeanSyncCatchup(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(17))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanSync(t, leanSyncOracle(t), leanSyncCatchupCases(t, seed))
	})
}

// leanSyncConfig projects the configuration-chain model without admission decisions.
func leanSyncConfig(arena *fastjson.Arena, config *sobject.SharedObjectConfig) *fastjson.Value {
	if config == nil {
		return arena.NewNull()
	}
	value := arena.NewObject()
	participants := leanSyncParticipants(arena, config.GetParticipants())
	for index, participant := range config.GetParticipants() {
		if _, err := participant.ParsePeerID(); err != nil {
			participants.GetArray()[index].Set("peer", arena.NewString(""))
		}
	}
	value.Set("participants", participants)
	value.Set("mode", arena.NewNumberInt(int(config.GetConsensusMode())))
	value.Set("hash", arena.NewString(hex.EncodeToString(config.GetConfigChainHash())))
	value.Set("seqno", arena.NewNumberString(strconv.FormatUint(config.GetConfigChainSeqno(), 10)))
	return value
}

// leanSyncChange projects canonical bytes, signature verification and the decoded entry.
func leanSyncChange(t *testing.T, arena *fastjson.Arena, change *sobject.SOConfigChange) *fastjson.Value {
	t.Helper()
	var digest []byte
	var hashOK bool
	signature := arena.NewNull()
	if change != nil {
		var err error
		digest, err = sobject.HashSOConfigChange(change)
		hashOK = err == nil
		if sig := change.GetSignature(); sig != nil {
			signature = arena.NewObject()
			var signer string
			var valid bool
			public, err := sig.ParsePubKey()
			if err == nil && public != nil {
				id, idErr := peer.IDFromPublicKey(public)
				unsigned := change.CloneVT()
				unsigned.Signature = nil
				data, encodeErr := unsigned.MarshalVT()
				if idErr == nil && encodeErr == nil {
					verified, verifyErr := sig.VerifyWithPublic("sobject config change", public, data)
					signer, valid = id.String(), verified && verifyErr == nil
				}
			}
			signature.Set("signer", arena.NewString(signer))
			signature.Set("valid", leanSyncBool(arena, valid))
		}
	}
	entry := arena.NewObject()
	entry.Set("seqno", arena.NewNumberString(strconv.FormatUint(change.GetConfigSeqno(), 10)))
	entry.Set("config", leanSyncConfig(arena, change.GetConfig()))
	entry.Set("sig", signature)
	entry.Set("prev", arena.NewString(hex.EncodeToString(change.GetPreviousHash())))
	entry.Set("kind", arena.NewNumberInt(int(change.GetChangeType())))
	entry.Set("hash", arena.NewString(hex.EncodeToString(digest)))
	value := arena.NewObject()
	value.Set("entry", entry)
	value.Set("bytes", arena.NewNumberInt(change.SizeVT()))
	value.Set("hashOK", leanSyncBool(arena, hashOK))
	return value
}

// leanSyncChanges preserves the original entry order, including duplicate buffer entries.
func leanSyncChanges(t *testing.T, arena *fastjson.Arena, changes []*sobject.SOConfigChange) *fastjson.Value {
	t.Helper()
	value := arena.NewArray()
	for index, change := range changes {
		value.SetArrayItem(index, leanSyncChange(t, arena, change))
	}
	return value
}

// leanSyncHead retains every advertisement field used by the receiver.
func leanSyncHead(arena *fastjson.Arena, head *SOSyncHead) *fastjson.Value {
	value := arena.NewObject()
	value.Set("revision", arena.NewNumberString(strconv.FormatUint(head.GetRevision(), 10)))
	value.Set("configHash", arena.NewString(hex.EncodeToString(head.GetConfigHash())))
	value.Set("configSeqno", arena.NewNumberString(strconv.FormatUint(head.GetConfigSeqno(), 10)))
	value.Set("rootSeqno", arena.NewNumberString(strconv.FormatUint(head.GetRootSeqno(), 10)))
	value.Set("stateHash", arena.NewString(hex.EncodeToString(head.GetStateHash())))
	return value
}

// leanSyncReceive projects only the buffer owned by syncReceive; it has no held host state.
func leanSyncReceive(t *testing.T, arena *fastjson.Arena, receiving *syncReceive) *fastjson.Value {
	t.Helper()
	value := arena.NewObject()
	value.Set("head", leanSyncHead(arena, receiving.head))
	value.Set("base", arena.NewString(hex.EncodeToString(receiving.base)))
	value.Set("cursor", arena.NewString(hex.EncodeToString(receiving.cursor)))
	value.Set("changes", leanSyncChanges(t, arena, receiving.changes))
	value.Set("bytes", arena.NewNumberInt(receiving.size))
	return value
}

// leanSyncPage measures the complete frame rather than only its history payload.
func leanSyncPage(t *testing.T, arena *fastjson.Arena, message *SOSyncMessage) *fastjson.Value {
	t.Helper()
	page := message.GetHistoryPage()
	if page == nil {
		return arena.NewNull()
	}
	value := arena.NewObject()
	value.Set("revision", arena.NewNumberString(strconv.FormatUint(page.GetRevision(), 10)))
	value.Set("cursor", arena.NewString(hex.EncodeToString(page.GetCursor())))
	value.Set("changes", leanSyncChanges(t, arena, page.GetChanges()))
	value.Set("bytes", arena.NewNumberInt(message.SizeVT()))
	return value
}

// leanSyncSnapshot retains the bytes actually sent, including all response binding fields.
func leanSyncSnapshot(arena *fastjson.Arena, snapshot *SOSyncSnapshot) *fastjson.Value {
	if snapshot == nil {
		return arena.NewNull()
	}
	value := arena.NewObject()
	value.Set("data", arena.NewString(hex.EncodeToString(snapshot.GetSoState())))
	value.Set("rootSeqno", arena.NewNumberString(strconv.FormatUint(snapshot.GetRootSeqno(), 10)))
	value.Set("revision", arena.NewNumberString(strconv.FormatUint(snapshot.GetRevision(), 10)))
	value.Set("base", arena.NewString(hex.EncodeToString(snapshot.GetBaseHash())))
	return value
}

// leanSyncResponse projects the remaining suffix and pinned serialized snapshot.
func leanSyncResponse(t *testing.T, arena *fastjson.Arena, response *syncResponse) *fastjson.Value {
	t.Helper()
	value := arena.NewObject()
	value.Set("revision", arena.NewNumberString(strconv.FormatUint(response.revision, 10)))
	value.Set("cursor", arena.NewString(hex.EncodeToString(response.cursor)))
	value.Set("changes", leanSyncChanges(t, arena, response.changes))
	value.Set("snapshot", leanSyncSnapshot(arena, response.snapshot))
	return value
}

// leanSyncCatchupCases builds real signed host history, then exercises sender and receiver mutations.
func leanSyncCatchupCases(t *testing.T, seed uint64) []leanSyncCase {
	t.Helper()
	const objectID = "lean-paged-catchup"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, objectID, owner, reader)
	sender := newAuthenticationPeer(t, objectID, owner, initial)
	for range maxHistoryPageEntries + 2 + int(seed%4) {
		current, err := sender.soHost.GetHostState(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		change, err := sobject.BuildSOConfigChange(current.Config, current.Config, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := sender.soHost.ApplyConfigChange(t.Context(), change, nil); err != nil {
			t.Fatal(err)
		}
	}
	target, err := sender.soHost.GetHostState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response, err := sender.prepareResponse(t.Context(), target, &SOSyncHistoryRequest{Revision: seed + 1, BaseHash: initial.Config.ConfigChainHash})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := syncStateHash(target)
	if err != nil {
		t.Fatal(err)
	}
	head := &SOSyncHead{
		Revision: response.revision, ConfigHash: target.Config.ConfigChainHash,
		ConfigSeqno: target.Config.ConfigChainSeqno, RootSeqno: target.Root.InnerSeqno, StateHash: digest,
	}
	var cases []leanSyncCase
	for variant := range 14 {
		receiving := &syncReceive{head: head, base: initial.Config.ConfigChainHash, cursor: initial.Config.ConfigChainHash}
		page := &SOSyncHistoryPage{Revision: response.revision, Cursor: initial.Config.ConfigChainHash, Changes: response.changes[:3]}
		page = page.CloneVT()
		message := &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: page}}
		switch variant {
		case 1:
			page.Revision ^= 1
		case 2:
			page.Cursor = []byte("wrong cursor")
		case 3:
			page.Changes = nil
		case 4:
			page.Changes[1].PreviousHash = nil
		case 5:
			page.Changes[0].Signature.SigData = make([]byte, maxHistoryPageBytes)
		case 6:
			page.Changes = response.changes[:maxHistoryPageEntries+1]
		case 7:
			receiving.size = sobject.MaxConfigSuffixBytes - page.Changes[0].SizeVT()
		case 8:
			receiving.size = sobject.MaxConfigSuffixBytes
		case 9, 10, 11:
			receiving.changes = make([]*sobject.SOConfigChange, sobject.MaxConfigSuffixEntries+variant-10)
			for index := range receiving.changes {
				receiving.changes[index] = &sobject.SOConfigChange{}
			}
		case 12:
			message = &SOSyncMessage{}
		case 13:
			page.Changes[0] = nil
		}
		var arena fastjson.Arena
		request := arena.NewObject()
		request.Set("op", arena.NewString("appendSyncPage"))
		request.Set("before", leanSyncReceive(t, &arena, receiving))
		request.Set("page", leanSyncPage(t, &arena, message))
		err := receiving.appendPage(message)
		expected, result := arena.NewObject(), arena.NewObject()
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		result.Set("ok", leanSyncBool(&arena, err == nil))
		result.Set("state", leanSyncReceive(t, &arena, receiving))
		expected.Set("received", result)
		cases = append(cases, leanSyncCase{
			name:    "appendSyncPage seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: request.MarshalTo(nil), expected: expected.MarshalTo(nil),
		})
	}
	for variant := range 8 {
		pending := &syncResponse{revision: response.revision, cursor: bytes.Clone(response.cursor), snapshot: response.snapshot}
		for _, change := range response.changes {
			pending.changes = append(pending.changes, change.CloneVT())
		}
		switch variant {
		case 1:
			pending.changes[0].Signature.SigData = make([]byte, maxHistoryPageBytes)
		case 2:
			pending.changes[1].Signature.SigData = make([]byte, maxHistoryPageBytes)
		case 3:
			pending.changes = pending.changes[:maxHistoryPageEntries]
		case 4:
			for _, change := range pending.changes[:4] {
				change.Signature.SigData = make([]byte, 400*1024)
			}
		case 7:
			pending.snapshot = nil
		case 5, 6:
			message := &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{
				Revision: pending.revision, Cursor: pending.cursor, Changes: pending.changes[:1],
			}}}
			for range 3 {
				padding := len(pending.changes[0].Signature.SigData) + maxHistoryPageBytes + variant - 5 - message.SizeVT()
				pending.changes[0].Signature.SigData = make([]byte, padding)
			}
			if message.SizeVT() != maxHistoryPageBytes+variant-5 {
				t.Fatal("failed to construct the exact frame-size boundary")
			}
		}
		for step := range 6 {
			var arena fastjson.Arena
			request := arena.NewObject()
			request.Set("op", arena.NewString("nextSyncMessage"))
			request.Set("before", leanSyncResponse(t, &arena, pending))
			sizes := arena.NewArray()
			for count := 1; count <= len(pending.changes); count++ {
				message := &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{
					Revision: pending.revision, Cursor: pending.cursor, Changes: pending.changes[:count],
				}}}
				sizes.SetArrayItem(count-1, arena.NewNumberInt(message.SizeVT()))
			}
			request.Set("sizes", sizes)
			message, err := pending.nextMessage()
			expected, result := arena.NewObject(), arena.NewObject()
			expected.Set("ok", leanSyncBool(&arena, err == nil))
			result.Set("ok", leanSyncBool(&arena, err == nil))
			result.Set("state", leanSyncResponse(t, &arena, pending))
			kind := 0
			switch message.GetBody().(type) {
			case *SOSyncMessage_HistoryPage:
				kind = 9
			case *SOSyncMessage_Snapshot:
				kind = 1
			}
			result.Set("kind", arena.NewNumberInt(kind))
			result.Set("page", leanSyncPage(t, &arena, message))
			result.Set("snapshot", leanSyncSnapshot(&arena, message.GetSnapshot()))
			expected.Set("message", result)
			cases = append(cases, leanSyncCase{
				name:    "nextSyncMessage seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant) + " step " + strconv.Itoa(step),
				request: request.MarshalTo(nil), expected: expected.MarshalTo(nil),
			})
			if err != nil || kind == 1 {
				break
			}
		}
	}
	return cases
}
