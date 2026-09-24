package sobject_sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// leanSyncMarshal encodes real protocol values for primitive observations.
func leanSyncMarshal(t *testing.T, value interface{ MarshalVT() ([]byte, error) }) []byte {
	t.Helper()
	data, err := value.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// leanSyncSig projects key parsing and raw cryptographic verification, leaving
// membership and consensus decisions to the Lean model.
func leanSyncSig(a *fastjson.Arena, sig *peer.Signature, data []byte, context func(string) string) *fastjson.Value {
	// ParsePubKey returns nil without error when the envelope omits its key.
	id, valid := "", false
	pub, err := sig.ParsePubKey()
	if err == nil && pub != nil {
		parsed, parseErr := peer.IDFromPublicKey(pub)
		if parseErr == nil {
			id = parsed.String()
			verified, verifyErr := sig.VerifyWithPublic(context(id), pub, data)
			valid = verifyErr == nil && verified
		}
	}

	// Preserve the signer even when the message signature is invalid.
	v := a.NewObject()
	v.Set("signer", a.NewString(id))
	v.Set("valid", leanSyncBool(a, valid))
	return v
}

// leanSyncNonces retains nonce order, duplicates, and all uint64 bits.
func leanSyncNonces(a *fastjson.Arena, nonces []*sobject.SOAccountNonce) *fastjson.Value {
	result := a.NewArray()
	for i, nonce := range nonces {
		v := a.NewObject()
		v.Set("peer", a.NewString(nonce.GetPeerId()))
		v.Set("nonce", a.NewNumberString(strconv.FormatUint(nonce.GetNonce(), 10)))
		result.SetArrayItem(i, v)
	}
	return result
}

// leanSyncRoot abstracts encoding and signature structure, retaining all root
// progression, nonce-order, signer-authorization, and consensus decisions.
func leanSyncRoot(t *testing.T, a *fastjson.Arena, objectID string, root *sobject.SORoot) *fastjson.Value {
	t.Helper()
	data, err := root.BuildSignatureData()
	if err != nil {
		t.Fatal(err)
	}
	var digest []byte
	if root != nil {
		digest, err = sobject.DigestSOAuthoritativeRoot(root)
		if err != nil {
			t.Fatal(err)
		}
	}
	format := len(root.GetInner()) <= sobject.MaxInnerDataSize
	for _, nonce := range root.GetAccountNonces() {
		if _, err := nonce.ParsePeerID(); err != nil {
			format = false
		}
	}
	sigs := a.NewArray()
	for i, sig := range root.GetValidatorSignatures() {
		format = format && sig.Validate() == nil
		sigs.SetArrayItem(i, leanSyncSig(a, sig, data, func(string) string {
			return sobject.BuildValidatorRootSignatureContext(objectID, root.GetInnerSeqno())
		}))
	}

	v := a.NewObject()
	encoded := "nil"
	if root != nil {
		encoded = hex.EncodeToString(leanSyncMarshal(t, root))
	}
	v.Set("data", a.NewString(encoded))
	v.Set("content", a.NewString(hex.EncodeToString(root.GetInner())))
	v.Set("seqno", a.NewNumberString(strconv.FormatUint(root.GetInnerSeqno(), 10)))
	v.Set("digest", a.NewString(hex.EncodeToString(digest)))
	v.Set("format", leanSyncBool(a, format))
	v.Set("hasInner", leanSyncBool(a, len(root.GetInner()) != 0))
	v.Set("nonces", leanSyncNonces(a, root.GetAccountNonces()))
	v.Set("sigs", sigs)
	return v
}

// leanSyncOperation retains decoded identity and verifies the signed bytes.
func leanSyncOperation(t *testing.T, a *fastjson.Arena, objectID string, operation *sobject.SOOperation) *fastjson.Value {
	t.Helper()
	inner := &sobject.SOOperationInner{}
	parsed := inner.UnmarshalVT(operation.GetInner()) == nil
	v := a.NewObject()
	data := "nil"
	if operation != nil {
		data = hex.EncodeToString(leanSyncMarshal(t, operation))
	}
	v.Set("data", a.NewString(data))
	v.Set("peer", a.NewString(inner.GetPeerId()))
	v.Set("localId", a.NewString(inner.GetLocalId()))
	v.Set("nonce", a.NewNumberString(strconv.FormatUint(inner.GetNonce(), 10)))
	v.Set("parsed", leanSyncBool(a, parsed))
	v.Set("innerValid", leanSyncBool(a, parsed && inner.Validate() == nil))
	v.Set("format", leanSyncBool(a, operation.Validate() == nil))
	v.Set("sig", leanSyncSig(a, operation.GetSignature(), operation.GetInner(), func(id string) string {
		return sobject.BuildSOOperationSignatureContext(objectID, id, inner.GetNonce(), inner.GetLocalId())
	}))
	return v
}

// leanSyncOperations projects a pending or explicitly accepted batch.
func leanSyncOperations(t *testing.T, a *fastjson.Arena, objectID string, operations []*sobject.SOOperation) *fastjson.Value {
	t.Helper()
	v := a.NewArray()
	for i, operation := range operations {
		v.SetArrayItem(i, leanSyncOperation(t, a, objectID, operation))
	}
	return v
}

// leanSyncRejections projects signed rejections without deciding their authority.
func leanSyncRejections(t *testing.T, a *fastjson.Arena, objectID string, rejections []*sobject.SOOperationRejection) *fastjson.Value {
	t.Helper()
	v := a.NewArray()
	for i, rejection := range rejections {
		inner := &sobject.SOOperationRejectionInner{}
		parsed := inner.UnmarshalVT(rejection.GetInner()) == nil
		r := a.NewObject()
		data := "nil"
		if rejection != nil {
			data = hex.EncodeToString(leanSyncMarshal(t, rejection))
		}
		r.Set("data", a.NewString(data))
		r.Set("peer", a.NewString(inner.GetPeerId()))
		r.Set("localId", a.NewString(inner.GetLocalId()))
		r.Set("nonce", a.NewNumberString(strconv.FormatUint(inner.GetOpNonce(), 10)))
		r.Set("parsed", leanSyncBool(a, parsed))
		r.Set("innerValid", leanSyncBool(a, parsed && inner.Validate() == nil))
		r.Set("format", leanSyncBool(a, rejection.Validate() == nil))
		r.Set("sig", leanSyncSig(a, rejection.GetSignature(), rejection.GetInner(), func(id string) string {
			return sobject.BuildSOOperationRejectionSignatureContext(objectID, id, inner.GetPeerId(), inner.GetOpNonce(), inner.GetLocalId())
		}))
		v.SetArrayItem(i, r)
	}
	return v
}

// leanSyncState retains all state fields, including grant authority and the
// opaque invitation records that host imports preserve locally.
func leanSyncState(t *testing.T, a *fastjson.Arena, objectID string, state *sobject.SOState) *fastjson.Value {
	t.Helper()
	grants := a.NewArray()
	for i, grant := range state.GetRootGrants() {
		g := a.NewObject()
		data := "nil"
		if grant != nil {
			data = hex.EncodeToString(leanSyncMarshal(t, grant))
		}
		g.Set("data", a.NewString(data))
		g.Set("peer", a.NewString(grant.GetPeerId()))
		g.Set("format", leanSyncBool(a, grant.Validate() == nil))
		g.Set("sig", leanSyncSig(a, grant.GetSignature(), grant.GetInnerData(), func(signer string) string {
			return sobject.BuildSOGrantSignatureContext(objectID, signer, grant.GetPeerId())
		}))
		grants.SetArrayItem(i, g)
	}
	invites := a.NewArray()
	for i, invite := range state.GetInvites() {
		invites.SetArrayItem(i, leanSyncInvite(t, a, invite))
	}
	groups := a.NewArray()
	for i, group := range state.GetOpRejections() {
		g := a.NewObject()
		g.Set("peer", a.NewString(group.GetPeerId()))
		g.Set("entries", leanSyncRejections(t, a, objectID, group.GetRejections()))
		groups.SetArrayItem(i, g)
	}

	v := a.NewObject()
	config := state.GetConfig()
	if config == nil {
		config = &sobject.SharedObjectConfig{}
	}
	v.Set("config", leanSyncConfig(a, config))
	v.Set("root", leanSyncRoot(t, a, objectID, state.GetRoot()))
	v.Set("grants", grants)
	v.Set("invites", invites)
	v.Set("ops", leanSyncOperations(t, a, objectID, state.GetOps()))
	v.Set("queued", leanSyncNonces(a, state.GetQueuedAccountNonces()))
	v.Set("rejections", groups)
	return v
}

// leanSyncInvite retains immutable bytes separately from the fields updated by invite.go.
func leanSyncInvite(t *testing.T, a *fastjson.Arena, invite *sobject.SOInvite) *fastjson.Value {
	t.Helper()
	data := "nil"
	if invite != nil {
		immutable := invite.CloneVT()
		immutable.Uses = 0
		immutable.Revoked = false
		data = hex.EncodeToString(leanSyncMarshal(t, immutable))
	}
	v := a.NewObject()
	v.Set("data", a.NewString(data))
	v.Set("id", a.NewString(invite.GetInviteId()))
	v.Set("tokenHash", a.NewString(hex.EncodeToString(invite.GetTokenHash())))
	v.Set("maxUses", a.NewNumberString(strconv.FormatUint(uint64(invite.GetMaxUses()), 10)))
	v.Set("uses", a.NewNumberString(strconv.FormatUint(uint64(invite.GetUses()), 10)))
	v.Set("revoked", leanSyncBool(a, invite.GetRevoked()))
	return v
}

// TestLeanSyncResponseConformance connects real pages, pinned snapshots and atomic host imports.
func TestLeanSyncResponseConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(4) {
		cases = append(cases, leanSyncResponseCases(t, seed)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncResponse varies complete signed responses and host failure observations.
func FuzzLeanSyncResponse(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(19))
	f.Fuzz(func(t *testing.T, seed uint64) {
		checkLeanSync(t, leanSyncOracle(t), leanSyncResponseCases(t, seed))
	})
}

// leanSyncResponseCases compares response preparation and acceptance through their real owners.
func leanSyncResponseCases(t *testing.T, seed uint64) []leanSyncCase {
	t.Helper()
	const objectID = "lean-sync-response"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, objectID, owner, reader)
	initial.Invites = []*sobject.SOInvite{{InviteId: "local capability", TokenHash: []byte("local token")}}
	sender := newAuthenticationPeer(t, objectID, owner, initial)
	for range maxHistoryPageEntries + 1 + int(seed%3) {
		current, err := sender.soHost.GetHostState(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		change, err := sobject.BuildSOConfigChange(current.Config, current.Config,
			sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
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
	target.Root.InnerSeqno = 2 + seed%4
	signSnapshotRoot(t, objectID, target, owner)
	target.Invites = []*sobject.SOInvite{{InviteId: "remote capability", TokenHash: []byte("secret token")}}
	target.QueuedAccountNonces = []*sobject.SOAccountNonce{{PeerId: mustPeerIDStr(t, owner), Nonce: 42}}
	request := &SOSyncHistoryRequest{Revision: seed + 1, BaseHash: initial.Config.ConfigChainHash}
	response, err := sender.prepareResponse(t.Context(), target, request)
	if err != nil {
		t.Fatal(err)
	}
	changes := response.changes
	digest, err := syncStateHash(target)
	if err != nil {
		t.Fatal(err)
	}
	head := &SOSyncHead{Revision: request.Revision, ConfigHash: target.Config.ConfigChainHash,
		ConfigSeqno: target.Config.ConfigChainSeqno, RootSeqno: target.Root.InnerSeqno, StateHash: digest}
	received := &syncReceive{head: head, base: request.BaseHash, cursor: request.BaseHash}
	var cases []leanSyncCase
	var pages []*SOSyncMessage
	for len(response.changes) != 0 {
		message, err := response.nextMessage()
		if err != nil {
			t.Fatal(err)
		}
		var arena fastjson.Arena
		input := arena.NewObject()
		pages = append(pages, message)
		input.Set("op", arena.NewString("appendSyncPage"))
		input.Set("before", leanSyncReceive(t, &arena, received))
		input.Set("page", leanSyncPage(t, &arena, message))
		if err := received.appendPage(message); err != nil {
			t.Fatal(err)
		}
		expected, result := arena.NewObject(), arena.NewObject()
		expected.Set("ok", arena.NewTrue())
		result.Set("ok", arena.NewTrue())
		result.Set("state", leanSyncReceive(t, &arena, received))
		result.Set("recovery", arena.NewFalse())
		expected.Set("received", result)
		cases = append(cases, leanSyncCase{name: "complete response page " + strconv.Itoa(len(cases)),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)})
	}
	message, err := response.nextMessage()
	if err != nil || message.GetSnapshot() == nil {
		t.Fatalf("complete response snapshot: %v", err)
	}

	// The pinned snapshot is the exact encoding whose digest was advertised.
	decoded := &sobject.SOState{}
	if err := decoded.UnmarshalVT(message.GetSnapshot().GetSoState()); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Invites) != 0 || len(decoded.QueuedAccountNonces) != 0 {
		t.Fatal("prepared snapshot disclosed local capabilities or reservations")
	}
	cases = append(cases, leanSyncReceptionCases(t, head, request.BaseHash, pages)...)
	for variant := range 29 {
		previous, candidate := initial.CloneVT(), decoded.CloneVT()
		receiving := &syncReceive{head: head.CloneVT(), base: append([]byte(nil), received.base...),
			cursor: append([]byte(nil), received.cursor...), size: received.size}
		for _, entry := range changes {
			receiving.changes = append(receiving.changes, entry.CloneVT())
		}
		snapshot := message.GetSnapshot().CloneVT()
		lockOK, accessOK, writeOK, readOK := true, true, true, true
		var advanced *sobject.SOState
		switch variant {
		case 1:
			snapshot.Revision ^= 1
		case 2:
			snapshot.BaseHash = []byte("wrong base")
		case 3:
			receiving.cursor = []byte("incomplete suffix")
		case 6:
			snapshot.RootSeqno++
		case 7:
			candidate.Root.InnerSeqno++
		case 8:
			candidate.Config.ConfigChainSeqno++
		case 9:
			candidate.Config.ConfigChainHash = []byte("unadvertised config")
		case 10:
			receiving.changes = receiving.changes[1:]
		case 11:
			receiving.changes[0].Signature.SigData = []byte("invalid signature")
		case 12:
			candidate.Root.ValidatorSignatures[0].SigData = []byte("invalid root proof")
		case 13:
			candidate.Root.InnerSeqno = previous.Root.InnerSeqno
			candidate.Root.Inner = []byte("conflicting accepted content")
			snapshot.RootSeqno = candidate.Root.InnerSeqno
			receiving.head.RootSeqno = candidate.Root.InnerSeqno
		case 14:
			accessOK = false
		case 15:
			lockOK = false
		case 16:
			writeOK = false
		case 17:
			previous = target.CloneVT()
			previous.Config.ConfigChainSeqno++
			previous.Root.InnerSeqno++
		case 18:
			candidate.Config.ConfigChainHash = []byte("equal-sequence fork")
			receiving.head.ConfigHash = candidate.Config.ConfigChainHash
			receiving.cursor = candidate.Config.ConfigChainHash
		case 19:
			previous.Root.InnerSeqno = candidate.Root.InnerSeqno + 1
		case 20:
			previous = decoded.CloneVT()
			receiving.changes = nil
		case 22:
			candidate.Config = nil
		case 23:
			config := candidate.Config.CloneVT()
			config.Participants = config.Participants[:1]
			change, err := sobject.BuildSOConfigChange(candidate.Config, config,
				sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, nil)
			if err != nil {
				t.Fatal(err)
			}
			candidate.Config, err = sobject.VerifyConfigChange(candidate.Config, change)
			if err != nil {
				t.Fatal(err)
			}
			receiving.changes = append(receiving.changes, change)
			receiving.cursor = candidate.Config.ConfigChainHash
			receiving.head.ConfigHash = candidate.Config.ConfigChainHash
			receiving.head.ConfigSeqno = candidate.Config.ConfigChainSeqno
		case 24:
			writeOK = false
			advanced = candidate.CloneVT()
			advanced.Root.InnerSeqno++
		case 25:
			readOK = false
		case 26:
			candidate.Root = previous.Root.CloneVT()
			candidate.Root.ValidatorSignatures[0].SigData = []byte("historical proof replaced")
			snapshot.RootSeqno = candidate.Root.InnerSeqno
			receiving.head.RootSeqno = candidate.Root.InnerSeqno
		case 27:
			candidate.Ops = []*sobject.SOOperation{{Inner: []byte("malformed operation")}}
		case 28:
			candidate.Invites = target.Invites
			candidate.QueuedAccountNonces = target.QueuedAccountNonces
		}
		snapshot.SoState = leanSyncMarshal(t, candidate)
		if variant == 5 {
			snapshot.SoState = []byte{0xff}
		}
		if variant == 21 {
			snapshot = nil
		}
		contentDigest := sha256.Sum256(snapshot.GetSoState())
		receiving.head.StateHash = contentDigest[:]
		if variant == 4 {
			receiving.head.StateHash = []byte("wrong digest")
		}
		candidate = &sobject.SOState{}
		decodeErr := candidate.UnmarshalVT(snapshot.GetSoState())
		published, wrote := previous, false
		observed := ccontainer.NewCContainerVT(previous)
		watch := func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
			if !readOK {
				return nil, nil, errors.New("injected watch failure")
			}
			return observed, func() {}, nil
		}
		lock := func(context.Context, string) (sobject.SOStateLock, error) {
			if advanced != nil {
				observed.SetValue(advanced)
			}
			if !lockOK {
				return nil, errors.New("injected lock failure")
			}
			return sobject.NewSOStateLock(previous, func(_ context.Context, state *sobject.SOState, _ ...*sobject.SOConfigChange) error {
				if !writeOK {
					return errors.New("injected atomic write failure")
				}
				published, wrote = state, true
				observed.SetValue(state)
				return nil
			}, func() {}), nil
		}
		host := sobject.NewSOHost(t.Context(), watch, lock, objectID)
		t.Cleanup(host.ClearContext)
		localID, err := peer.IDFromPrivateKey(reader)
		if err != nil {
			t.Fatal(err)
		}
		local := NewSOSync(gateLogger(), nil, objectID, localID, reader, host, nil,
			func(context.Context, *sobject.SOState) error {
				if !accessOK {
					return errors.New("injected access failure")
				}
				return nil
			})
		var arena fastjson.Arena
		input := arena.NewObject()
		input.Set("previous", leanSyncState(t, &arena, objectID, previous))
		input.Set("receiving", leanSyncReceive(t, &arena, receiving))
		input.Set("snapshot", leanSyncSnapshot(&arena, snapshot))
		input.Set("digest", arena.NewString(hex.EncodeToString(contentDigest[:])))
		projection := arena.NewNull()
		if decodeErr == nil {
			projection = leanSyncState(t, &arena, objectID, candidate)
		}
		input.Set("decoded", projection)
		input.Set("candidateBytes", arena.NewNumberInt(candidate.SizeVT()))
		beforeRead := arena.NewNull()
		if readOK {
			beforeRead = leanSyncState(t, &arena, objectID, previous)
		}
		input.Set("beforeRead", beforeRead)
		input.Set("localPeer", arena.NewString(localID.String()))
		input.Set("lockOK", leanSyncBool(&arena, lockOK))
		input.Set("accessOK", leanSyncBool(&arena, accessOK))
		input.Set("writeOK", leanSyncBool(&arena, writeOK))
		err = local.acceptResponse(t.Context(), receiving, snapshot)
		afterRead := arena.NewNull()
		if readOK {
			afterRead = leanSyncState(t, &arena, objectID, observed.GetValue())
		}
		input.Set("afterRead", afterRead)
		req, expected, accepted, publication := arena.NewObject(), arena.NewObject(), arena.NewObject(), arena.NewObject()
		req.Set("op", arena.NewString("acceptSyncResponse"))
		req.Set("input", input)
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		accepted.Set("ok", leanSyncBool(&arena, err == nil))
		publication.Set("state", leanSyncState(t, &arena, objectID, published))
		publication.Set("wrote", leanSyncBool(&arena, wrote))
		publication.Set("revoked", leanSyncBool(&arena, errors.Is(err, sobject.ErrParticipantRevoked)))
		accepted.Set("host", publication)
		expected.Set("accepted", accepted)
		cases = append(cases, leanSyncCase{name: "acceptSyncResponse seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: req.MarshalTo(nil), expected: expected.MarshalTo(nil)})
	}
	cases = append(cases, leanSyncPreparedCases(t, objectID, sender, target, request, changes)...)
	return append(cases, leanSyncObsoleteCases(t, objectID, target)...)
}

// leanSyncPreparedCases exercises the provider read, disclosure projection and complete-frame budget.
func leanSyncPreparedCases(t *testing.T, objectID string, sender *SOSync, target *sobject.SOState,
	request *SOSyncHistoryRequest, changes []*sobject.SOConfigChange,
) []leanSyncCase {
	t.Helper()
	var cases []leanSyncCase
	for variant := range 6 {
		state, req := target.CloneVT(), request.CloneVT()
		var history []*sobject.SOConfigChange
		for _, entry := range changes {
			history = append(history, entry.CloneVT())
		}
		readOK := true
		switch variant {
		case 1:
			readOK = false
		case 2:
			readOK = false
			req.BaseHash = state.Config.ConfigChainHash
		case 3:
			history[0].Signature.SigData = make([]byte, maxHistoryPageBytes)
		case 4:
			state.Config.ConfigChainHash = nil
		case 5:
			for range 3 {
				padding := len(history[0].Signature.SigData) + maxHistoryPageBytes - 256 - history[0].SizeVT()
				history[0].Signature.SigData = make([]byte, padding)
			}
			if history[0].SizeVT()+256 != maxHistoryPageBytes {
				t.Fatal("failed to construct prepareResponse entry boundary")
			}
		}
		host := sobject.NewSOHost(t.Context(), nil, nil, objectID, &sobject.SOHostSyncFuncs{
			History: func(context.Context, string, []byte, []byte) ([]*sobject.SOConfigChange, error) {
				if !readOK {
					return nil, sobject.ErrConfigHistoryUnavailable
				}
				return history, nil
			},
		})
		t.Cleanup(host.ClearContext)
		local := NewSOSync(gateLogger(), nil, objectID, sender.localObjectPeerID, sender.localObjectKey, host, nil)
		wire := state.CloneVT()
		wire.Invites, wire.QueuedAccountNonces = nil, nil
		encoded := leanSyncMarshal(t, wire)
		snapshot := &SOSyncSnapshot{SoState: encoded, RootSeqno: state.Root.InnerSeqno, Revision: req.Revision, BaseHash: req.BaseHash}
		frame := &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: snapshot}}
		var arena fastjson.Arena
		input, requestJSON := arena.NewObject(), arena.NewObject()
		input.Set("op", arena.NewString("prepareSyncResponse"))
		input.Set("state", leanSyncState(t, &arena, objectID, state))
		requestJSON.Set("revision", arena.NewNumberString(strconv.FormatUint(req.Revision, 10)))
		requestJSON.Set("base", arena.NewString(hex.EncodeToString(req.BaseHash)))
		input.Set("request", requestJSON)
		retained := arena.NewNull()
		if readOK {
			retained = leanSyncChanges(t, &arena, history)
		}
		input.Set("history", retained)
		input.Set("encoded", arena.NewString(hex.EncodeToString(encoded)))
		input.Set("bytes", arena.NewNumberInt(frame.SizeVT()))
		response, err := local.prepareResponse(t.Context(), state, req)
		expected := arena.NewObject()
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		projected := arena.NewNull()
		if response != nil {
			projected = leanSyncResponse(t, &arena, response)
		}
		expected.Set("response", projected)
		cases = append(cases, leanSyncCase{name: "prepareSyncResponse variant " + strconv.Itoa(variant),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)})

		input = arena.NewObject()
		input.Set("op", arena.NewString("syncStateHash"))
		input.Set("state", leanSyncState(t, &arena, objectID, state))
		input.Set("bytes", arena.NewNumberInt(wire.SizeVT()))
		input.Set("encoded", arena.NewString(hex.EncodeToString(encoded)))
		digest := sha256.Sum256(encoded)
		input.Set("digest", arena.NewString(hex.EncodeToString(digest[:])))
		actual, err := syncStateHash(state)
		expected = arena.NewObject()
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		result := arena.NewNull()
		if err == nil {
			result = arena.NewString(hex.EncodeToString(actual))
		}
		expected.Set("digest", result)
		cases = append(cases, leanSyncCase{name: "syncStateHash variant " + strconv.Itoa(variant),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)})
	}
	return cases
}

