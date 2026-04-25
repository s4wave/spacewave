package resource_session

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/util/routine"
	webrtc "github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// localPairingState holds the ManualSignalTransport between RPC calls.
type localPairingState struct {
	mu        sync.Mutex
	transport *s4wave_session.ManualSignalTransport
	waitLink  *routine.RoutineContainer
}

// getOrInitLocalPairing returns the session's local pairing state, creating it
// lazily. The transport is stored so the offerer can call CreateLocalPairingOffer
// and later AcceptLocalPairingAnswer on the same PeerConnection.
func (r *SessionResource) getOrInitLocalPairing() *localPairingState {
	r.localPairingMu.Lock()
	defer r.localPairingMu.Unlock()
	if r.localPairing == nil {
		r.localPairing = &localPairingState{
			waitLink: routine.NewRoutineContainer(
				routine.WithExitCb(func(err error) {
					if err != nil && err != context.Canceled {
						r.le.WithError(err).Warn("local pairing wait-link routine exited with error")
					}
				}),
			),
		}
	}
	return r.localPairing
}

// replaceLocalPairingTransport swaps the manual signal transport and stops any
// outstanding wait-link routine bound to the previous transport.
func (r *SessionResource) replaceLocalPairingTransport(tpt *s4wave_session.ManualSignalTransport) {
	lps := r.getOrInitLocalPairing()
	lps.mu.Lock()
	defer lps.mu.Unlock()

	_, _ = lps.waitLink.SetRoutine(nil)
	if lps.transport != nil && lps.transport != tpt {
		_ = lps.transport.Close()
	}
	lps.transport = tpt
}

// startLocalPairingLinkWaiter starts or replaces the direct-link wait routine.
func (r *SessionResource) startLocalPairingLinkWaiter(remotePeerID peer.ID) error {
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return errors.New("provider account is not local")
	}
	parentCtx := localAcc.GetPairingContext()
	if parentCtx == nil {
		return errors.New("local pairing lifecycle context unavailable")
	}

	lps := r.getOrInitLocalPairing()
	lps.mu.Lock()
	defer lps.mu.Unlock()

	tpt := lps.transport
	if tpt == nil {
		return errors.New("no pending local pairing transport")
	}

	_, _ = lps.waitLink.SetRoutine(func(ctx context.Context) error {
		r.waitLocalPairingLink(ctx, tpt, remotePeerID, localAcc)
		return nil
	})
	lps.waitLink.SetContext(parentCtx, true)
	return nil
}

// defaultICEServers provides a basic STUN server for ICE candidate gathering.
var defaultICEServers = []webrtc.ICEServer{
	{URLs: []string{"stun:stun.l.google.com:19302"}},
}

// CreateLocalPairingOffer generates a WebRTC SDP offer for no-cloud pairing.
func (r *SessionResource) CreateLocalPairingOffer(ctx context.Context, _ *s4wave_session.CreateLocalPairingOfferRequest) (*s4wave_session.CreateLocalPairingOfferResponse, error) {
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	identity, err := p2ptls.NewIdentity(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "create identity")
	}

	localPeerID := r.session.GetPeerId()
	tpt, err := s4wave_session.NewManualSignalTransport(
		r.le.WithField("component", "local-pairing"),
		identity,
		localPeerID,
		defaultICEServers,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create manual signal transport")
	}

	sdp, err := tpt.CreateOffer(ctx)
	if err != nil {
		_ = tpt.Close()
		return nil, errors.Wrap(err, "create offer")
	}

	minified := s4wave_session.MinifySDP(sdp)
	offer := &s4wave_session.LocalPairingOffer{
		Sdp:    minified,
		PeerId: localPeerID.String(),
	}
	encoded, err := s4wave_session.EncodeLocalPairingOffer(offer)
	if err != nil {
		_ = tpt.Close()
		return nil, errors.Wrap(err, "encode offer")
	}

	r.replaceLocalPairingTransport(tpt)

	return &s4wave_session.CreateLocalPairingOfferResponse{OfferPayload: encoded}, nil
}

