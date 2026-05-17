package s4wave_secret

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// ReadPayloadChallengeSignatureContext is the signature context for Secret payload read challenges.
const ReadPayloadChallengeSignatureContext = "spacewave secret 2026-05-10 read payload challenge v1"

type payloadReadChallenge struct {
	data      []byte
	challenge *ReadPayloadChallenge
	expiresAt time.Time
}

// ReadSecretPayloadForPeer reads a Secret payload after checking kind and reader grant.
func ReadSecretPayloadForPeer(
	ctx context.Context,
	b bus.Bus,
	secret *Secret,
	expectedKind string,
	readerPeerID string,
) (*SecretPayload, error) {
	if readerPeerID == "" {
		return nil, peer.ErrEmptyPeerID
	}
	if _, err := peer.IDB58Decode(readerPeerID); err != nil {
		return nil, err
	}
	if secret == nil || secret.GetRef() == nil {
		return nil, ErrMissingSecretRef
	}
	if expectedKind != "" && secret.GetKind() != expectedKind {
		return nil, ErrSecretKindMismatch
	}
	r := &SecretResource{b: b}
	if err := r.checkReaderGrant(ctx, secret, readerPeerID); err != nil {
		return nil, err
	}
	return ReadSecretPayload(ctx, b, secret)
}

// BeginReadPayload starts a peer-authenticated Secret payload read.
func (r *SecretResource) BeginReadPayload(ctx context.Context, req *BeginReadPayloadRequest) (*BeginReadPayloadResponse, error) {
	readerPeerID := req.GetReaderPeerId()
	if readerPeerID == "" {
		return nil, peer.ErrEmptyPeerID
	}
	if _, err := peer.IDB58Decode(readerPeerID); err != nil {
		return nil, err
	}

	secret, err := r.readSecret(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetExpectedKind() != "" && secret.GetKind() != req.GetExpectedKind() {
		return nil, ErrSecretKindMismatch
	}
	if err := r.checkReaderGrant(ctx, secret, readerPeerID); err != nil {
		return nil, err
	}

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(r.challengeTTL)
	challenge := &ReadPayloadChallenge{
		ChallengeId:          ulid.NewULID(),
		ReaderPeerId:         readerPeerID,
		ObjectKey:            r.objKey,
		SecretKind:           secret.GetKind(),
		ExpectedKind:         req.GetExpectedKind(),
		NestedSharedObjectId: secret.GetNestedSharedObjectId(),
		Nonce:                nonce,
		ExpiresAt:            timestamppb.New(expiresAt),
	}
	challengeData, err := challenge.MarshalVT()
	if err != nil {
		return nil, err
	}

	r.challengeMu.Lock()
	if r.challenges == nil {
		r.challenges = make(map[string]*payloadReadChallenge)
	}
	for id, entry := range r.challenges {
		if entry == nil || !time.Now().Before(entry.expiresAt) {
			delete(r.challenges, id)
		}
	}
	r.challenges[challenge.GetChallengeId()] = &payloadReadChallenge{
		data:      append([]byte(nil), challengeData...),
		challenge: challenge.CloneVT(),
		expiresAt: expiresAt,
	}
	r.challengeMu.Unlock()

	return &BeginReadPayloadResponse{
		ChallengeId: challenge.GetChallengeId(),
		Challenge:   challengeData,
		ExpiresAt:   timestamppb.New(expiresAt),
		Secret:      secret.CloneVT(),
	}, nil
}

// ReadPayload completes a peer-authenticated Secret payload read.
func (r *SecretResource) ReadPayload(ctx context.Context, req *ReadPayloadRequest) (*ReadPayloadResponse, error) {
	entry, err := r.takePayloadReadChallenge(req.GetChallengeId())
	if err != nil {
		return nil, err
	}
	readerID, err := peer.IDB58Decode(entry.challenge.GetReaderPeerId())
	if err != nil {
		return nil, err
	}
	readerPub, err := readerID.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	sig := req.GetSignature()
	if sig == nil {
		return nil, peer.ErrSignatureInvalid
	}
	if sigPub, err := sig.ParsePubKey(); err != nil {
		return nil, err
	} else if sigPub != nil && !readerID.MatchesPublicKey(sigPub) {
		return nil, peer.ErrSignatureInvalid
	}
	ok, err := sig.VerifyWithPublic(ReadPayloadChallengeSignatureContext, readerPub, entry.data)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, peer.ErrSignatureInvalid
	}

	secret, err := r.readSecret(ctx)
	if err != nil {
		return nil, err
	}
	if entry.challenge.GetExpectedKind() != "" && secret.GetKind() != entry.challenge.GetExpectedKind() {
		return nil, ErrSecretKindMismatch
	}
	if secret.GetKind() != entry.challenge.GetSecretKind() ||
		secret.GetNestedSharedObjectId() != entry.challenge.GetNestedSharedObjectId() {
		return nil, ErrPayloadAccessDenied
	}
	if err := r.checkReaderGrant(ctx, secret, entry.challenge.GetReaderPeerId()); err != nil {
		return nil, err
	}
	payload, err := ReadSecretPayload(ctx, r.b, secret)
	if err != nil {
		return nil, err
	}
	return &ReadPayloadResponse{Payload: payload}, nil
}

func (r *SecretResource) takePayloadReadChallenge(challengeID string) (*payloadReadChallenge, error) {
	if challengeID == "" {
		return nil, ErrReadChallengeNotFound
	}
	r.challengeMu.Lock()
	defer r.challengeMu.Unlock()
	entry, ok := r.challenges[challengeID]
	if ok {
		delete(r.challenges, challengeID)
	}
	if !ok || entry == nil {
		return nil, ErrReadChallengeNotFound
	}
	if !time.Now().Before(entry.expiresAt) {
		return nil, ErrReadChallengeExpired
	}
	return entry, nil
}

func (r *SecretResource) checkReaderGrant(ctx context.Context, secret *Secret, readerPeerID string) error {
	if secret == nil || secret.GetRef() == nil {
		return ErrMissingSecretRef
	}
	so, soRef, err := sobject.ExMountSharedObject(ctx, r.b, secret.GetRef(), false, nil)
	if err != nil {
		return err
	}
	defer soRef.Release()

	ih, ok := so.(sobject.InviteHost)
	if !ok {
		return ErrPayloadAccessDenied
	}
	state, err := ih.GetSOHost().GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get secret shared object state")
	}
	cfg := state.GetConfig()
	if cfg == nil {
		return ErrPayloadAccessDenied
	}
	var readable bool
	for _, participant := range cfg.GetParticipants() {
		if participant.GetPeerId() == readerPeerID && sobject.CanReadState(participant.GetRole()) {
			readable = true
			break
		}
	}
	if !readable {
		return ErrPayloadAccessDenied
	}
	for _, grant := range state.GetRootGrants() {
		if grant.GetPeerId() != readerPeerID {
			continue
		}
		if err := grant.ValidateSignature(so.GetSharedObjectID(), cfg.GetParticipants()); err != nil {
			return ErrPayloadAccessDenied
		}
		return nil
	}
	return ErrPayloadAccessDenied
}
