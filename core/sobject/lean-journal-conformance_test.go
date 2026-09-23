package sobject

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
)

// TestLeanJournalConformance compares reducer transitions and compact checkpoint replay.
func TestLeanJournalConformance(t *testing.T) {
	oracle := leanOracle(t)
	var cases []leanCase
	for seed := range uint64(30) {
		cases = append(cases, runLeanJournalScenario(t, seed)...)
	}
	checkLeanCases(t, oracle, cases)
}

// FuzzLeanJournal searches transition, identity and evidence combinations.
func FuzzLeanJournal(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(17))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanCases(t, leanOracle(t), runLeanJournalScenario(t, seed))
	})
}

// runLeanJournalScenario constructs real encrypted intents and every reducer transition kind.
func runLeanJournalScenario(t *testing.T, seed uint64) []leanCase {
	t.Helper()
	scope := testScope("lean journal " + strconv.FormatUint(seed, 10))
	crypto := testJournalCrypto(t, scope)
	key := testMutationKey(scope, "peer", "attempt")
	lineage := testLineage(key, nil)
	version := JournalVersion(seed%19+1, seed%23+1, seed%7+1, testDigest("config"))
	intent := testIntent(t, crypto, key, lineage, version, 1, "opaque operation")
	decoded := testDecodedIntent(t, crypto, intent, key, lineage, version, 1)
	envelope, err := NewJournalEnvelopeRecord(crypto, 2, decoded, []byte("signed envelope"), journalDefaultIdentity())
	if err != nil {
		t.Fatal(err)
	}
	sent := newJournalSentRecord(key, lineage, version)
	receipt := testReceipt(key, envelope.GetEnvelopeDigest(), []byte("terminal receipt"), seed%31+1, testDigest("root"))
	ack := &SOJournalAcknowledgement{Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(), AcknowledgedUnixMillis: seed}
	projection := &SOJournalProjection{
		Key: key.CloneVT(), ReceiptDigest: receipt.GetTerminalReceiptDigest(),
		AuthoritativeRootSeqno: receipt.GetAuthoritativeRootSeqno(), AuthoritativeRootDigest: receipt.GetAuthoritativeRootDigest(),
	}
	lookup := &SOJournalLookup{
		Key: key.CloneVT(), State: SOReceiptState_SO_RECEIPT_STATE_NO_RECORD,
		Response: []byte("no record"), ResponseDigest: testDigest("no record"), ConfigChainDigest: testDigest("config"),
	}
	pending := lookup.CloneVT()
	pending.State = SOReceiptState_SO_RECEIPT_STATE_PENDING
	terminal := lookup.CloneVT()
	terminal.State, terminal.Receipt = SOReceiptState_SO_RECEIPT_STATE_ACCEPTED, receipt.CloneVT()
	corpus := []*SOJournalRecord{
		intent, envelope, sent, NewJournalReceiptRecord(key, lineage, version, receipt),
		NewJournalAcknowledgementRecord(key, lineage, version, ack), NewJournalProjectionRecord(key, lineage, version, projection),
		NewJournalStaleEpochRecord(key, lineage, version), NewJournalRecoveryBlockedRecord(key, lineage, version, SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE),
		NewJournalLineageRecoveryBlockedRecord(key, lineage, version), NewJournalReceiptLookupRecord(key, lineage, version, lookup),
		NewJournalResendAuthorizedRecord(key, lineage, version), NewJournalReceiptLookupRecord(key, lineage, version, pending),
		NewJournalReceiptLookupRecord(key, lineage, version, terminal),
	}
	prefixes := [][]*SOJournalRecord{
		nil,
		{intent},
		{intent, envelope},
		{intent, envelope, sent},
		{intent, envelope, sent, corpus[11]},
		{intent, envelope, sent, corpus[9]},
		{intent, envelope, sent, corpus[9], corpus[10]},
		{intent, envelope, sent, corpus[9], corpus[10], sent},
		{intent, envelope, sent, corpus[3]},
		{intent, envelope, sent, corpus[3], corpus[4], corpus[5]},
		{intent, envelope, corpus[6], corpus[8]},
		{intent, corpus[7]},
		{intent, envelope, sent, corpus[12]},
	}
	for readiness := SOJournalReadiness_SO_JOURNAL_READINESS_MISSING; readiness <= SOJournalReadiness_SO_JOURNAL_READINESS_OBSOLETE; readiness++ {
		unready := intent.CloneVT()
		unready.Readiness = readiness
		blocked := NewJournalRecoveryBlockedRecord(key, lineage, version, SOJournalRecoveryReason(int32(readiness)+2))
		prefixes = append(prefixes, []*SOJournalRecord{unready}, []*SOJournalRecord{unready, blocked})
	}
	other := testMutationKey(scope, "another peer", "attempt")
	otherIntent := testIntent(t, crypto, other, testLineage(other, nil), version, 2, "another operation")
	prefixes = append(prefixes, []*SOJournalRecord{intent, otherIntent},
		[]*SOJournalRecord{intent, envelope, sent, corpus[9], corpus[10], sent, corpus[3], corpus[4], corpus[5]})
	var cases []leanCase
	for index, source := range prefixes {
		records := make([]*SOJournalRecord, len(source))
		for i, record := range source {
			records[i] = record.CloneVT()
			records[i].Sequence = uint64(i + 1)
		}
		reducer, err := ReduceJournal(records)
		if err != nil {
			t.Fatalf("valid prefix %d: %v", index, err)
		}
		name := " seed " + strconv.FormatUint(seed, 10) + " prefix " + strconv.Itoa(index)
		cases = append(cases, leanJournalReplayCase(t, records, "replayJournal"+name))
		cases = append(cases, leanJournalSuffixCases(t, crypto, records, name)...)
		for variant := range 47 {
			record := corpus[variant%len(corpus)].CloneVT()
			record.Sequence = uint64(len(records) + 1)
			switch variant {
			case 13:
				record = nil
			case 14:
				record.FormatVersion = 99
			case 15:
				record.Sequence = 0
			case 16:
				record.Kind = SOJournalRecordKind(99)
			case 17:
				record.Key = nil
			case 18:
				record.Key.OriginScopeId = nil
			case 19:
				record.Key.ParticipantPeerId = "other peer"
			case 20:
				record.Lineage = nil
			case 21:
				record.Lineage.RootKey.LocalId = "other attempt"
			case 22:
				record.Version = nil
			case 23:
				record.Version.LocalVersion++
			case 24:
				record.AttemptState = SOJournalAttemptState(99)
			case 25:
				record = intent.CloneVT()
				record.Sequence = uint64(len(records) + 1)
				record.Readiness = SOJournalReadiness(99)
			case 26:
				record.Intent.Nonce = nil
			case 27:
				record.Envelope.Ciphertext = nil
			case 28:
				record.EnvelopeDigest = nil
			case 29:
				record.Receipt.TerminalReceipt = []byte("changed receipt")
			case 30:
				record.Acknowledgement.ReceiptDigest = testDigest("foreign receipt")
			case 31:
				record.Projection.AuthoritativeRootSeqno++
			case 32:
				record.RecoveryReason = SOJournalRecoveryReason(99)
			case 33:
				record.RecoveryReason = SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_BODY_MISSING
			case 34:
				record.RecoveryReason = SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH
			case 35:
				record.Lookup.Response = []byte("changed response")
			case 36:
				record.Lineage.Supersedes = key.CloneVT()
			case 37:
				record.Lookup.State = SOReceiptState(99)
			case 38:
				record.Lookup.Receipt.EnvelopeDigest = testDigest("foreign envelope")
			case 39:
				record.Key.LocalId, record.Lineage.RootKey.LocalId = "successor", "successor"
				record.Lineage.Supersedes = key.CloneVT()
			case 40:
				record.Version.ConfigChainDigest = testDigest("different config")
			case 41:
				record.Sequence += 2
			case 42:
				record.Receipt.Outcome = SOJournalOutcome(99)
			case 43:
				record.Acknowledgement.AcknowledgedUnixMillis++
			case 44:
				record.Projection.AuthoritativeRootDigest = testDigest("different root")
			case 45:
				record.RecoveryReason = SOJournalRecoveryReason_SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE
			case 46:
				encoded := append(mustMarshalVT(t, record.Version), 0xc0, 0x3e, 1)
				if err := record.Version.UnmarshalVT(encoded); err != nil {
					t.Fatal(err)
				}
			}
			if index == 0 {
				request := map[string]any{"op": "validateJournalRecord", "record": projectLeanJournalRecord(t, record)}
				cases = append(cases, leanCase{name: "validateJournalRecord" + name + " variant " + strconv.Itoa(variant), request: marshalLeanJournal(t, request), ok: validateJournalRecord(record) == nil})
			}
			candidate := reducer.Clone()
			before := candidate.Snapshot()
			err := candidate.Apply(record)
			var result any
			if err == nil {
				result = projectLeanJournalState(t, candidate.Snapshot())
			} else if !reflect.DeepEqual(before, candidate.Snapshot()) {
				t.Fatal("rejected journal transition changed reducer state")
			}
			request := map[string]any{"op": "applyJournal", "state": projectLeanJournalState(t, before), "record": projectLeanJournalRecord(t, record)}
			caseName := name + " variant " + strconv.Itoa(variant)
			cases = append(cases, leanCase{name: "applyJournal" + caseName, request: marshalLeanJournal(t, request), ok: err == nil, field: "state", value: marshalLeanJournal(t, result)})
			cases = append(cases, leanJournalReplayCase(t, append(append([]*SOJournalRecord(nil), records...), record), "replayJournal"+caseName))
		}
		cases = append(cases, leanJournalCheckpointCases(t, reducer.Snapshot(), name)...)
		identity, generation, nextSequence := journalDefaultIdentity(), seed%13+1, uint64(len(records)+1)
		encoded, err := marshalCompactJournalCheckpoint(identity, generation, nextSequence, reducer)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := unmarshalCompactJournalCheckpoint(encoded, identity, generation, nextSequence)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, leanJournalCheckpointReadCases(t, encoded, identity, generation, nextSequence, name)...)
		if !reflect.DeepEqual(reducer.Snapshot(), restored.Snapshot()) {
			t.Fatalf("checkpoint changed prefix %d", index)
		}
		request := map[string]any{
			"op": "journalCheckpoint", "identity": hex.EncodeToString(identity), "generation": generation,
			"nextSequence": nextSequence, "state": projectLeanJournalState(t, reducer.Snapshot()),
		}
		cases = append(cases, leanCase{name: "journalCheckpoint" + name, request: marshalLeanJournal(t, request), ok: true, field: "state", value: marshalLeanJournal(t, projectLeanJournalState(t, restored.Snapshot()))})
	}
	return cases
}

