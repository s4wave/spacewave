package sobject_sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// leanSyncCase keeps one actual Go decision beside its projected oracle input.
type leanSyncCase struct {
	// name identifies the generated operation and seed.
	name string
	// request carries the model inputs as one JSON line.
	request []byte
	// expected contains the observable Go result fields.
	expected []byte
}

// leanSyncOracle skips optional conformance when no built oracle was supplied.
func leanSyncOracle(t *testing.T) string {
	t.Helper()
	oracle := os.Getenv("SPACEWAVE_LEAN_ORACLE")
	if oracle == "" {
		t.Skip("SPACEWAVE_LEAN_ORACLE is not set; build it with bun run lean")
	}
	return oracle
}

// checkLeanSync compares actual Go observations with one batched oracle process.
func checkLeanSync(t *testing.T, oracle string, cases []leanSyncCase) {
	t.Helper()
	var input bytes.Buffer
	for _, test := range cases {
		input.Write(test.request)
		input.WriteByte('\n')
	}
	cmd := exec.CommandContext(t.Context(), oracle)
	cmd.Stdin = &input
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run Lean sync oracle: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != len(cases) {
		t.Fatalf("oracle returned %d of %d results", len(lines), len(cases))
	}
	var actualParser, expectedParser fastjson.Parser
	for index, test := range cases {
		actual, err := actualParser.Parse(lines[index])
		if err != nil {
			t.Fatal(err)
		}
		expected, err := expectedParser.ParseBytes(test.expected)
		if err != nil {
			t.Fatal(err)
		}
		if actual.Get("error") != nil {
			t.Fatalf("%s oracle error: %.512s", test.name, lines[index])
		}
		if difference := leanSyncDifference(actual, expected, "$"); difference != "" {
			t.Fatalf("%s differs at %s", test.name, difference)
		}
	}
	t.Logf("%d sync cases agree", len(cases))
}

// leanSyncDifference locates the first mismatched field without dumping a large history buffer.
func leanSyncDifference(actual, expected *fastjson.Value, path string) string {
	if actual == nil || actual.Type() != expected.Type() {
		return path
	}
	switch expected.Type() {
	case fastjson.TypeArray:
		left, right := actual.GetArray(), expected.GetArray()
		if len(left) != len(right) {
			return path + ".length"
		}
		for index, value := range right {
			if difference := leanSyncDifference(left[index], value, path+"["+strconv.Itoa(index)+"]"); difference != "" {
				return difference
			}
		}
	case fastjson.TypeObject:
		var difference string
		expected.GetObject().Visit(func(key []byte, value *fastjson.Value) {
			if difference == "" {
				difference = leanSyncDifference(actual.Get(string(key)), value, path+"."+string(key))
			}
		})
		return difference
	default:
		if !bytes.Equal(actual.MarshalTo(nil), expected.MarshalTo(nil)) {
			return path
		}
	}
	return ""
}

// TestLeanSyncAuthenticationConformance checks real participant roles and stream handshakes.
func TestLeanSyncAuthenticationConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for seed := range uint64(12) {
		cases = append(cases, leanSyncAuthenticationCases(t, seed)...)
	}
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncAuthentication varies signed identities, duplicate roles and challenged exchanges.
func FuzzLeanSyncAuthentication(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(17))
	f.Fuzz(func(t *testing.T, seed uint64) {
		oracle := leanSyncOracle(t)
		checkLeanSync(t, oracle, leanSyncAuthenticationCases(t, seed))
	})
}

// leanSyncBool selects the arena's immutable Boolean value.
func leanSyncBool(arena *fastjson.Arena, value bool) *fastjson.Value {
	if value {
		return arena.NewTrue()
	}
	return arena.NewFalse()
}

