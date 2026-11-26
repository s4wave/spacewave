package sobject_sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// ErrAccessDenied means a participant no longer has readable object access.
var ErrAccessDenied = errors.New("shared object synchronization access denied")

// authenticationContext separates stream proofs from all other peer signatures.
const authenticationContext = "spacewave shared object synchronization participant proof v2"

// authenticationNonceSize is the fresh challenge entropy for each endpoint.
const authenticationNonceSize = 32

// authenticate binds possession of a readable participant key to both transport
// identities and fresh challenges. It returns no state or history to either peer.
func (s *SOSync) authenticate(ctx context.Context, sess *stream_packet.Session, localTransport, remoteTransport peer.ID) (peer.ID, error) {
	// The stream owner supplies transport identities; claims on the wire cannot replace them.
	if localTransport == "" || remoteTransport == "" || localTransport == remoteTransport {
		return "", errors.New("distinct authenticated transport identities are required")
	}
	if s.localObjectKey == nil {
		return "", errors.New("participant signing key is required")
	}
	localID, err := peer.IDFromPrivateKey(s.localObjectKey)
	if err != nil || localID != s.localObjectPeerID {
		return "", errors.New("participant signing key does not match the object identity")
	}

	// Ordering avoids relying on transport buffering for two simultaneous writers.
	sendFirst := localTransport < remoteTransport
	localNonce := make([]byte, authenticationNonceSize)
	if _, err := rand.Read(localNonce); err != nil {
		return "", err
	}
	incoming, err := exchangeMessage(sess, sendFirst, &SOSyncMessage{
		Body: &SOSyncMessage_Challenge{Challenge: &SOSyncChallenge{Nonce: localNonce}},
	})
	if err != nil {
		return "", err
	}
	remoteNonce := incoming.GetChallenge().GetNonce()
	if len(remoteNonce) != authenticationNonceSize || bytes.Equal(localNonce, remoteNonce) {
		return "", errors.New("invalid synchronization challenge")
	}

	// The signature's public key identifies the participant without another identity claim.
	transcript := &SOSyncAuthTranscript{
		SharedObjectId: s.soID, SenderTransport: []byte(localTransport), ReceiverTransport: []byte(remoteTransport),
		SenderNonce: localNonce, ReceiverNonce: remoteNonce,
	}
	data, err := transcript.MarshalVT()
	if err != nil {
		return "", err
	}
	proof, err := peer.NewSignature(authenticationContext, s.localObjectKey, hash.RecommendedHashType, data, true)
	if err != nil {
		return "", err
	}

	// Exchange proofs and verify the remote participant's signature.
	incoming, err = exchangeMessage(sess, sendFirst, &SOSyncMessage{Body: &SOSyncMessage_Proof{Proof: proof}})
	if err != nil {
		return "", err
	}
	transcript.SenderTransport, transcript.ReceiverTransport = transcript.ReceiverTransport, transcript.SenderTransport
	transcript.SenderNonce, transcript.ReceiverNonce = transcript.ReceiverNonce, transcript.SenderNonce
	remoteID, err := verifyParticipantProof(transcript, incoming.GetProof())
	if err != nil {
		return "", err
	}

	// Both endpoints acknowledge admission before either endpoint discloses data.
	state, err := s.soHost.GetHostState(ctx)
	if err != nil {
		return "", err
	}
	admissionErr := s.authorizeParticipants(state, remoteID)
	accepted := admissionErr == nil
	incoming, err = exchangeMessage(sess, sendFirst, &SOSyncMessage{
		Body: &SOSyncMessage_Authorization{Authorization: &SOSyncAuthorization{Accepted: accepted}},
	})
	if err != nil {
		return "", err
	}

	// Report the exchange outcome and surface the local admission error.
	authorization := incoming.GetAuthorization()
	if authorization == nil {
		return "", errors.New("expected synchronization admission response")
	}
	if accepted {
		s.observer.PeerAdmission(remoteID, authorization.GetAccepted())
	}

	// A participant that refused admission this side granted may hold a
	// config older than this side's, so offer it the newer changes.
	if errors.Is(admissionErr, ErrAccessDenied) && authorization.GetAccepted() {
		s.receiveConfigSuffix(ctx, sess)
	}
	if admissionErr != nil {
		return remoteID, admissionErr
	}
	if !authorization.GetAccepted() {
		s.sendConfigSuffix(ctx, sess, state)
		return "", ErrAccessDenied
	}
	return remoteID, nil
}