// TestLeanSyncResponseSizeLimits measures real snapshot-envelope and state-payload caps.
func TestLeanSyncResponseSizeLimits(t *testing.T) {
	oracle := leanSyncOracle(t)
	const objectID = "lean-response-size"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	for extra := range 3 {
		state := authenticationState(t, objectID, owner, reader)
		state.Root.Inner = make([]byte, maxMessageSize)
		snapshot := &SOSyncSnapshot{RootSeqno: state.Root.InnerSeqno, Revision: 1, BaseHash: state.Config.ConfigChainHash}
		frame := &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: snapshot}}
		for range 3 {
			snapshot.SoState = leanSyncMarshal(t, state)
			padding := len(state.Root.Inner) + maxMessageSize + extra - frame.SizeVT()
			if extra == 2 {
				padding = len(state.Root.Inner) + maxMessageSize + 1 - state.SizeVT()
			}
			state.Root.Inner = make([]byte, padding)
		}
		snapshot.SoState = leanSyncMarshal(t, state)
		if extra < 2 && frame.SizeVT() != maxMessageSize+extra || extra == 2 && state.SizeVT() != maxMessageSize+1 {
			t.Fatal("failed to construct snapshot frame boundary")
		}
		local := newAuthenticationPeer(t, objectID, owner, state)
		request := &SOSyncHistoryRequest{Revision: 1, BaseHash: state.Config.ConfigChainHash}
		var arena fastjson.Arena
		input, req := arena.NewObject(), arena.NewObject()
		input.Set("op", arena.NewString("prepareSyncResponse"))
		input.Set("state", leanSyncState(t, &arena, objectID, state))
		req.Set("revision", arena.NewNumberInt(1))
		req.Set("base", arena.NewString(hex.EncodeToString(request.BaseHash)))
		input.Set("request", req)
		input.Set("history", arena.NewNull())
		input.Set("encoded", arena.NewString(hex.EncodeToString(snapshot.SoState)))
		input.Set("bytes", arena.NewNumberInt(frame.SizeVT()))
		actual, err := local.prepareResponse(t.Context(), state, request)
		expected := arena.NewObject()
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		result := arena.NewNull()
		if actual != nil {
			result = leanSyncResponse(t, &arena, actual)
		}
		expected.Set("response", result)
		checkLeanSync(t, oracle, []leanSyncCase{{name: "snapshot frame cap plus " + strconv.Itoa(extra),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)}})

		input, expected = arena.NewObject(), arena.NewObject()
		input.Set("op", arena.NewString("syncStateHash"))
		input.Set("state", leanSyncState(t, &arena, objectID, state))
		input.Set("bytes", arena.NewNumberInt(state.SizeVT()))
		input.Set("encoded", arena.NewString(hex.EncodeToString(snapshot.SoState)))
		digest := sha256.Sum256(snapshot.SoState)
		input.Set("digest", arena.NewString(hex.EncodeToString(digest[:])))
		actualDigest, err := syncStateHash(state)
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		result = arena.NewNull()
		if err == nil {
			result = arena.NewString(hex.EncodeToString(actualDigest))
		}
		expected.Set("digest", result)
		checkLeanSync(t, oracle, []leanSyncCase{{name: "state payload cap case " + strconv.Itoa(extra),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)}})
	}
}