// leanJournalReplayCase runs the real contiguous replay contract before projecting its result.
func leanJournalReplayCase(t *testing.T, records []*SOJournalRecord, name string) leanCase {
	t.Helper()
	projected := make([]any, len(records))
	for i, record := range records {
		projected[i] = projectLeanJournalRecord(t, record)
	}
	reducer, err := ReduceJournal(records)
	var result any
	if err == nil {
		result = projectLeanJournalState(t, reducer.Snapshot())
	}
	return leanCase{name: name, request: marshalLeanJournal(t, map[string]any{"op": "replayJournal", "records": projected}), ok: err == nil, field: "state", value: marshalLeanJournal(t, result)}
}

// marshalLeanJournal encodes exact integers and primitive projection fields without float conversion.
func marshalLeanJournal(t *testing.T, value any) []byte {
	t.Helper()
	var arena fastjson.Arena
	return leanJournalJSON(t, &arena, value).MarshalTo(nil)
}

// leanJournalJSON encodes the finite set of primitive journal projection types.
func leanJournalJSON(t *testing.T, arena *fastjson.Arena, value any) *fastjson.Value {
	t.Helper()
	switch value := value.(type) {
	case nil:
		return arena.NewNull()
	case string:
		return arena.NewString(value)
	case bool:
		return leanBool(arena, value)
	case uint64:
		return arena.NewNumberString(strconv.FormatUint(value, 10))
	case uint32:
		return arena.NewNumberString(strconv.FormatUint(uint64(value), 10))
	case int64:
		return arena.NewNumberString(strconv.FormatInt(value, 10))
	case int32:
		return arena.NewNumberString(strconv.FormatInt(int64(value), 10))
	case []any:
		result := arena.NewArray()
		for i, element := range value {
			result.SetArrayItem(i, leanJournalJSON(t, arena, element))
		}
		return result
	case map[string]any:
		result := arena.NewObject()
		for key, element := range value {
			result.Set(key, leanJournalJSON(t, arena, element))
		}
		return result
	default:
		t.Fatalf("unsupported journal projection type %T", value)
		return nil
	}
}