// leanSyncParticipants projects enum codes and peer strings without validating held state.
func leanSyncParticipants(arena *fastjson.Arena, participants []*sobject.SOParticipantConfig) *fastjson.Value {
	result := arena.NewArray()
	for index, participant := range participants {
		value := arena.NewObject()
		value.Set("peer", arena.NewString(participant.GetPeerId()))
		value.Set("role", arena.NewNumberInt(int(participant.GetRole())))
		value.Set("entity", arena.NewString(participant.GetEntityId()))
		result.SetArrayItem(index, value)
	}
	return result
}

// leanSyncProof projects only key parsing, exact-transcript verification and signer derivation.
func leanSyncProof(arena *fastjson.Arena, transcript *SOSyncAuthTranscript, proof *peer.Signature) *fastjson.Value {
	if proof == nil {
		return arena.NewNull()
	}
	var signer string
	var valid bool
	public, err := proof.ParsePubKey()
	if err == nil && public != nil {
		data, encodeErr := transcript.MarshalVT()
		if encodeErr == nil {
			verified, verifyErr := proof.VerifyWithPublic(authenticationContext, public, data)
			id, idErr := peer.IDFromPublicKey(public)
			signer, valid = id.String(), verified && verifyErr == nil && idErr == nil
		}
	}
	result := arena.NewObject()
	result.Set("signer", arena.NewString(signer))
	result.Set("valid", leanSyncBool(arena, valid))
	return result
}

// leanSyncAuthenticationCases exercises role scans and every handshake rejection boundary.
func leanSyncAuthenticationCases(t *testing.T, seed uint64) []leanSyncCase {
	t.Helper()
	keys := []crypto.PrivKey{mustKeyPair(t), mustKeyPair(t), mustKeyPair(t)}
	ids := []string{mustPeerIDStr(t, keys[0]), mustPeerIDStr(t, keys[1]), mustPeerIDStr(t, keys[2])}
	rng := rand.New(rand.NewPCG(seed, 0x61757468))
	var arena fastjson.Arena
	var cases []leanSyncCase
	for variant := range 50 {
		var participants []*sobject.SOParticipantConfig
		for range rng.IntN(12) {
			participants = append(participants, participantCfg(ids[rng.IntN(len(ids))], sobject.SOParticipantRole(rng.IntN(8)-2)))
		}
		configHash := []byte("held-head")
		if variant%5 == 0 {
			configHash = nil
		}
		localID, _ := peer.IDB58Decode(ids[rng.IntN(len(ids))])
		remoteID, _ := peer.IDB58Decode(ids[rng.IntN(len(ids))])
		state := &sobject.SOState{Config: &sobject.SharedObjectConfig{Participants: participants, ConfigChainHash: configHash}}
		sync := &SOSync{localObjectPeerID: localID}
		request, expected := arena.NewObject(), arena.NewObject()
		request.Set("op", arena.NewString("authorizeSync"))
		request.Set("participants", leanSyncParticipants(&arena, participants))
		request.Set("hash", arena.NewString(hex.EncodeToString(configHash)))
		request.Set("local", arena.NewString(localID.String()))
		request.Set("remote", arena.NewString(remoteID.String()))
		expected.Set("ok", leanSyncBool(&arena, sync.authorizeParticipants(state, remoteID) == nil))
		cases = append(cases, leanSyncCase{
			name:    "authorizeSync seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant),
			request: request.MarshalTo(nil), expected: expected.MarshalTo(nil),
		})
	}
	for variant := range 23 {
		cases = append(cases, leanSyncHandshake(t, seed, variant, keys)...)
	}
	return cases
}