// leanSyncObsoleteCases covers every ordering of the two sequence coordinates.
func leanSyncObsoleteCases(t *testing.T, objectID string, state *sobject.SOState) []leanSyncCase {
	t.Helper()
	var cases []leanSyncCase
	for configDelta := -1; configDelta <= 1; configDelta++ {
		for rootDelta := -1; rootDelta <= 1; rootDelta++ {
			head := &SOSyncHead{ConfigSeqno: state.Config.ConfigChainSeqno, RootSeqno: state.Root.InnerSeqno}
			current := state.CloneVT()
			current.Config.ConfigChainSeqno = uint64(int64(head.ConfigSeqno) + int64(configDelta))
			current.Root.InnerSeqno = uint64(int64(head.RootSeqno) + int64(rootDelta))
			host, _ := newMemHost(objectID, current)
			t.Cleanup(host.ClearContext)
			local := &SOSync{soHost: host}
			var arena fastjson.Arena
			input, expected := arena.NewObject(), arena.NewObject()
			input.Set("op", arena.NewString("syncResponseObsolete"))
			input.Set("current", leanSyncState(t, &arena, objectID, current))
			input.Set("head", leanSyncHead(&arena, head))
			expected.Set("ok", leanSyncBool(&arena, local.responseObsolete(t.Context(), head)))
			cases = append(cases, leanSyncCase{name: "obsolete coordinates " + strconv.Itoa(configDelta) + "/" + strconv.Itoa(rootDelta),
				request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)})
		}
	}
	return cases
}