// projectLeanJournalKey preserves semantic identity and independently computes its digest.
func projectLeanJournalKey(key *SOMutationKey) any {
	if key == nil {
		return nil
	}
	digest, _ := MutationKeyDigest(key)
	return map[string]any{"origin": hex.EncodeToString(key.GetOriginScopeId()), "object": key.GetSharedObjectId(), "peer": key.GetParticipantPeerId(), "localID": key.GetLocalId(), "digest": hex.EncodeToString(digest)}
}

// projectLeanJournalLineage preserves omitted predecessor and root keys.
func projectLeanJournalLineage(lineage *SOJournalLineage) any {
	if lineage == nil {
		return nil
	}
	return map[string]any{"root": projectLeanJournalKey(lineage.GetRootKey()), "supersedes": projectLeanJournalKey(lineage.GetSupersedes())}
}

// projectLeanJournalVersion retains exact protobuf identity for immutable version comparisons.
func projectLeanJournalVersion(t *testing.T, version *SOJournalVersionTuple) any {
	t.Helper()
	if version == nil {
		return nil
	}
	return map[string]any{"data": hex.EncodeToString(mustMarshalVT(t, version)), "localVersion": version.GetLocalVersion(), "remoteVersion": version.GetRemoteVersion(), "transformEpoch": version.GetTransformEpoch(), "configDigest": hex.EncodeToString(version.GetConfigChainDigest())}
}

