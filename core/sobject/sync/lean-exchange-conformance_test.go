package sobject_sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// leanSyncExchangeFrame projects body tags and protobuf getter defaults without validating protocol state.
func leanSyncExchangeFrame(t *testing.T, a *fastjson.Arena, message *SOSyncMessage) *fastjson.Value {
	t.Helper()
	if message == nil {
		return a.NewNull()
	}
	kind := leanSyncWriterKind(message)
	switch message.GetBody().(type) {
	case *SOSyncMessage_Op:
		kind = 2
	case *SOSyncMessage_Challenge:
		kind = 4
	case *SOSyncMessage_Proof:
		kind = 5
	case *SOSyncMessage_HistoryRequest:
		kind = 8
	}
	v := a.NewObject()
	v.Set("kind", a.NewNumberInt(kind))
	v.Set("head", leanSyncHead(a, message.GetHead()))
	v.Set("hashBytes", a.NewNumberInt(len(message.GetHead().GetConfigHash())))
	v.Set("stateHashBytes", a.NewNumberInt(len(message.GetHead().GetStateHash())))
	request := a.NewObject()
	request.Set("revision", a.NewNumberString(strconv.FormatUint(message.GetHistoryRequest().GetRevision(), 10)))
	request.Set("base", a.NewString(hex.EncodeToString(message.GetHistoryRequest().GetBaseHash())))
	v.Set("request", request)
	v.Set("baseBytes", a.NewNumberInt(len(message.GetHistoryRequest().GetBaseHash())))
	v.Set("page", leanSyncPage(t, a, message))
	v.Set("snapshot", leanSyncSnapshot(a, message.GetSnapshot()))
	revision := message.GetAck().GetRevision()
	if message.GetRecoveryRequired() != nil {
		revision = message.GetRecoveryRequired().GetRevision()
	}
	v.Set("revision", a.NewNumberString(strconv.FormatUint(revision, 10)))
	v.Set("accepted", leanSyncBool(a, message.GetAuthorization().GetAccepted()))
	v.Set("recoveryPresent", leanSyncBool(a, message.GetRecoveryRequired() != nil))
	return v
}

// leanSyncClockOrigin preserves monotonic comparisons for process-local sync deadlines.
var leanSyncClockOrigin = time.Now()

// leanSyncTime distinguishes zero time from an offset on the process's monotonic clock.
func leanSyncTime(a *fastjson.Arena, value time.Time) *fastjson.Value {
	if value.IsZero() {
		return a.NewNull()
	}
	return a.NewNumberString(strconv.FormatInt(value.Sub(leanSyncClockOrigin).Nanoseconds(), 10))
}

// leanSyncExchange projects the state held exclusively by the production loop.
func leanSyncExchange(t *testing.T, a *fastjson.Arena, x *syncExchange) *fastjson.Value {
	t.Helper()
	v := a.NewObject()
	for name, state := range map[string]*sobject.SOState{"advertised": x.advertised, "lastAdvertised": x.lastAdvertised} {
		projected := a.NewNull()
		if state != nil {
			projected = leanSyncState(t, a, x.sync.soID, state)
		}
		v.Set(name, projected)
	}
	v.Set("revision", a.NewNumberString(strconv.FormatUint(x.revision, 10)))
	v.Set("remoteRevision", a.NewNumberString(strconv.FormatUint(x.remoteRevision, 10)))
	v.Set("advertisementDeadline", leanSyncTime(a, x.advertisementDeadline))
	v.Set("requested", leanSyncBool(a, x.requested))
	receiving, deadline := a.NewNull(), a.NewNull()
	if x.receiving != nil {
		receiving = leanSyncReceive(t, a, x.receiving)
		deadline = leanSyncTime(a, x.receiving.deadline)
	}
	v.Set("receiving", receiving)
	v.Set("receiveDeadline", deadline)
	response := a.NewNull()
	if x.response != nil {
		response = leanSyncResponse(t, a, x.response)
	}
	v.Set("response", response)
	v.Set("control", leanSyncExchangeFrame(t, a, x.control))
	v.Set("outgoing", leanSyncExchangeFrame(t, a, x.outgoing))
	v.Set("inFlight", leanSyncExchangeFrame(t, a, x.inFlight))
	v.Set("terminal", leanSyncBool(a, x.terminal != nil))
	return v
}