// sendConfigSuffix sends the newest retained config changes, ending at the
// held head, to an admitted participant that refused this side. It is best
// effort: the stream ends with the refusal either way.
func (s *SOSync) sendConfigSuffix(ctx context.Context, sess *stream_packet.Session, state *sobject.SOState) {
	// Walk back from the held head, newest first, while the frame has room.
	var changes []*sobject.SOConfigChange
	size := 0
	head := state.GetConfig().GetConfigChainHash()
	for len(head) != 0 && len(changes) < maxHistoryPageEntries {
		change, err := s.soHost.ReadConfigEntry(ctx, head)
		if err != nil {
			break
		}
		// Leave room for the field tag and length prefix of each change.
		size += change.SizeVT() + 8
		if size > authMessageSize-64 {
			break
		}
		changes = append(changes, change)
		head = change.GetPreviousHash()
	}
	if len(changes) == 0 {
		return
	}
	slices.Reverse(changes)

	// The refusing side may be an older build that has already closed.
	_ = sess.SendMsg(&SOSyncMessage{
		Body: &SOSyncMessage_ConfigSuffix{ConfigSuffix: &SOSyncConfigSuffix{Changes: changes}},
	})
}

// receiveConfigSuffix adopts config changes from a participant this side
// refused, so the next stream can admit it if the changes add it. The sender
// may be an older build that sends nothing and closes the stream.
func (s *SOSync) receiveConfigSuffix(ctx context.Context, sess *stream_packet.Session) {
	// Read at most one message, bounded by the authentication deadline.
	incoming := &SOSyncMessage{}
	if err := sess.RecvMsg(incoming); err != nil {
		return
	}
	changes := incoming.GetConfigSuffix().GetChanges()
	if len(changes) == 0 {
		return
	}

	// Adopt only changes that extend the held head under its own authority.
	adopted, err := s.soHost.ImportPeerConfig(ctx, changes, s.localObjectPeerID, s.validateSnapshotAccess)
	if err != nil {
		s.le.WithError(err).Warn("refused config changes from a refused participant")
		return
	}
	if adopted != 0 {
		s.le.WithField("changes", adopted).Info("adopted config changes from a refused participant")
	}
}

// verifyParticipantProof verifies the exact directional transcript and returns its signer.
func verifyParticipantProof(transcript *SOSyncAuthTranscript, proof *peer.Signature) (peer.ID, error) {
	// Resolve the proof's public key.
	if proof == nil {
		return "", errors.New("participant proof is required")
	}
	pub, err := proof.ParsePubKey()
	if err != nil || pub == nil {
		return "", errors.New("participant proof public key is invalid")
	}

	// Verify the signature over the transcript and return its signer.
	data, err := transcript.MarshalVT()
	if err != nil {
		return "", err
	}
	valid, err := proof.VerifyWithPublic(authenticationContext, pub, data)
	if err != nil || !valid {
		return "", errors.New("participant proof signature is invalid")
	}
	return peer.IDFromPublicKey(pub)
}

// authorizeParticipants evaluates both endpoint roles under the latest held authority.
func (s *SOSync) authorizeParticipants(state *sobject.SOState, remoteID peer.ID) error {
	// Require both endpoints to hold a readable role.
	cfg := state.GetConfig()
	var localReadable, remoteReadable bool
	for _, participant := range cfg.GetParticipants() {
		if !sobject.CanReadState(participant.GetRole()) {
			continue
		}
		localReadable = localReadable || participant.GetPeerId() == s.localObjectPeerID.String()
		remoteReadable = remoteReadable || participant.GetPeerId() == remoteID.String()
	}
	if !localReadable || !remoteReadable {
		return ErrAccessDenied
	}

	// Require a resolvable configuration history for catch-up.
	if len(cfg.GetConfigChainHash()) == 0 {
		return sobject.ErrConfigHistoryUnavailable
	}
	return nil
}

// exchangeMessage exchanges one frame in transport-identity order.
func exchangeMessage(sess *stream_packet.Session, sendFirst bool, outgoing *SOSyncMessage) (*SOSyncMessage, error) {
	incoming := &SOSyncMessage{}
	if sendFirst {
		if err := sess.SendMsg(outgoing); err != nil {
			return nil, err
		}
		if err := sess.RecvMsg(incoming); err != nil {
			return nil, err
		}
	} else {
		if err := sess.RecvMsg(incoming); err != nil {
			return nil, err
		}
		if err := sess.SendMsg(outgoing); err != nil {
			return nil, err
		}
	}
	return incoming, nil
}