// projectLeanJournalPayload retains actual nonce and ciphertext lengths and bytes.
func projectLeanJournalPayload(payload *SOJournalEncryptedPayload) any {
	if payload == nil {
		return nil
	}
	return map[string]any{"nonce": hex.EncodeToString(payload.GetNonce()), "ciphertext": hex.EncodeToString(payload.GetCiphertext())}
}

// projectLeanJournalReceipt supplies only a primitive hash, leaving receipt admission to Lean.
func projectLeanJournalReceipt(t *testing.T, receipt *SOJournalReceipt) any {
	t.Helper()
	if receipt == nil {
		return nil
	}
	hash := sha256.Sum256(receipt.GetTerminalReceipt())
	return map[string]any{
		"data": hex.EncodeToString(mustMarshalVT(t, receipt)), "key": projectLeanJournalKey(receipt.GetKey()), "supersedes": projectLeanJournalKey(receipt.GetSupersedes()),
		"envelopeDigest": hex.EncodeToString(receipt.GetEnvelopeDigest()), "outcome": int32(receipt.GetOutcome()),
		"terminal": hex.EncodeToString(receipt.GetTerminalReceipt()), "terminalDigest": hex.EncodeToString(receipt.GetTerminalReceiptDigest()), "terminalHash": hex.EncodeToString(hash[:]),
		"rootSeqno": receipt.GetAuthoritativeRootSeqno(), "rootDigest": hex.EncodeToString(receipt.GetAuthoritativeRootDigest()), "configDigest": hex.EncodeToString(receipt.GetConfigChainDigest()), "terminalTime": receipt.GetTerminalUnixMillis(),
	}
}

// projectLeanJournalLookup independently hashes the actual retained lookup response.
func projectLeanJournalLookup(t *testing.T, lookup *SOJournalLookup) any {
	t.Helper()
	if lookup == nil {
		return nil
	}
	hash := sha256.Sum256(lookup.GetResponse())
	return map[string]any{
		"data": hex.EncodeToString(mustMarshalVT(t, lookup)), "key": projectLeanJournalKey(lookup.GetKey()), "state": int32(lookup.GetState()), "receipt": projectLeanJournalReceipt(t, lookup.GetReceipt()),
		"response": hex.EncodeToString(lookup.GetResponse()), "responseDigest": hex.EncodeToString(lookup.GetResponseDigest()), "responseHash": hex.EncodeToString(hash[:]), "configDigest": hex.EncodeToString(lookup.GetConfigChainDigest()),
	}
}

// projectLeanJournalAcknowledgement retains exact immutable acknowledgement bytes.
func projectLeanJournalAcknowledgement(t *testing.T, ack *SOJournalAcknowledgement) any {
	t.Helper()
	if ack == nil {
		return nil
	}
	return map[string]any{"data": hex.EncodeToString(mustMarshalVT(t, ack)), "key": projectLeanJournalKey(ack.GetKey()), "receiptDigest": hex.EncodeToString(ack.GetReceiptDigest()), "time": ack.GetAcknowledgedUnixMillis()}
}