// leanSyncExchangeCheck compares one real owner operation and its observable host effects.
func leanSyncExchangeCheck(t *testing.T, x *syncExchange, current *sobject.SOState,
	message *SOSyncMessage, pump bool, writes *int, name string, loop *leanSyncLoopRun,
) leanSyncCase {
	t.Helper()
	var a fastjson.Arena
	before := leanSyncExchange(t, &a, x)
	previous, err := x.sync.soHost.GetHostState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var admission, recovery *bool
	x.sync.peerAdmission = func(_ peer.ID, value bool) { admission = &value }
	x.sync.peerRecovery = func(_ peer.ID, value bool) { recovery = &value }
	input := a.NewObject()
	input.Set("now", a.NewNumberInt(0))
	input.Set("sameCurrent", leanSyncBool(&a, current.EqualVT(x.lastAdvertised)))
	input.Set("currentHashBytes", a.NewNumberInt(len(current.GetConfig().GetConfigChainHash())))
	wire := current.CloneVT()
	if wire == nil {
		wire = &sobject.SOState{}
	}
	wire.Invites, wire.QueuedAccountNonces = nil, nil
	encoded := leanSyncMarshal(t, wire)
	digest := sha256.Sum256(encoded)
	input.Set("stateBytes", a.NewNumberInt(wire.SizeVT()))
	input.Set("encoded", a.NewString(hex.EncodeToString(encoded)))
	input.Set("digest", a.NewString(hex.EncodeToString(digest[:])))
	history := a.NewNull()
	advertisedEncoded := a.NewNull()
	snapshotBytes := 0
	if x.advertised != nil {
		entries, err := x.sync.soHost.ReadConfigHistory(t.Context(), message.GetHistoryRequest().GetBaseHash(), x.advertised.GetConfig().GetConfigChainHash())
		if err == nil {
			history = leanSyncChanges(t, &a, entries)
		}
		advertised := x.advertised.CloneVT()
		advertised.Invites, advertised.QueuedAccountNonces = nil, nil
		data := leanSyncMarshal(t, advertised)
		advertisedEncoded = a.NewString(hex.EncodeToString(data))
		snapshot := &SOSyncSnapshot{SoState: data, RootSeqno: advertised.GetRoot().GetInnerSeqno(),
			Revision: message.GetHistoryRequest().GetRevision(), BaseHash: message.GetHistoryRequest().GetBaseHash()}
		snapshotBytes = (&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: snapshot}}).SizeVT()
	}
	input.Set("history", history)
	input.Set("advertisedEncoded", advertisedEncoded)
	input.Set("snapshotBytes", a.NewNumberInt(snapshotBytes))
	sizes := a.NewArray()
	if x.response != nil {
		for count := 1; count <= len(x.response.changes) && count <= maxHistoryPageEntries; count++ {
			page := &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{
				Revision: x.response.revision, Cursor: x.response.cursor, Changes: x.response.changes[:count],
			}}}
			sizes.SetArrayItem(count-1, a.NewNumberInt(page.SizeVT()))
		}
	}
	input.Set("sizes", sizes)
	input.Set("admissionObserver", a.NewTrue())
	input.Set("recoveryObserver", a.NewTrue())
	accepted := a.NewObject()
	accepted.Set("previous", leanSyncState(t, &a, x.sync.soID, previous))
	receiving := x.receiving
	if receiving == nil {
		receiving = &syncReceive{}
	}
	accepted.Set("receiving", leanSyncReceive(t, &a, receiving))
	snapshot := message.GetSnapshot()
	accepted.Set("snapshot", leanSyncSnapshot(&a, snapshot))
	contentDigest := sha256.Sum256(snapshot.GetSoState())
	accepted.Set("digest", a.NewString(hex.EncodeToString(contentDigest[:])))
	candidate := &sobject.SOState{}
	decoded := a.NewNull()
	if candidate.UnmarshalVT(snapshot.GetSoState()) == nil {
		decoded = leanSyncState(t, &a, x.sync.soID, candidate)
	}
	accepted.Set("decoded", decoded)
	accepted.Set("candidateBytes", a.NewNumberInt(candidate.SizeVT()))
	accepted.Set("beforeRead", leanSyncState(t, &a, x.sync.soID, previous))
	accepted.Set("localPeer", a.NewString(x.sync.localObjectPeerID.String()))
	for _, field := range []string{"lockOK", "accessOK", "writeOK"} {
		accepted.Set(field, a.NewTrue())
	}
	input.Set("acceptance", accepted)
	oldAdvertisement := x.advertisementDeadline
	var oldReceive time.Time
	if x.receiving != nil {
		oldReceive = x.receiving.deadline
	}
	writeCount := *writes
	logger := gateLogger()
	var logs bytes.Buffer
	logger.Logger.SetOutput(&logs)
	start := time.Now()
	if loop != nil {
		err = loop.run(t, x, logger)
	} else if pump {
		err = x.prepareSend(current)
	} else {
		err = x.receive(t.Context(), logger, current, message)
	}
	finish := time.Now()

	// Validate clock observations independently before using the returned timestamp in the oracle.
	observeClock := func(deadline, old time.Time) *fastjson.Value {
		if deadline.IsZero() || deadline.Equal(old) {
			return a.NewNumberInt(0)
		}
		now := deadline.Add(-catchupTimeout)
		if now.Before(start) || now.After(finish) {
			t.Fatal("new pinned deadline is not one catch-up budget after the clock observation")
		}
		return a.NewNumberString(strconv.FormatInt(now.Sub(leanSyncClockOrigin).Nanoseconds(), 10))
	}
	preparationNow := observeClock(x.advertisementDeadline, oldAdvertisement)
	receptionNow := a.NewNumberInt(0)
	if x.receiving != nil {
		receptionNow = observeClock(x.receiving.deadline, oldReceive)
	}
	input.Set("now", receptionNow)
	if pump {
		input.Set("now", preparationNow)
	}
	after, readErr := x.sync.soHost.GetHostState(t.Context())
	if readErr != nil {
		t.Fatal(readErr)
	}
	accepted.Set("afterRead", leanSyncState(t, &a, x.sync.soID, after))
	input.Set("healthRead", leanSyncState(t, &a, x.sync.soID, after))
	request, expected, result := a.NewObject(), a.NewObject(), a.NewObject()
	request.Set("op", a.NewString("receiveSyncExchange"))
	if pump {
		request.Set("op", a.NewString("prepareSyncOutgoing"))
	}
	request.Set("before", before)
	projectedCurrent := a.NewNull()
	if current != nil {
		projectedCurrent = leanSyncState(t, &a, x.sync.soID, current)
	}
	request.Set("current", projectedCurrent)
	frame := leanSyncExchangeFrame(t, &a, message)
	if frame.Type() == fastjson.TypeNull {
		frame = leanSyncExchangeFrame(t, &a, &SOSyncMessage{})
	}
	request.Set("frame", frame)
	request.Set("input", input)
	if loop != nil {
		request.Set("op", a.NewString("advanceSyncExchange"))
		observation := a.NewObject()
		observation.Set("current", projectedCurrent)
		fresh := a.NewNull()
		if loop.next != nil {
			fresh = leanSyncState(t, &a, x.sync.soID, loop.next)
		}
		observation.Set("incomingCurrent", fresh)
		observation.Set("event", a.NewNumberInt(loop.event))
		observation.Set("eventOK", leanSyncBool(&a, loop.eventOK))
		drain := a.NewNull()
		if loop.drain != nil {
			drain = leanSyncBool(&a, *loop.drain)
		}
		observation.Set("drainAuthorization", drain)
		preparation, reception := a.NewObject(), a.NewObject()
		input.GetObject().Visit(func(key []byte, value *fastjson.Value) {
			preparation.Set(string(key), value)
			reception.Set(string(key), value)
		})
		preparation.Set("now", preparationNow)
		reception.Set("now", receptionNow)
		observation.Set("preparation", preparation)
		observation.Set("reception", reception)
		request.Set("loop", observation)
		request.Set("local", a.NewString(x.sync.localObjectPeerID.String()))
		request.Set("remote", a.NewString(x.remoteID.String()))
		expected.Set("denial", leanSyncBool(&a, loop.denial))
		expected.Set("handed", leanSyncExchangeFrame(t, &a, loop.handed))
	}
	result.Set("ok", leanSyncBool(&a, err == nil))
	result.Set("state", leanSyncExchange(t, &a, x))
	dispatched := strings.Contains(logs.String(), "failed to unmarshal remote op")
	result.Set("operation", leanSyncBool(&a, dispatched))
	for field, value := range map[string]*bool{"admission": admission, "recovery": recovery} {
		observation := a.NewNull()
		if value != nil {
			observation = leanSyncBool(&a, *value)
		}
		result.Set(field, observation)
	}
	expected.Set("ok", leanSyncBool(&a, err == nil))
	expected.Set("exchange", result)
	if !pump || loop != nil {
		host := a.NewNull()
		if !dispatched {
			host = a.NewObject()
			host.Set("state", leanSyncState(t, &a, x.sync.soID, after))
			host.Set("wrote", leanSyncBool(&a, *writes != writeCount))
			host.Set("revoked", leanSyncBool(&a, errors.Is(err, sobject.ErrParticipantRevoked)))
		}
		expected.Set("host", host)
	}
	return leanSyncCase{name: name, request: request.MarshalTo(nil), expected: expected.MarshalTo(nil)}
}