// AcceptLocalPairingOffer accepts a remote SDP offer and returns an answer.
func (r *SessionResource) AcceptLocalPairingOffer(ctx context.Context, req *s4wave_session.AcceptLocalPairingOfferRequest) (*s4wave_session.AcceptLocalPairingOfferResponse, error) {
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	remoteOffer, err := s4wave_session.DecodeLocalPairingOffer(req.GetOfferPayload())
	if err != nil {
		return nil, errors.Wrap(err, "decode offer")
	}

	identity, err := p2ptls.NewIdentity(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "create identity")
	}

	localPeerID := r.session.GetPeerId()
	tpt, err := s4wave_session.NewManualSignalTransport(
		r.le.WithField("component", "local-pairing"),
		identity,
		localPeerID,
		defaultICEServers,
	)
	if err != nil {
		return nil, errors.Wrap(err, "create manual signal transport")
	}

	answerSDP, err := tpt.AcceptOffer(ctx, remoteOffer.GetSdp())
	if err != nil {
		_ = tpt.Close()
		return nil, errors.Wrap(err, "accept offer")
	}

	minified := s4wave_session.MinifySDP(answerSDP)
	answer := &s4wave_session.LocalPairingAnswer{
		Sdp:    minified,
		PeerId: localPeerID.String(),
	}
	encoded, err := s4wave_session.EncodeLocalPairingAnswer(answer)
	if err != nil {
		_ = tpt.Close()
		return nil, errors.Wrap(err, "encode answer")
	}

	// Store the transport for link establishment (answerer also needs WaitLink).
	r.replaceLocalPairingTransport(tpt)

	// Start link establishment in the background. The WatchPairingStatus
	// stream picks up the status change once the link is ready.
	remotePeerID, err := remoteOffer.ParsePeerID()
	if err != nil {
		return nil, errors.Wrap(err, "decode remote peer ID")
	}
	if err := r.startLocalPairingLinkWaiter(remotePeerID); err != nil {
		return nil, errors.Wrap(err, "start local pairing link wait")
	}

	return &s4wave_session.AcceptLocalPairingOfferResponse{AnswerPayload: encoded}, nil
}

// AcceptLocalPairingAnswer accepts a remote SDP answer to complete the connection.
func (r *SessionResource) AcceptLocalPairingAnswer(ctx context.Context, req *s4wave_session.AcceptLocalPairingAnswerRequest) (*s4wave_session.AcceptLocalPairingAnswerResponse, error) {
	remoteAnswer, err := s4wave_session.DecodeLocalPairingAnswer(req.GetAnswerPayload())
	if err != nil {
		return nil, errors.Wrap(err, "decode answer")
	}

	lps := r.getOrInitLocalPairing()
	lps.mu.Lock()
	tpt := lps.transport
	lps.mu.Unlock()
	if tpt == nil {
		return nil, errors.New("no pending local pairing offer")
	}

	if err := tpt.AcceptAnswer(remoteAnswer.GetSdp()); err != nil {
		return nil, errors.Wrap(err, "accept answer")
	}

	remotePeerID, err := remoteAnswer.ParsePeerID()
	if err != nil {
		return nil, errors.Wrap(err, "decode remote peer ID")
	}

	if err := r.startLocalPairingLinkWaiter(remotePeerID); err != nil {
		return nil, errors.Wrap(err, "start local pairing link wait")
	}

	return &s4wave_session.AcceptLocalPairingAnswerResponse{
		RemotePeerId: remoteAnswer.GetPeerId(),
	}, nil
}

// localPairingLinkTimeout is the maximum time to wait for a direct WebRTC
// link to establish (ICE + DTLS + QUIC handshake).
const localPairingLinkTimeout = 60 * time.Second

// waitLocalPairingLink waits for the WebRTC link to be established and
// feeds it into the pairing state machine for SAS verification. On failure
// or timeout, updates the pairing status so the UI can show an error.
func (r *SessionResource) waitLocalPairingLink(
	parentCtx context.Context,
	tpt *s4wave_session.ManualSignalTransport,
	remotePeerID peer.ID,
	localAcc *provider_local.ProviderAccount,
) {
	ctx, cancel := context.WithTimeout(parentCtx, localPairingLinkTimeout)
	defer cancel()

	lnk, err := tpt.WaitLink(ctx, remotePeerID)
	if err != nil {
		if parentCtx.Err() != nil {
			return
		}
		r.le.WithError(err).Warn("local pairing link failed")
		if ctx.Err() != nil {
			localAcc.SetPairingFailed("direct connection timed out")
		} else {
			localAcc.SetPairingFailed("direct connection failed: " + err.Error())
		}
		return
	}

	r.le.WithField("remote-peer", remotePeerID.String()).Info("local pairing link established")

	privKey := r.session.GetPrivKey()
	if privKey == nil {
		r.le.Warn("local pairing: session is locked")
		localAcc.SetPairingFailed("session is locked")
		_ = lnk.Close()
		return
	}

	localPeerID := r.session.GetPeerId()
	localAcc.OnDirectPairingConnected(remotePeerID, privKey, localPeerID, lnk, tpt.IsOfferer())
}