// projectLeanJournalProjection retains the exact receipt and authoritative root observation.
func projectLeanJournalProjection(t *testing.T, projection *SOJournalProjection) any {
	t.Helper()
	if projection == nil {
		return nil
	}
	return map[string]any{"data": hex.EncodeToString(mustMarshalVT(t, projection)), "key": projectLeanJournalKey(projection.GetKey()), "receiptDigest": hex.EncodeToString(projection.GetReceiptDigest()), "rootSeqno": projection.GetAuthoritativeRootSeqno(), "rootDigest": hex.EncodeToString(projection.GetAuthoritativeRootDigest())}
}

// projectLeanJournalRecord projects each independently validated field without calling admission.
func projectLeanJournalRecord(t *testing.T, record *SOJournalRecord) any {
	t.Helper()
	if record == nil {
		return nil
	}
	return map[string]any{
		"format": record.GetFormatVersion(), "sequence": record.GetSequence(), "kind": int32(record.GetKind()),
		"key": projectLeanJournalKey(record.GetKey()), "lineage": projectLeanJournalLineage(record.GetLineage()), "version": projectLeanJournalVersion(t, record.GetVersion()),
		"state": int32(record.GetAttemptState()), "readiness": int32(record.GetReadiness()), "intent": projectLeanJournalPayload(record.GetIntent()), "envelope": projectLeanJournalPayload(record.GetEnvelope()), "envelopeDigest": hex.EncodeToString(record.GetEnvelopeDigest()),
		"receipt": projectLeanJournalReceipt(t, record.GetReceipt()), "acknowledgement": projectLeanJournalAcknowledgement(t, record.GetAcknowledgement()), "projection": projectLeanJournalProjection(t, record.GetProjection()), "lookup": projectLeanJournalLookup(t, record.GetLookup()), "recoveryReason": int32(record.GetRecoveryReason()),
	}
}

// projectLeanJournalState preserves Snapshot's digest ordering and every retained attempt field.
func projectLeanJournalState(t *testing.T, snapshots []*JournalAttemptSnapshot) []any {
	t.Helper()
	result := make([]any, len(snapshots))
	for i, attempt := range snapshots {
		result[i] = projectLeanJournalAttempt(t, attempt)
	}
	return result
}

// projectLeanJournalAttempt preserves optional evidence and bounded lookup-history entries.
func projectLeanJournalAttempt(t *testing.T, attempt *JournalAttemptSnapshot) any {
	t.Helper()
	history := make([]any, len(attempt.LookupHistory))
	for j, lookup := range attempt.LookupHistory {
		history[j] = projectLeanJournalLookup(t, lookup)
	}
	return map[string]any{
		"key": projectLeanJournalKey(attempt.Key), "lineage": projectLeanJournalLineage(attempt.Lineage), "version": projectLeanJournalVersion(t, attempt.Version), "state": int32(attempt.State), "readiness": int32(attempt.Readiness),
		"intentSequence": attempt.IntentSequence, "envelopeSequence": attempt.EnvelopeSequence, "intent": projectLeanJournalPayload(attempt.Intent), "envelope": projectLeanJournalPayload(attempt.Envelope), "envelopeDigest": hex.EncodeToString(attempt.EnvelopeDigest),
		"receipt": projectLeanJournalReceipt(t, attempt.Receipt), "acknowledgement": projectLeanJournalAcknowledgement(t, attempt.Acknowledgement), "projection": projectLeanJournalProjection(t, attempt.Projection), "lookup": projectLeanJournalLookup(t, attempt.Lookup), "lookupHistory": history,
		"sendAttempted": attempt.SendAttempted, "resendAuthorized": attempt.ResendAuthorized, "lineageRecoveryBlocked": attempt.LineageRecoveryBlocked, "checkpointEligible": attempt.CheckpointEligible,
	}
}

