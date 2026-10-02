//go:build !tinygo

package resource_session

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/util/routine"
	webrtc "github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// localPairingState holds the ManualSignalTransport between RPC calls.
type localPairingState struct {
	mu           sync.Mutex
	transport    *s4wave_session.ManualSignalTransport
	waitLink     *routine.RoutineContainer
	offerCurrent bool
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
func (r *SessionResource) replaceLocalPairingTransport(tpt *s4wave_session.ManualSignalTransport, offerCurrent bool) {
	// Lock the local pairing state for the swap.
	lps := r.getOrInitLocalPairing()
	lps.mu.Lock()
	defer lps.mu.Unlock()

	// Stop the previous wait-link routine and close a replaced transport.
	_, _ = lps.waitLink.SetRoutine(nil)
	if lps.transport != nil && lps.transport != tpt {
		_ = lps.transport.Close()
	}
	lps.transport = tpt
	lps.offerCurrent = offerCurrent
}

// startLocalPairingLinkWaiter starts or replaces the direct-link wait routine.
func (r *SessionResource) startLocalPairingLinkWaiter(remotePeerID peer.ID) error {
	// Open the pairing engine and take its context.
	engine, err := r.getPairingEngine()
	if err != nil {
		return err
	}
	parentCtx := engine.Context()

	// Lock the local pairing state for the waiter swap.
	lps := r.getOrInitLocalPairing()
	lps.mu.Lock()
	defer lps.mu.Unlock()

	// Require a pending transport before replacing the waiter.
	tpt := lps.transport
	offerCurrent := lps.offerCurrent
	if tpt == nil {
		return errors.New("no pending local pairing transport")
	}

	// A watcher may start before the new link arrives. Retire the previous
	// decision before returning its replacement signaling response.
	engine.Clear()

	// Start the wait-link routine on the pairing engine context.
	_, _ = lps.waitLink.SetRoutine(func(ctx context.Context) error {
		r.waitLocalPairingLink(ctx, tpt, remotePeerID, engine, offerCurrent)
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
	// Refuse an offer while the Session is locked.
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	// Build a P2P TLS identity from the session key.
	identity, err := p2ptls.NewIdentity(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "create identity")
	}
	p2ptls.LogBrowserCurvePreferenceWarning(r.le)

	// Open a manual signal transport for this peer.
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

	// Create the SDP offer, closing the transport on failure.
	sdp, err := tpt.CreateOffer(ctx)
	if err != nil {
		_ = tpt.Close()
		return nil, errors.Wrap(err, "create offer")
	}

	// Encode the minified offer with this peer ID.
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

	// Keep the transport for the later answer.
	r.replaceLocalPairingTransport(tpt, true)

	return &s4wave_session.CreateLocalPairingOfferResponse{OfferPayload: encoded}, nil
}

// AcceptLocalPairingOffer accepts a remote SDP offer and returns an answer.
func (r *SessionResource) AcceptLocalPairingOffer(ctx context.Context, req *s4wave_session.AcceptLocalPairingOfferRequest) (*s4wave_session.AcceptLocalPairingOfferResponse, error) {
	// Refuse an answer while the Session is locked.
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	// Decode the remote pairing offer.
	remoteOffer, err := s4wave_session.DecodeLocalPairingOffer(req.GetOfferPayload())
	if err != nil {
		return nil, errors.Wrap(err, "decode offer")
	}

	// Build a P2P TLS identity from the session key.
	identity, err := p2ptls.NewIdentity(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "create identity")
	}
	p2ptls.LogBrowserCurvePreferenceWarning(r.le)

	// Open a manual signal transport for this peer.
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

	// Accept the remote SDP, closing the transport on failure.
	answerSDP, err := tpt.AcceptOffer(ctx, remoteOffer.GetSdp())
	if err != nil {
		_ = tpt.Close()
		return nil, errors.Wrap(err, "accept offer")
	}

	// Encode the minified answer with this peer ID.
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
	r.replaceLocalPairingTransport(tpt, req.GetOfferCurrentAccount())

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
	// Decode the remote pairing answer.
	remoteAnswer, err := s4wave_session.DecodeLocalPairingAnswer(req.GetAnswerPayload())
	if err != nil {
		return nil, errors.Wrap(err, "decode answer")
	}

	// Take the pending offer transport.
	lps := r.getOrInitLocalPairing()
	lps.mu.Lock()
	tpt := lps.transport
	lps.mu.Unlock()
	if tpt == nil {
		return nil, errors.New("no pending local pairing offer")
	}

	// Apply the remote SDP answer.
	if err := tpt.AcceptAnswer(remoteAnswer.GetSdp()); err != nil {
		return nil, errors.Wrap(err, "accept answer")
	}

	// Decode the remote peer ID from the answer.
	remotePeerID, err := remoteAnswer.ParsePeerID()
	if err != nil {
		return nil, errors.Wrap(err, "decode remote peer ID")
	}

	// Wait for the direct link in the background.
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
	engine *pairing.Engine,
	offerCurrent bool,
) {
	// Bound the ICE, DTLS, and QUIC handshake.
	ctx, cancel := context.WithTimeout(parentCtx, localPairingLinkTimeout)
	defer cancel()

	// Wait for the link, and mark the pairing failed on timeout.
	lnk, err := tpt.WaitLink(ctx, parentCtx, remotePeerID)
	if err != nil {
		if parentCtx.Err() != nil {
			return
		}
		r.le.WithError(err).Warn("local pairing link failed")
		failMsg := "direct connection failed: " + err.Error()
		if ctx.Err() != nil {
			failMsg = "direct connection timed out"
		}
		engine.SetFailed(failMsg)
		return
	}

	// Log the established link.
	r.le.WithField("remote-peer", remotePeerID.String()).Info("local pairing link established")

	// Fail the pairing when the Session is locked.
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		r.le.Warn("local pairing: session is locked")
		engine.SetFailed("session is locked")
		_ = lnk.Close()
		return
	}

	// Hand the link to the pairing engine.
	engine.StartDirect(lnk, tpt.IsOfferer(), offerCurrent)
}