// TestLeanSyncExchangeConformance compares the real owner pump, dispatch and atomic publications.
func TestLeanSyncExchangeConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(4) {
		cases = append(cases, leanSyncExchangeCases(t, seed, false)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncExchange varies signed checkpoints and the exact negotiation branches.
func FuzzLeanSyncExchange(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(19))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanSync(t, leanSyncOracle(t), leanSyncExchangeCases(t, seed, false))
	})
}

// leanSyncExchangeCases exercises both directions using real signed retained history.
func leanSyncExchangeCases(t *testing.T, seed uint64, withLoop bool) []leanSyncCase {
	t.Helper()
	const soID = "lean-sync-exchange"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	sender := newAuthenticationPeer(t, soID, owner, initial)
	change, err := sobject.BuildSOConfigChange(initial.Config, initial.Config,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.soHost.ApplyConfigChange(t.Context(), change, nil); err != nil {
		t.Fatal(err)
	}
	target, err := sender.soHost.GetHostState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	target.Root.InnerSeqno = 2 + seed%4
	signSnapshotRoot(t, soID, target, owner)
	localID, err := peer.IDFromPrivateKey(reader)
	if err != nil {
		t.Fatal(err)
	}
	revision := seed%100 + 1
	request := &SOSyncHistoryRequest{Revision: revision, BaseHash: initial.Config.ConfigChainHash}
	prepared, err := sender.prepareResponse(t.Context(), target, request)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := syncStateHash(target)
	if err != nil {
		t.Fatal(err)
	}
	head := &SOSyncHead{Revision: revision, ConfigHash: target.Config.ConfigChainHash,
		ConfigSeqno: target.Config.ConfigChainSeqno, RootSeqno: target.Root.InnerSeqno, StateHash: digest}
	var cases []leanSyncCase
	variants := 55
	if withLoop {
		variants = 69
	}
	for variant := range variants {
		writes := 0
		host, ctr := newMemHost(soID, initial.CloneVT(), func() { writes++ })
		t.Cleanup(host.ClearContext)
		local := NewSOSync(gateLogger(), nil, soID, localID, reader, host, nil)
		x := &syncExchange{sync: local, remoteID: sender.localObjectPeerID, revision: revision}
		current := initial.CloneVT()
		response := &syncResponse{revision: revision, cursor: bytes.Clone(request.BaseHash),
			changes: []*sobject.SOConfigChange{change.CloneVT()}, snapshot: prepared.snapshot.CloneVT()}
		receiving := &syncReceive{head: head.CloneVT(), base: bytes.Clone(request.BaseHash), cursor: bytes.Clone(request.BaseHash),
			deadline: time.Now().Add(catchupTimeout)}
		complete := func() {
			receiving.cursor = bytes.Clone(target.Config.ConfigChainHash)
			receiving.changes = []*sobject.SOConfigChange{change.CloneVT()}
			receiving.size = change.SizeVT()
			x.receiving = receiving
		}
		advertise := func() {
			if err := host.ApplyConfigChange(t.Context(), change, nil); err != nil {
				t.Fatal(err)
			}
			ctr.SetValue(target.CloneVT())
			current = target.CloneVT()
			x.advertised = target.CloneVT()
			x.advertisementDeadline = time.Now().Add(catchupTimeout)
		}
		message := &SOSyncMessage{Body: &SOSyncMessage_Head{Head: head.CloneVT()}}
		pump := variant < 14
		switch variant {
		case 0:
			x.revision = 0
		case 1:
			x.outgoing = syncAcknowledgment(7)
		case 2:
			x.inFlight = syncAcknowledgment(7)
		case 3:
			x.control, x.response = syncAcknowledgment(7), response
		case 4:
			x.response = response
		case 5:
			response.changes = nil
			x.response = response
		case 6:
			response.changes, response.snapshot = nil, nil
			x.response = response
		case 7:
			response.changes[0].Signature.SigData = bytes.Repeat([]byte{0x12}, maxHistoryPageBytes)
			x.response = response
		case 8:
			current = nil
		case 9:
			x.lastAdvertised = current.CloneVT()
		case 10:
			x.advertised = current.CloneVT()
		case 11:
			x.revision = math.MaxUint64
		case 12:
			current.Config.ConfigChainHash = nil
		case 13:
			x.revision = math.MaxUint64 - 1
		case 15:
			message.GetHead().ConfigHash = nil
		case 16:
			x.remoteRevision = revision
		case 17:
			message.GetHead().ConfigHash = bytes.Repeat([]byte{1}, 129)
		case 18:
			message.GetHead().StateHash = bytes.Repeat([]byte{1}, 31)
		case 19:
			x.receiving = receiving
		case 20:
			x.control = syncAcknowledgment(7)
		case 21:
			currentDigest, err := syncStateHash(current)
			if err != nil {
				t.Fatal(err)
			}
			message.GetHead().StateHash = currentDigest
		case 22:
			message.GetHead().ConfigSeqno = 0
		case 23:
			message.GetHead().ConfigHash = current.Config.ConfigChainHash
			message.GetHead().RootSeqno = 0
		case 24, 25, 26, 27, 28:
			advertise()
			message = &SOSyncMessage{Body: &SOSyncMessage_HistoryRequest{HistoryRequest: request.CloneVT()}}
			switch variant {
			case 25:
				x.requested = true
			case 26:
				message.GetHistoryRequest().Revision++
			case 27:
				message.GetHistoryRequest().BaseHash = nil
			case 28:
				message.GetHistoryRequest().BaseHash = []byte("unknown history")
			}
		case 29, 30, 31, 32, 33:
			message = &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{
				Revision: revision, Cursor: request.BaseHash, Changes: []*sobject.SOConfigChange{change.CloneVT()},
			}}}
			if variant != 29 {
				x.receiving = receiving
			}
			switch variant {
			case 30:
				message.GetHistoryPage().Cursor = []byte("wrong cursor")
			case 31:
				for len(message.GetHistoryPage().Changes) <= maxHistoryPageEntries {
					message.GetHistoryPage().Changes = append(message.GetHistoryPage().Changes, change.CloneVT())
				}
			case 32:
				message.GetHistoryPage().Changes[0].PreviousHash = []byte("wrong predecessor")
			}
		case 34, 35, 36, 37, 38, 53:
			message = &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: prepared.snapshot.CloneVT()}}
			if variant != 34 {
				complete()
			}
			switch variant {
			case 35:
				x.control = syncAcknowledgment(7)
			case 37:
				receiving.cursor = bytes.Clone(request.BaseHash)
			case 38:
				x.terminal = sobject.ErrConfigHistoryUnavailable
			case 53:
				message = &SOSyncMessage{Body: &SOSyncMessage_Snapshot{}}
			}
		case 39, 40:
			message = &SOSyncMessage{Body: &SOSyncMessage_Op{Op: &SOSyncOp{Operation: []byte{0xff}}}}
			if variant == 39 {
				x.terminal = sobject.ErrConfigHistoryUnavailable
			}
		case 41, 42, 43, 44, 54:
			message = syncAcknowledgment(revision)
			if variant != 41 {
				advertise()
			}
			switch variant {
			case 43:
				message.GetAck().Revision++
			case 44:
				x.response = response
			case 54:
				message = &SOSyncMessage{Body: &SOSyncMessage_Ack{}}
			}
		case 45, 46:
			x.receiving = receiving
			message = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: revision}}}
			if variant == 46 {
				message.GetRecoveryRequired().Revision++
			}
		case 47, 48, 49:
			message = &SOSyncMessage{Body: &SOSyncMessage_Authorization{Authorization: &SOSyncAuthorization{Accepted: variant == 48}}}
			if variant == 49 {
				x.terminal = sobject.ErrConfigHistoryUnavailable
			}
		case 50:
			message = &SOSyncMessage{}
		case 51:
			message = &SOSyncMessage{Body: &SOSyncMessage_Head{}}
		case 52:
			x.receiving = receiving
			message = &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{}}
		}
		name := "sync exchange seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
		var loop *leanSyncLoopRun
		if withLoop {
			loop = &leanSyncLoopRun{current: current, next: current, message: message, event: 5, eventOK: true}
			if pump {
				loop.event = 2
			}
			switch variant {
			case 55:
				loop.event = 0
			case 56, 57, 58:
				loop.event = 1
				x.advertised = current.CloneVT()
				x.advertisementDeadline = time.Now().Add(-time.Second)
				if variant != 56 {
					x.receiving = receiving
					if variant == 57 {
						x.advertisementDeadline = time.Now().Add(time.Minute)
						receiving.deadline = time.Now().Add(-time.Second)
					}
				}
			case 59:
				loop.event = 3
			case 60, 61, 62, 63, 64:
				loop.event = 4
				x.inFlight = syncAcknowledgment(9)
				if variant == 61 {
					x.inFlight = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: revision}}}
				}
				if variant >= 62 {
					loop.eventOK = false
				}
				if variant == 63 || variant == 64 {
					positive := variant == 64
					loop.drain = &positive
				}
			case 65:
				loop.eventOK = false
				x.terminal = sobject.ErrConfigHistoryUnavailable
			case 66:
				loop.next = current.CloneVT()
				loop.next.Config.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
			case 67:
				current.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
			case 68:
				loop.event = 4
				x.inFlight = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{}}
			}
			name = "loop " + name
		}
		cases = append(cases, leanSyncExchangeCheck(t, x, current, message, pump, &writes, name, loop))
	}
	return cases
}