// leanSyncReceptionCases compares complete and interrupted sequences of the actual received pages.
func leanSyncReceptionCases(t *testing.T, head *SOSyncHead, base []byte, pages []*SOSyncMessage) []leanSyncCase {
	t.Helper()
	var cases []leanSyncCase
	for variant := range 8 {
		receiving := &syncReceive{head: head, base: base, cursor: base}
		var messages []*SOSyncMessage
		for _, page := range pages {
			messages = append(messages, page.CloneVT())
		}
		switch variant {
		case 1:
			messages[0].GetHistoryPage().Revision ^= 1
		case 2:
			messages[1].GetHistoryPage().Cursor = []byte("wrong second cursor")
		case 3:
			messages[0], messages[1] = messages[1], messages[0]
		case 4:
			messages = messages[:len(messages)-1]
		case 5:
			messages = append(messages[:1], messages...)
		case 6:
			messages = append(messages, &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: &SOSyncHistoryPage{
				Revision: head.Revision, Cursor: head.ConfigHash,
			}}})
		case 7:
			receiving.size = sobject.MaxConfigSuffixBytes
		}
		var arena fastjson.Arena
		input, projected := arena.NewObject(), arena.NewArray()
		input.Set("op", arena.NewString("receiveSyncPages"))
		input.Set("before", leanSyncReceive(t, &arena, receiving))
		for index, message := range messages {
			projected.SetArrayItem(index, leanSyncPage(t, &arena, message))
		}
		input.Set("pages", projected)
		var err error
		for _, message := range messages {
			if err = receiving.appendPage(message); err != nil {
				break
			}
		}
		expected, result := arena.NewObject(), arena.NewNull()
		if err == nil {
			result = leanSyncReceive(t, &arena, receiving)
		}
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		expected.Set("received", result)
		cases = append(cases, leanSyncCase{name: "receiveSyncPages variant " + strconv.Itoa(variant),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)})
	}
	return cases
}