// leanJournalCheckpointCases compares independent checkpoint field corruption with Go admission.
func leanJournalCheckpointCases(t *testing.T, snapshots []*JournalAttemptSnapshot, name string) []leanCase {
	t.Helper()
	if len(snapshots) == 0 {
		return nil
	}
	var cases []leanCase
	for variant := range 28 {
		attempt := cloneJournalAttempt(snapshots[0])
		switch variant {
		case 1:
			attempt.Key = nil
		case 2:
			attempt.Lineage = nil
		case 3:
			attempt.Version = nil
		case 4:
			attempt.State = SOJournalAttemptState(99)
		case 5:
			attempt.Readiness = SOJournalReadiness(99)
		case 6:
			attempt.LookupHistory = []*SOJournalLookup{nil, nil}
		case 7:
			attempt.LookupHistory = []*SOJournalLookup{nil}
		case 8:
			attempt.Intent = nil
		case 9:
			attempt.Intent.Nonce = nil
		case 10:
			attempt.IntentSequence = 0
		case 11:
			attempt.Envelope = nil
		case 12:
			attempt.EnvelopeSequence = attempt.IntentSequence
		case 13:
			attempt.EnvelopeDigest = nil
		case 14:
			attempt.Receipt = nil
		case 15:
			attempt.Acknowledgement = &SOJournalAcknowledgement{Key: attempt.Key.CloneVT(), ReceiptDigest: testDigest("wrong receipt")}
		case 16:
			attempt.Projection = &SOJournalProjection{Key: attempt.Key.CloneVT(), ReceiptDigest: testDigest("wrong receipt"), AuthoritativeRootDigest: testDigest("root")}
		case 17:
			attempt.ResendAuthorized = !attempt.ResendAuthorized
		case 18:
			attempt.LineageRecoveryBlocked = !attempt.LineageRecoveryBlocked
		case 19:
			attempt.SendAttempted = !attempt.SendAttempted
		case 20:
			attempt.CheckpointEligible = !attempt.CheckpointEligible
		case 21:
			attempt.Lookup = nil
		case 22:
			attempt.LookupHistory = nil
		case 23:
			if attempt.Receipt != nil {
				attempt.Receipt.TerminalReceipt = []byte("corrupt receipt")
			}
		case 24:
			if attempt.Lookup != nil {
				attempt.Lookup.Response = []byte("corrupt response")
			}
		case 25:
			attempt.Key.LocalId = "other attempt"
		case 26:
			attempt.Lineage.Supersedes = attempt.Key.CloneVT()
		case 27:
			attempt.Intent.Ciphertext = nil
		}
		request := map[string]any{"op": "validateJournalCheckpoint", "attempt": projectLeanJournalAttempt(t, attempt)}
		cases = append(cases, leanCase{name: "validateJournalCheckpoint" + name + " variant " + strconv.Itoa(variant), request: marshalLeanJournal(t, request), ok: validateCheckpointAttempt(attempt) == nil})
	}
	return cases
}

// leanJournalCheckpointReadCases corrupts real compact checkpoint bytes and expected metadata.
func leanJournalCheckpointReadCases(t *testing.T, encoded, identity []byte, generation, nextSequence uint64, name string) []leanCase {
	t.Helper()
	var cases []leanCase
	for variant := range 9 {
		checkpoint := &SOJournalCheckpoint{}
		if err := checkpoint.UnmarshalVT(encoded); err != nil {
			t.Fatal(err)
		}
		wantIdentity, wantGeneration, wantSequence := identity, generation, nextSequence
		switch variant {
		case 1:
			wantIdentity = testDigest("foreign journal")
		case 2:
			wantGeneration++
		case 3:
			wantSequence++
		case 4:
			if len(checkpoint.Attempts) != 0 {
				checkpoint.Attempts = append(checkpoint.Attempts, checkpoint.Attempts[0].CloneVT())
			}
		case 5:
			checkpoint.Attempts = append(checkpoint.Attempts, &SOJournalCheckpointAttempt{})
		case 6:
			checkpoint.Generation, wantGeneration = 0, 0
		case 7:
			checkpoint.NextSequence, wantSequence = 0, 0
		}
		data := mustMarshalVT(t, checkpoint)
		if variant == 8 {
			data = []byte{0xff}
		}
		actual, err := unmarshalCompactJournalCheckpoint(data, wantIdentity, wantGeneration, wantSequence)
		var result any
		if err == nil {
			result = projectLeanJournalState(t, actual.Snapshot())
		}
		request := map[string]any{
			"op": "readJournalCheckpoint", "checkpoint": projectLeanJournalCheckpoint(t, data),
			"identity": hex.EncodeToString(wantIdentity), "generation": wantGeneration, "nextSequence": wantSequence,
		}
		cases = append(cases, leanCase{name: "readJournalCheckpoint" + name + " variant " + strconv.Itoa(variant), request: marshalLeanJournal(t, request), ok: err == nil, field: "state", value: marshalLeanJournal(t, result)})
	}
	return cases
}