// leanSyncHandshake runs authenticate against a real challenged peer over an unbuffered transport.
func leanSyncHandshake(t *testing.T, seed uint64, variant int, keys []crypto.PrivKey) []leanSyncCase {
	t.Helper()
	const objectID = "lean-sync-authentication"
	state := authenticationState(t, objectID, keys[0], keys[1])
	if variant == 14 {
		state.Config.ConfigChainHash = nil
	}
	if variant == 15 || variant == 20 {
		state.Config.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
	}
	if variant == 16 {
		state.Config.Participants[1].Role = sobject.SOParticipantRole_SOParticipantRole_UNKNOWN
	}
	if variant == 17 {
		state.Config.Participants = append(state.Config.Participants, participantCfg(mustPeerIDStr(t, keys[1]), sobject.SOParticipantRole_SOParticipantRole_UNKNOWN))
	}

	// Install adversarial held values after the initial signed checkpoint was built.
	sync := newAuthenticationPeer(t, objectID, keys[0], state)
	var notification *bool
	sync.peerAdmission = func(_ peer.ID, accepted bool) { notification = &accepted }
	if variant == 21 {
		sync.peerAdmission = nil
	}
	if variant == 1 {
		sync.localObjectKey = nil
	}
	if variant == 2 {
		sync.localObjectKey = keys[2]
	}
	localTransport, remoteTransport := peer.ID("transport-a"), peer.ID("transport-b")
	if variant == 22 {
		localTransport, remoteTransport = remoteTransport, localTransport
	}
	if variant == 3 {
		remoteTransport = localTransport
	}
	if variant == 4 {
		localTransport = ""
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	stop := context.AfterFunc(ctx, func() { left.Close(); right.Close() })
	defer stop()
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 8)}
	remoteNonce := bytes.Repeat([]byte{byte(seed)}, authenticationNonceSize)
	if variant == 5 {
		remoteNonce = remoteNonce[:31]
	}
	var localNonce []byte
	var proof *peer.Signature
	var transcript *SOSyncAuthTranscript
	remoteDone := make(chan error, 1)
	go func() {
		remoteDone <- func() error {
			session := stream_packet.NewSession(right, 64*1024)
			var challenge *SOSyncMessage
			if variant == 6 {
				challenge = &SOSyncMessage{}
				if err := session.RecvMsg(challenge); err != nil {
					return err
				}
				remoteNonce = bytes.Clone(challenge.GetChallenge().GetNonce())
				if err := session.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Challenge{Challenge: &SOSyncChallenge{Nonce: remoteNonce}}}); err != nil {
					return err
				}
			} else {
				var err error
				challenge, err = exchangeMessage(session, remoteTransport < localTransport,
					&SOSyncMessage{Body: &SOSyncMessage_Challenge{Challenge: &SOSyncChallenge{Nonce: remoteNonce}}})
				if err != nil {
					return err
				}
			}
			localNonce = bytes.Clone(challenge.GetChallenge().GetNonce())
			if variant == 18 {
				return right.Close()
			}
			transcript = &SOSyncAuthTranscript{
				SharedObjectId: objectID, SenderTransport: []byte(remoteTransport), ReceiverTransport: []byte(localTransport),
				SenderNonce: remoteNonce, ReceiverNonce: challenge.GetChallenge().GetNonce(),
			}
			signed := transcript.CloneVT()
			if variant == 8 {
				signed.SharedObjectId = "another-object"
			}
			if variant == 9 {
				signed.SenderTransport, signed.ReceiverTransport = signed.ReceiverTransport, signed.SenderTransport
				signed.SenderNonce, signed.ReceiverNonce = signed.ReceiverNonce, signed.SenderNonce
			}
			data, err := signed.MarshalVT()
			if err != nil {
				return err
			}
			signContext, key := authenticationContext, keys[1]
			if variant == 10 {
				signContext = "another-context"
			}
			if variant == 13 {
				key = keys[2]
			}
			proof, err = peer.NewSignature(signContext, key, hash.RecommendedHashType, data, true)
			if err != nil {
				return err
			}
			if variant == 7 {
				proof = nil
			}
			if _, err := exchangeMessage(session, remoteTransport < localTransport, &SOSyncMessage{Body: &SOSyncMessage_Proof{Proof: proof}}); err != nil {
				return err
			}
			if variant == 19 {
				return right.Close()
			}
			authorization := &SOSyncMessage{Body: &SOSyncMessage_Authorization{Authorization: &SOSyncAuthorization{Accepted: variant != 11 && variant != 20}}}
			if variant == 12 {
				authorization = &SOSyncMessage{}
			}
			_, err = exchangeMessage(session, remoteTransport < localTransport, authorization)
			return err
		}()
	}()
	remote, authErr := sync.authenticate(ctx, stream_packet.NewSession(observed, 64*1024), localTransport, remoteTransport)
	left.Close()
	<-remoteDone
	if ctx.Err() != nil {
		t.Fatal("authentication scenario exceeded its deadline")
	}
	if variant == 0 && authErr != nil {
		t.Fatalf("valid authentication failed: %v", authErr)
	}

	var arena fastjson.Arena
	input := arena.NewObject()
	input.Set("localTransport", arena.NewString(string(localTransport)))
	input.Set("remoteTransport", arena.NewString(string(remoteTransport)))
	input.Set("localPeer", arena.NewString(sync.localObjectPeerID.String()))
	keyPeer := arena.NewNull()
	if sync.localObjectKey != nil {
		id, err := peer.IDFromPrivateKey(sync.localObjectKey)
		if err == nil {
			keyPeer = arena.NewString(id.String())
		}
	}
	input.Set("keyPeer", keyPeer)
	for _, key := range []string{"nonceOK", "challengeOK", "signOK", "stateOK"} {
		input.Set(key, leanSyncBool(&arena, true))
	}
	input.Set("remoteNonceSize", arena.NewNumberInt(len(remoteNonce)))
	input.Set("noncesDistinct", leanSyncBool(&arena, !bytes.Equal(localNonce, remoteNonce)))
	input.Set("proofExchangeOK", leanSyncBool(&arena, variant != 18))
	input.Set("proof", leanSyncProof(&arena, transcript, proof))
	input.Set("participants", leanSyncParticipants(&arena, state.GetConfig().GetParticipants()))
	input.Set("hash", arena.NewString(hex.EncodeToString(state.GetConfig().GetConfigChainHash())))
	input.Set("authorizationOK", leanSyncBool(&arena, variant != 19))
	remoteAuthorization := arena.NewNull()
	if variant != 12 {
		remoteAuthorization = leanSyncBool(&arena, variant != 11 && variant != 20)
	}
	input.Set("remoteAuthorization", remoteAuthorization)
	input.Set("observer", leanSyncBool(&arena, sync.peerAdmission != nil))
	request, expected, result := arena.NewObject(), arena.NewObject(), arena.NewObject()
	request.Set("op", arena.NewString("authenticateSync"))
	request.Set("input", input)
	result.Set("ok", leanSyncBool(&arena, authErr == nil))
	result.Set("remote", arena.NewString(remote.String()))
	admission := arena.NewNull()
	if notification != nil {
		admission = leanSyncBool(&arena, *notification)
	}
	result.Set("admission", admission)
	expected.Set("ok", leanSyncBool(&arena, authErr == nil))
	expected.Set("authentication", result)
	for len(observed.messages) != 0 {
		message := <-observed.messages
		if message.GetChallenge() == nil && message.GetProof() == nil && message.GetAuthorization() == nil {
			t.Fatal("authentication emitted a data frame")
		}
	}
	name := "authenticateSync seed " + strconv.FormatUint(seed, 10) + " variant " + strconv.Itoa(variant)
	cases := []leanSyncCase{{name: name, request: request.MarshalTo(nil), expected: expected.MarshalTo(nil)}}
	if transcript != nil {
		verifiedPeer, err := verifyParticipantProof(transcript, proof)
		request, expected := arena.NewObject(), arena.NewObject()
		request.Set("op", arena.NewString("verifySyncProof"))
		request.Set("proof", leanSyncProof(&arena, transcript, proof))
		expected.Set("ok", leanSyncBool(&arena, err == nil))
		identity := arena.NewNull()
		if err == nil {
			identity = arena.NewString(verifiedPeer.String())
		}
		expected.Set("remote", identity)
		cases = append(cases, leanSyncCase{name: "verifySyncProof " + name, request: request.MarshalTo(nil), expected: expected.MarshalTo(nil)})
	}
	return cases
}