// projectLeanJournalCheckpoint decodes only protobuf bytes, independently of checkpoint admission.
func projectLeanJournalCheckpoint(t *testing.T, data []byte) any {
	t.Helper()
	checkpoint := &SOJournalCheckpoint{}
	if checkpoint.UnmarshalVT(data) != nil {
		return nil
	}
	attempts := make([]any, len(checkpoint.GetAttempts()))
	for i, attempt := range checkpoint.GetAttempts() {
		snapshot := decodeLeanCheckpointAttempt(attempt)
		attempts[i] = projectLeanJournalAttempt(t, snapshot)
	}
	return map[string]any{"identity": hex.EncodeToString(checkpoint.GetJournalIdentity()), "generation": checkpoint.GetGeneration(), "nextSequence": checkpoint.GetNextSequence(), "attempts": attempts}
}

// decodeLeanCheckpointAttempt preserves raw protobuf fields without checkpoint admission.
func decodeLeanCheckpointAttempt(attempt *SOJournalCheckpointAttempt) *JournalAttemptSnapshot {
	return &JournalAttemptSnapshot{
		Key: attempt.GetKey(), Lineage: attempt.GetLineage(), Version: attempt.GetVersion(),
		State: attempt.GetState(), Readiness: attempt.GetReadiness(),
		IntentSequence: attempt.GetIntentSequence(), EnvelopeSequence: attempt.GetEnvelopeSequence(),
		Intent: attempt.GetIntent(), Envelope: attempt.GetEnvelope(), EnvelopeDigest: attempt.GetEnvelopeDigest(),
		Receipt: attempt.GetReceipt(), Acknowledgement: attempt.GetAcknowledgement(), Projection: attempt.GetProjection(), Lookup: attempt.GetLookup(),
		SendAttempted: attempt.GetSendAttempted(), ResendAuthorized: attempt.GetResendAuthorized(),
		LineageRecoveryBlocked: attempt.GetLineageRecoveryBlocked(), CheckpointEligible: attempt.GetCheckpointEligible(),
	}
}

// leanJournalSuffixCases compacts and reopens real Go journals at every record boundary.
func leanJournalSuffixCases(t *testing.T, crypto *JournalCrypto, records []*SOJournalRecord, name string) []leanCase {
	t.Helper()
	var cases []leanCase
	for cut := 0; cut <= len(records); cut++ {
		pipeline := testPipeline(t, crypto)
		for _, record := range records[:cut] {
			if err := pipeline.appendRecord(record); err != nil {
				t.Fatal(err)
			}
		}
		if err := pipeline.journal.checkpoint(); err != nil {
			t.Fatal(err)
		}
		restored, err := OpenJournalPipelineWithCrypto(pipeline.journal.writer.storage, crypto, testReceiptVerifier(), testLookupVerifier())
		if err != nil {
			t.Fatal(err)
		}
		checkpoint := projectLeanJournalState(t, restored.Snapshot())
		sequence := restored.journal.nextSequence()
		suffix := make([]any, len(records)-cut)
		for i, record := range records[cut:] {
			suffix[i] = projectLeanJournalRecord(t, record)
			if err := restored.appendRecord(record); err != nil {
				t.Fatal(err)
			}
		}
		full, err := ReduceJournal(records)
		if err != nil || !reflect.DeepEqual(full.Snapshot(), restored.Snapshot()) {
			t.Fatalf("checkpoint plus suffix differs from full replay at cut %d: %v", cut, err)
		}
		request := map[string]any{"op": "replayJournalFrom", "state": checkpoint, "sequence": sequence, "records": suffix}
		cases = append(cases, leanCase{name: "replayJournalFrom" + name + " cut " + strconv.Itoa(cut), request: marshalLeanJournal(t, request), ok: true, field: "state", value: marshalLeanJournal(t, projectLeanJournalState(t, restored.Snapshot()))})
	}
	return cases
}
