package webrtc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"runtime/debug"
	"strings"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pion/datachannel"
	webrtc "github.com/pion/webrtc/v4"
	pkgerrors "github.com/pkg/errors"
	"github.com/quic-go/quic-go"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	"github.com/s4wave/spacewave/net/util/rwc"
	"github.com/sirupsen/logrus"
)

// dataChannelID is the channel ID used in webRTC for Quic-Over-WebRTC
var dataChannelID = "bifrost-quic"

// sessionTracker wraps an ongoing connection with a peer.
type sessionTracker struct {
	// w is the transport
	w *WebRTC
	// le is the logger
	le *logrus.Entry
	// key is the string encoding of the peer id.
	key string
	// peerID is the parsed version of the peer id
	peerID peer.ID
	// peerPub is the peer public key
	peerPub crypto.PubKey
	// offerer indicates if we are offering or answering
	offerer bool

	// executionGeneration identifies each execute invocation on this tracker.
	// execution is non-nil only while that invocation is live.
	// w.bcast guards executionGeneration, execution, and link.
	executionGeneration uint64
	execution           *sessionTrackerExecution
	// link contains the current link, if any
	link *Link
	// linkRef holds the transport-owned tracker reference while a published
	// link is live. The transport session, not the signal-ingress lease, owns
	// the established-link reference: retiring the ingress lease must not
	// cancel a tracker whose PeerConnection and datachannel are still in use.
	linkRef *keyed.KeyedRef[string, *sessionTracker]
}

// sessionTrackerExecution is one live invocation of sessionTracker.execute.
type sessionTrackerExecution struct {
	generation     uint64
	rxSignal       chan *incomingSignal
	carriedOfferID []byte
}

// beginExecution publishes a new execution generation.
func (s *sessionTracker) beginExecution() *sessionTrackerExecution {
	var execution *sessionTrackerExecution
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.executionGeneration++
		execution = &sessionTrackerExecution{
			generation: s.executionGeneration,
			rxSignal:   make(chan *incomingSignal),
		}
		s.execution = execution
		broadcast()
	})
	return execution
}

// retireExecution clears a generation and reports its lease retirement.
func (s *sessionTracker) retireExecution(execution *sessionTrackerExecution) {
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.execution != execution {
			return
		}
		s.execution = nil
		s.w.retireSignalIngressLocked(s.key, s, broadcast)
		broadcast()
	})
}

// completeExecution waits for both child routines before retiring the execution.
func (s *sessionTracker) completeExecution(
	execution *sessionTrackerExecution,
	linkDone, xmitDone <-chan struct{},
) {
	if linkDone != nil {
		<-linkDone
	}
	if xmitDone != nil {
		<-xmitDone
	}
	s.retireExecution(execution)
}

// incomingSignal holds a decoded signal until a live tracker accepts it.
type incomingSignal struct {
	sig      *WebRtcSignal
	accepted chan struct{}
}

// accept records that the current live tracker owns the signal.
func (s *incomingSignal) accept() {
	close(s.accepted)
}

// newSessionTracker constructs a new sessionTracker.
func (w *WebRTC) newSessionTracker(peerIDStr string) (keyed.Routine, *sessionTracker) {
	// note: we confirmed that parsePeerID is valid before adding the key
	peerID, peerPub, _ := peer.ParsePeerIDWithPubKey(peerIDStr)
	localPeerIDStr := w.peerID.String()
	offerer := isOfferer(localPeerIDStr, peerIDStr)
	le := w.le.WithField("remote-peer-id", peerIDStr)

	// Construct the tracker with its remote identity and negotiation role.
	sess := &sessionTracker{
		w:       w,
		le:      le,
		key:     peerIDStr,
		peerID:  peerID,
		peerPub: peerPub,
		offerer: offerer,
	}

	return sess.execute, sess
}

// outgoingSignal is a signal queued for the remote peer.
type outgoingSignal struct {
	// sig is the signal to send.
	sig *WebRtcSignal
	// sentCh is closed once sig is sent.
	sentCh chan struct{}
}

// signalOutbox queues signals for the remote peer and sends them in order. A
// queued signal is never replaced: a candidate trickled before a retransmitted
// offer or answer must still reach the remote peer, which never asks for it
// again.
type signalOutbox struct {
	// bcast guards queue.
	bcast broadcast.Broadcast
	// queue holds the signals waiting to be sent, oldest first.
	queue []*outgoingSignal
}

// push queues sig and returns a channel closed once it is sent.
func (o *signalOutbox) push(sig *WebRtcSignal) <-chan struct{} {
	out := &outgoingSignal{sig: sig, sentCh: make(chan struct{})}
	o.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		o.queue = append(o.queue, out)
		broadcast()
	})
	return out.sentCh
}

// executeOutbox sends the signals queued in outbox over sess, in order, until
// ctx ends or a send fails.
func (s *sessionTracker) executeOutbox(
	ctx context.Context,
	outbox *signalOutbox,
	sess signaling.SignalPeerSession,
) error {
	for {
		// Wait for the oldest queued signal.
		var next *outgoingSignal
		var waitCh <-chan struct{}
		outbox.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if len(outbox.queue) != 0 {
				next = outbox.queue[0]
			} else {
				waitCh = getWaitCh()
			}
		})
		if next == nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
				continue
			}
		}

		// Send it, then dequeue it and report it sent.
		if err := s.sendSignal(ctx, sess, next.sig); err != nil {
			return err
		}
		outbox.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			outbox.queue[0] = nil
			outbox.queue = outbox.queue[1:]
		})
		close(next.sentCh)
	}
}

// sendSignal encrypts sig for the remote peer and sends it over sess.
func (s *sessionTracker) sendSignal(
	ctx context.Context,
	sess signaling.SignalPeerSession,
	sig *WebRtcSignal,
) (err error) {
	// Translate signaling panics into errors for the tracker execution.
	defer func() {
		if e := recover(); e != nil {
			err = pkgerrors.Errorf("xmit signal panic: %v\n%s", e, debug.Stack())
		}
	}()

	// Encode the signal for the remote peer before transmission.
	msgEnc, err := EncodeWebRtcSignal(sig, s.peerPub)
	if err != nil {
		return pkgerrors.Wrap(err, "encode web rtc signal")
	}
	defer scrub.Scrub(msgEnc)

	// Send the encrypted signal.
	if err := sess.Send(ctx, msgEnc); err != nil {
		return pkgerrors.Wrap(err, "send signaling message")
	}
	return nil
}

// executeLink executes the quic link over a data channel of peerConn.
func (s *sessionTracker) executeLink(
	ctx context.Context,
	peerConn *webrtc.PeerConnection,
	dcRwc datachannel.ReadWriteCloser,
) error {
	// Packet conn: maximum packet size should be larger than the MTU quic uses.
	// Use one that aligns with one memory page (4096 bytes)
	// Buffer 8 packets at a time.
	// Wrap the data channel as a packet connection with peer addresses.
	localAddr := peer.NewNetAddr(s.w.peerID)
	remoteAddr := peer.NewNetAddr(s.peerID)
	pc := rwc.NewRwcPacketConn(dcRwc, localAddr, remoteAddr)

	// Resolve the QUIC role from the deterministic offerer selection.
	role := "client"
	if s.offerer {
		role = "server"
	}
	s.le.WithField("quic-role", role).Info("webrtc quic phase: data channel ready")

	// Configure QUIC for WebRTC data-channel transport.
	linkOpts := s.w.conf.GetQuic().CloneVT()
	if linkOpts == nil {
		linkOpts = &transport_quic.Opts{}
	}

	// Keep QUIC keepalive enabled for WebRTC links. The link rides a datachannel
	// and can sit idle (a quiet resource stream, a lull between bursts) longer
	// than MaxIdleTimeout; without keepalive PINGs quic-go reaps it with an idle
	// timeout ("no recent network activity") and the link has to re-establish,
	// stalling traffic that depended on it. Keepalive (MaxIdleTimeout/2) holds an
	// otherwise-healthy idle link open. Unlike the websocket transport, the
	// datachannel has no lower-layer keepalive of its own.
	linkOpts.DisableKeepAlive = false
	linkOpts.DisablePathMtuDiscovery = true
	linkOpts.MaxIdleTimeoutDur = "60s"

	// Negotiate QUIC with the offerer listening and answerer dialing.
	// This evenly splits responsibilities between the peers.
	//
	// Assuming peer A is the offerer and B the answerer:
	// 1. A -> B: offer SDP
	// 2. B -> A: answer SDP
	// 3. A -> B: ICE candidate
	// 4. B -> A: ICE candidate
	// 5. B -> A: Dial quic (mTLS)
	// 6. A -> B: Answer dial quic
	var sess *quic.Conn
	var err error
	s.le.WithField("quic-role", role).Info("webrtc quic phase: handshake starting")
	if s.offerer {
		sess, err = transport_quic.ListenSession(
			ctx,
			s.le,
			linkOpts,
			pc,
			s.w.identity,
			s.peerID,
		)
	} else {
		sess, _, err = transport_quic.DialSession(
			ctx,
			s.le,
			linkOpts,
			pc,
			s.w.identity,
			remoteAddr,
			s.peerID,
		)
	}
	if err != nil {
		s.le.WithFields(logrus.Fields{
			"error":     err,
			"quic-role": role,
		}).Warn("webrtc quic phase: handshake failed")
		return pkgerrors.Wrap(err, "construct quic session")
	}
	s.le.WithField("quic-role", role).Info("webrtc quic phase: handshake complete")

	// Prepare link-close signaling before constructing the QUIC link.
	errCh := make(chan error, 1)
	var nextLink *Link
	var wasClosed atomic.Bool

	// publishedRef is the transport-owned tracker reference this publication
	// established or reused. The link closure releases it only while it is
	// still the tracker's owned reference.
	var publishedRef *keyed.KeyedRef[string, *sessionTracker]
	closed := func() {
		// Retire the published link and its tracker reference under the transport lock.
		if wasClosed.Swap(true) {
			return
		}
		s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if s.link == nextLink {
				s.link = nil
			}
			if publishedRef != nil && s.linkRef == publishedRef {
				s.linkRef = nil
				publishedRef.Release()
			}
			broadcast()
		})
		go s.w.handler.HandleLinkLost(nextLink)
		_ = dcRwc.Close()
		errCh <- io.EOF
	}

	// Construct the link and report any construction failure.
	quicLink, err := transport_quic.NewLink(
		ctx,
		s.le,
		&transport_quic.Opts{},
		s.w.GetUUID(),
		s.w.peerID,
		localAddr,
		sess,
		closed,
	)
	if err != nil {
		return pkgerrors.Wrap(err, "construct quic link")
	}
	nextLink = &Link{Link: quicLink, pc: peerConn}
	s.le.WithField("quic-role", role).Info("webrtc quic phase: link constructed")

	// Publish the link under the broadcast lock and notify the handler.
	// Acquire the transport-owned tracker reference for the live link so
	// retiring the signal-ingress lease cannot cancel an established session.
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.link = nextLink
		if s.linkRef == nil {
			ref, _, _, err := s.w.addSessionTrackerRef(s.key)
			if err != nil {
				s.le.WithError(err).Warn("webrtc quic phase: transport link reference not acquired")
			} else {
				s.linkRef = ref
			}
		}
		publishedRef = s.linkRef
		broadcast()
	})
	s.w.handler.HandleLinkEstablished(nextLink)
	s.le.WithField("quic-role", role).Info("webrtc quic phase: link published")

	// Close the link if this execution exits before its close callback.
	defer func() {
		if !wasClosed.Load() {
			go nextLink.Close()
		}
	}()

	// Wait for cancellation or link closure.
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// session contains the state for a single ongoing PeerConnection.
type session struct {
	// t is the session tracker
	t *sessionTracker
	// pc is the peer connection
	pc *webrtc.PeerConnection

	// bcast guards the following fields
	bcast broadcast.Broadcast

	// NOTE: these fields are managed by pion-webrtc.

	// fatalErr contains any fatal error
	fatalErr error
	// connState is the current connection state
	connState webrtc.PeerConnectionState

	// localSeqno is the local session sequence number
	// incremented when negotiation is needed
	// if offerer: transmit sdp offer when changed
	// if !offerer: transmit request_offer=localSeqno when changed
	localSeqno uint64

	// rxOfferID is the SHA-256 digest of the exact offer SDP bytes of the
	// remote offer currently applied to this session, when set. It records
	// the active generation identity for the signal fence.
	rxOfferID []byte

	// rxOfferAnswerSDP holds the exact local answer SDP bytes transmitted for
	// rxOfferID. Pion augments LocalDescription with gathered ICE candidates,
	// so duplicate-offer replay uses these bytes to preserve the generation.
	rxOfferAnswerSDP string

	// pendingOfferID is the SHA-256 digest of the local offer SDP bytes most
	// recently transmitted by this session, when we are the offerer. Answers
	// whose offer_id does not match it are dropped before Pion.
	pendingOfferID []byte

	// pendingOfferSDP holds the exact local offer SDP bytes whose digest is
	// pendingOfferID. Pion augments LocalDescription with gathered ICE
	// candidates after the initial transmission, so the outstanding-offer
	// retransmit path replays these bytes to keep the generation identity
	// stable across retransmissions.
	pendingOfferSDP string

	// retiredOfferIDs holds the digests of remote offers this session already
	// applied and replaced with a newer generation. A replayed copy of a
	// retired offer is dropped before Pion so a stale description can never
	// re-enter negotiation. The set lives and dies with the session; it is
	// never retained across sessions.
	retiredOfferIDs map[string]struct{}

	// pendingRemoteIce holds remote ICE candidates buffered before their
	// generation could accept them, tagged with the offer id they arrived
	// under. A tracker restart hands this buffer to the successor with the
	// session itself, so an in-flight trickle survives the restart.
	pendingRemoteIce []pendingRemoteCandidate

	// localIceCandidates contains the current list of local ice candidates.
	localIceCandidates []*webrtc.ICECandidateInit
	// localIceCandidatesComplete indicates the ice candidate list is complete.
	localIceCandidatesComplete bool

	// dc is the data channel
	dc *webrtc.DataChannel
	// dcOpen indicates the data channel is open.
	dcOpen bool
	// dcRwc is the data channel read/write/closer
	// nil unless dcOpen=true
	dcRwc datachannel.ReadWriteCloser

	// NOTE: these fields are managed by execute().
}

// newSession constructs a new session.
func (s *sessionTracker) newSession(ctx context.Context) (*session, <-chan struct{}, error) {
	// Wrap peer connection callbacks so registration failures return errors.
	setCallback := func(name string, cb func()) (err error) {
		defer func() {
			if e := recover(); e != nil {
				if recoveredErr, ok := e.(error); ok {
					err = pkgerrors.Wrap(recoveredErr, name)
					return
				}
				err = pkgerrors.Errorf("%s: recovered with non-error value: %v", name, e)
			}
		}()
		cb()
		return nil
	}

	// Create the peer connection.
	pc, err := s.w.newPeerConnection(ctx)
	if err != nil {
		return nil, nil, pkgerrors.Wrap(err, "create peer connection")
	}

	// Bind the new peer connection to its session state.
	sess := &session{t: s, pc: pc}

	// Capture the first session notification before callbacks can change state.
	var waitCh <-chan struct{}
	sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	// Create the negotiated data channel and attach it to the session.
	var dc *webrtc.DataChannel
	var createErr error
	if err := setCallback("initialize negotiated data channel", func() {
		dc, createErr = sess.createDataChannel(
			pc.OnNegotiationNeeded,
			pc.CreateDataChannel,
		)
	}); err != nil {
		_ = pc.Close()
		return nil, nil, err
	}
	if createErr != nil {
		_ = pc.Close()
		return nil, nil, pkgerrors.Wrap(createErr, "create data channel")
	}
	sess.dc = dc

	// DataChannel callbacks
	if err := setCallback("register data channel onopen", func() {
		dc.OnOpen(sess.onDataChannelOpen)
	}); err != nil {
		_ = dc.Close()
		_ = pc.Close()
		return nil, nil, err
	}
	if err := setCallback("register data channel onclose", func() {
		dc.OnClose(sess.onDataChannelClose)
	}); err != nil {
		_ = dc.Close()
		_ = pc.Close()
		return nil, nil, err
	}

	// When an ICE candidate is available send to the other Pion instance
	// the other Pion instance will add this candidate by calling AddICECandidate
	//
	// This begins being called once SetRemoteDescription is called.
	if err := setCallback("register peer connection onconnectionstatechange", func() {
		pc.OnConnectionStateChange(sess.onConnectionStateChange)
	}); err != nil {
		_ = dc.Close()
		_ = pc.Close()
		return nil, nil, err
	}
	if err := setCallback("register peer connection onicecandidate", func() {
		pc.OnICECandidate(sess.onIceCandidate)
	}); err != nil {
		_ = dc.Close()
		_ = pc.Close()
		return nil, nil, err
	}

	// pc.OnDataChannel(sess.onDataChannel)
	if s.w.GetVerbose() {
		s.le.Debug("session constructed")
	}

	return sess, waitCh, nil
}

// createDataChannel installs negotiation ownership before creating the channel.
func (s *session) createDataChannel(
	onNegotiationNeeded func(func()),
	createDataChannel func(string, *webrtc.DataChannelInit) (*webrtc.DataChannel, error),
) (*webrtc.DataChannel, error) {
	// Register the session negotiation callback before creating the channel.
	onNegotiationNeeded(s.onNegotiationNeeded)

	// Configure the shared data channel label and negotiated mode.
	negotiated := true
	protocol := dataChannelID

	// The channel carries QUIC packets, which QUIC orders and retransmits
	// itself, so the channel neither orders nor retransmits them. A reliable
	// channel would hold every later packet behind an SCTP retransmission
	// after a loss, stalling even the unreliable streams for a round trip.
	ordered := false
	var maxRetransmits uint16
	var channelID uint16 = 1
	return createDataChannel(dataChannelID, &webrtc.DataChannelInit{
		// We use the same channel label on both sides and set Negotiated: true.
		// This avoids sending redundant info via the OnDataChannel callback.
		Negotiated:     &negotiated,
		Protocol:       &protocol,
		ID:             &channelID,
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
}

func (s *session) onNegotiationNeeded() {
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.localSeqno++
		broadcast()
		if s.t.w.GetVerbose() {
			s.t.le.
				WithField("local-seqno", s.localSeqno).
				Debug("negotiation is needed")
		}
	})
}

// acceptIncomingSignalLocked accepts sig only while this session is live.
// The caller must hold s.bcast.
func (s *session) acceptIncomingSignalLocked(sig *incomingSignal) {
	if sig == nil || s.fatalErr != nil {
		return
	}
	switch s.connState {
	case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed:
		return
	}
	sig.accept()
}

// failWithErr fences signal acceptance before exposing a routine error.
func (s *session) failWithErr(errCh chan<- error, err error) {
	// Ignore successful exits and cancellation when recording session failures.
	if err == nil || err == context.Canceled {
		return
	}

	// Record the first fatal session error and notify state watchers.
	var recorded bool
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.fatalErr == nil {
			s.fatalErr = err
			recorded = true
			broadcast()
		}
	})
	if !recorded {
		return
	}

	// Report the recorded session failure without blocking its callback.
	select {
	case errCh <- err:
	default:
	}
}

func (s *session) onIceCandidate(c *webrtc.ICECandidate) {
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Publish completion when the ICE agent finishes gathering candidates.
		if c == nil {
			if !s.localIceCandidatesComplete {
				if s.t.w.GetVerbose() {
					s.t.le.Debug("local ice candidates complete")
				}
				s.localIceCandidatesComplete = true
				broadcast()
			}
			return
		}

		// Retain the gathered ICE candidate and notify negotiation watchers.
		cJson := c.ToJSON()
		s.localIceCandidates = append(s.localIceCandidates, &cJson)
		s.localIceCandidatesComplete = false
		if s.t.w.GetVerbose() {
			s.t.le.Debugf("local ice candidate added: %v", c.String())
		}
		broadcast()
	})
}

func (s *session) onConnectionStateChange(connState webrtc.PeerConnectionState) {
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.connState != connState {
			s.t.le.Debugf("connection state changed: %v", connState.String())
			s.connState = connState
			broadcast()
		}
	})
}

// onDataChannelOpen is called when the data channel opens.
func (s *session) onDataChannelOpen() {
	if s.t.w.GetVerbose() {
		s.t.le.Debugf("data channel open: %v", s.dc.Label())
	}

	// We set DetachDataChannels in the WebRTC settings engine.
	rwc, err := s.dc.Detach()
	if err != nil {
		s.t.le.WithError(err).Warn("pion data-channel detach failed")
		return
	}
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.dcOpen = true
		s.dcRwc = rwc
		broadcast()
	})
}

// onDataChannelClose is called when the data channel closes.
func (s *session) onDataChannelClose() {
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.dcOpen {
			s.dcOpen = false
			s.dcRwc = nil
			broadcast()
		}
	})
}

// close closes the session and releases its generation fence state.
func (s *session) close() {
	// fatalErr and connState are written by pion callback goroutines and the
	// child routine exit paths; snapshot them under the lock.
	var fatalErr error
	var connState webrtc.PeerConnectionState
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		fatalErr = s.fatalErr
		connState = s.connState
	})

	// An in-flight negotiation survives this tracker: hand the session to the
	// peer's ingress lease for adoption by a successor instead of disposing an
	// outstanding offer. The successor adopts the connection with the same
	// pending offer id and retransmits the identical offer via the outstanding-
	// offer path in transmitLocalNegotiation, so an answer already sent or
	// still in flight correlates instead of dying with the retired generation.
	// Only a session holding a live outstanding local offer is adoptable; a
	// session with a recorded fatal error, a finished or unusable connection,
	// or no outstanding offer cannot continue negotiation and is disposed so
	// its successor mints a fresh generation on a new connection.
	localDesc := s.pc.LocalDescription()
	if fatalErr == nil && s.t != nil && s.t.offerer && len(s.pendingOfferID) > 0 &&
		localDesc != nil && localDesc.SDP != "" &&
		s.pc.SignalingState() == webrtc.SignalingStateHaveLocalOffer &&
		connState != webrtc.PeerConnectionStateConnected &&
		connState != webrtc.PeerConnectionStateFailed &&
		connState != webrtc.PeerConnectionStateClosed &&
		s.t.w.stashAdoptableSession(s.t.key, s) {
		return
	}
	s.dispose()
}

// dispose releases the session's generation fence state and closes the peer
// connection exactly once. It never stashes: the ingress-lease cleanup path
// uses it to dispose a handed-over session outside every w.bcast section.
func (s *session) dispose() {
	s.retiredOfferIDs = nil
	s.pendingRemoteIce = nil
	_ = s.pc.Close()
}

// adoptSession binds a stashed in-flight session to this tracker generation
// and returns the session's wait channel.
func (s *sessionTracker) adoptSession(sess *session) <-chan struct{} {
	var waitCh <-chan struct{}
	sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		sess.t = s
		// The successor owns the session lifecycle now; a predecessor fatal
		// error must not exit the successor before it negotiates.
		sess.fatalErr = nil
		waitCh = getWaitCh()
		broadcast()
	})
	return waitCh
}

// activeOfferID returns the digest identifying the negotiation generation
// this session transmits signals for: the pending local offer while offering,
// otherwise the remote offer applied to this session.
func (s *session) activeOfferID() []byte {
	if s.t != nil && s.t.offerer {
		return s.pendingOfferID
	}
	return s.rxOfferID
}

// retransmitOutstandingOffer replays the identical outstanding local offer
// bytes and generation identity instead of minting a new generation, so an
// in-flight answer still correlates. The minted bytes are replayed verbatim:
// Pion augments LocalDescription with gathered ICE candidates, which would
// present the remote with a byte-different offer that answers under a new
// identity. Returns false when no offer is outstanding.
func (s *sessionTracker) retransmitOutstandingOffer(
	sess *session,
	currLocalSeqno uint64,
	xmitSignal func(*WebRtcSignal),
) (bool, error) {
	// Require an outstanding local offer with retained SDP before replaying it.
	if sess.pendingOfferID == nil ||
		sess.pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer {
		return false, nil
	}
	if sess.pendingOfferSDP == "" {
		return false, pkgerrors.New("retransmit outstanding offer: no pending offer sdp")
	}
	if s.w.GetVerbose() {
		s.le.Debug("signal tx: retransmit outstanding offer")
	}
	xmitSdp := &WebRtcSdp{
		TxSeqno: currLocalSeqno,
		SdpType: webrtc.SDPTypeOffer.String(),
		Sdp:     sess.pendingOfferSDP,
	}
	xmitSdp.OfferId = sess.pendingOfferID
	xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: xmitSdp}})
	return true, nil
}

// transmitLocalNegotiation emits one offer or request for each local sequence.
func (s *sessionTracker) transmitLocalNegotiation(
	sess *session,
	le *logrus.Entry,
	currLocalSeqno uint64,
	lastLocalSeqno uint64,
	xmitSignal func(*WebRtcSignal),
) (uint64, bool, error) {
	// Skip negotiation when the session sequence has already been transmitted.
	if currLocalSeqno == lastLocalSeqno {
		return lastLocalSeqno, false, nil
	}

	// Choose an offer or offer request for the current negotiation role.
	var xmit *WebRtcSignal
	if s.offerer {
		if s.w.GetVerbose() {
			le.Debug("signal tx: offer sdp")
		}
		retransmitted, err := s.retransmitOutstandingOffer(sess, currLocalSeqno, xmitSignal)
		if err != nil {
			return lastLocalSeqno, false, err
		}
		if retransmitted {
			return currLocalSeqno, true, nil
		}

		// Create and retain the local offer and its generation digest.
		localDesc, err := sess.pc.CreateOffer(nil)
		if err != nil {
			return lastLocalSeqno, false, pkgerrors.Wrap(err, "create offer")
		}
		if err := sess.pc.SetLocalDescription(localDesc); err != nil {
			return lastLocalSeqno, false, pkgerrors.Wrap(err, "set local description(offer)")
		}
		offerSum := sha256.Sum256([]byte(localDesc.SDP))
		sess.pendingOfferID = offerSum[:]
		sess.pendingOfferSDP = localDesc.SDP
		xmitSdp := NewWebRtcSdp(currLocalSeqno, &localDesc)
		xmitSdp.OfferId = offerSum[:]
		xmit = &WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: xmitSdp}}
	} else {
		// Ask for an offer only while none is applied. The offerer reads a
		// request after its answer landed as a new remote PeerConnection.
		if sess.pc.RemoteDescription() != nil {
			return currLocalSeqno, false, nil
		}
		if s.w.GetVerbose() {
			le.Debug("signal tx: offer request")
		}
		xmit = &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: currLocalSeqno}}
	}

	// Transmit the negotiation signal for the current session sequence.
	xmitSignal(xmit)
	return currLocalSeqno, true, nil
}

// execute executes the sessionTracker.
func (s *sessionTracker) execute(ctx context.Context) (err error) {
	// Publish the tracker execution and arrange retirement of its child routines.
	defer s.le.Warn("session tracker exited")
	phase := "startup"
	defer func() {
		if e := recover(); e != nil {
			err = pkgerrors.Errorf("%s: recovered panic: %v", phase, e)
		}
	}()
	s.le.Info("session tracker starting")
	execution := s.beginExecution()
	var linkDone, xmitDone <-chan struct{}
	defer func() {
		s.completeExecution(execution, linkDone, xmitDone)
	}()

	// Construct the PeerConnection and attach the callbacks. A session left
	// by a retired predecessor with an offer still outstanding is adopted so
	// its pending generation survives regeneration and the remote answer
	// still correlates.
	phase = "construct session"
	sess := s.w.takeAdoptableSession(s.key, execution)
	var waitCh <-chan struct{}
	if sess != nil {
		waitCh = s.adoptSession(sess)
		s.le.Debug("adopted in-flight negotiation session from retired predecessor")
	} else {
		sess, waitCh, err = s.newSession(ctx)
		if err != nil {
			return pkgerrors.Wrap(err, phase)
		}
	}
	defer func() {
		sess.close()
	}()

	// current is the session the child routines report failures to. It
	// changes when the remote peer replaces its PeerConnection.
	var current atomic.Pointer[session]
	current.Store(sess)

	// Construct the QUIC link routine and route failures into session state.
	errCh := make(chan error, 1)
	linkRoutine := routine.NewStateRoutineContainer(
		func(t1, t2 datachannel.ReadWriteCloser) bool { return t1 == t2 },
		routine.WithExitCb(func(err error) {
			if err != nil {
				current.Load().failWithErr(errCh, pkgerrors.Wrap(err, "link routine"))
			}
		}),
	)
	_, _, _ = linkRoutine.SetStateRoutine(func(ctx context.Context, dcRwc datachannel.ReadWriteCloser) error {
		return s.executeLink(ctx, current.Load().pc, dcRwc)
	})

	// Construct the signal transmit routine and route failures into session
	// state. It starts once the signaling session is open.
	xmitRoutine := routine.NewRoutineContainer(
		routine.WithExitCb(func(err error) {
			if err != nil {
				current.Load().failWithErr(errCh, pkgerrors.Wrap(err, "signal transmit routine"))
			}
		}),
	)

	// Set the context for the link routine.
	phase = "bind routines"
	linkRoutine.SetContext(ctx, true)
	xmitRoutine.SetContext(ctx, true)

	// Stop child routines before retiring this execution generation.
	defer func() {
		linkDone, _, _, _ = linkRoutine.SetState(nil)
		xmitDone, _ = xmitRoutine.SetRoutine(nil)
	}()

	// Open the signaling session with the remote peer.
	phase = "open signaling session"
	signal, signalRel, err := signaling.ExSignalPeer(
		ctx,
		s.w.b,
		s.w.conf.GetSignalingId(),
		s.w.peerID,
		s.peerID,
		false,
	)
	if err != nil {
		return pkgerrors.Wrap(err, phase)
	}
	defer signalRel()

	// xmitSignal queues a signal for the remote peer. signalSent is closed
	// once the most recently queued signal is sent.
	var outbox signalOutbox
	_, _ = xmitRoutine.SetRoutine(func(ctx context.Context) error {
		return s.executeOutbox(ctx, &outbox, signal)
	})
	var signalSent <-chan struct{}
	xmitSignal := func(msg *WebRtcSignal) {
		signalSent = outbox.push(msg)
	}

	// Watch the state and act accordingly.
	recheck := make(chan struct{}, 1)
	recheckNext := func() {
		select {
		case recheck <- struct{}{}:
		default:
		}
	}

	// Currently processed local sequence number.
	var lastLocalSeqno, currRemoteSeqno uint64
	var currLinkRwc datachannel.ReadWriteCloser

	// lastAppliedRemoteSdp is the SDP string we last applied via
	// SetRemoteDescription. A byte-identical duplicate is ignored to avoid an
	// unnecessary renegotiation / ICE restart.
	var lastAppliedRemoteSdp string

	// Which ICE candidate index did we send last?
	var lastSentICE int

	// sentIceComplete records whether the end-of-candidates marker has been
	// transmitted for the current negotiation generation. It is reset whenever
	// lastSentICE resets (a new local offer/answer regathers candidates).
	var sentIceComplete bool

	// Remote ICE candidates buffer on the session, not in this frame: a
	// tracker restart hands the buffer to the successor with the session
	// itself, so an in-flight trickle survives the restart. See
	// session.pendingRemoteIce.
	remoteICE := remoteICECandidateApplier{add: sess.pc.AddICECandidate}

	// replaceSession closes the current session and continues on a fresh
	// PeerConnection. The remote peer replaced its own PeerConnection, so the
	// current one can never reconnect: renegotiating it would only wait out
	// the ICE and QUIC idle timeouts while the stale link stays published.
	replaceSession := func() error {
		// Stop the link on the current connection and wait for it to exit.
		waitReturn, _, _, _ := linkRoutine.SetState(nil)
		if waitReturn != nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitReturn:
			}
		}
		currLinkRwc = nil

		// Close the session and open its replacement, keeping remote
		// candidates buffered for a generation not applied yet.
		pending := sess.pendingRemoteIce
		sess.close()
		next, nextWaitCh, err := s.newSession(ctx)
		if err != nil {
			return pkgerrors.Wrap(err, "replace session")
		}
		next.pendingRemoteIce = pending
		sess, waitCh = next, nextWaitCh
		current.Store(sess)
		remoteICE = remoteICECandidateApplier{add: sess.pc.AddICECandidate}

		// Start the negotiation state over for the new connection.
		lastLocalSeqno, lastAppliedRemoteSdp = 0, ""
		lastSentICE, sentIceComplete = 0, false
		return nil
	}

	// React to session notifications and incoming signaling messages.
	for {
		phase = "wait for session change"

		// Wait for something to change or for an incoming signal.
		var currIncomingSignal *incomingSignal

		// Prioritize receiving an incoming signal first.
		select {
		case <-ctx.Done():
			return context.Canceled
		case currIncomingSignal = <-execution.rxSignal:
		default:
		}

		// Then allow also re-checking in case we need to transmit ice candidates.
		if currIncomingSignal == nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case err := <-errCh:
				return err
			case currIncomingSignal = <-execution.rxSignal:
			case <-signalSent:
				signalSent = nil
			case <-waitCh:
			case <-recheck:
			}
		}

		// A channel receive is not acceptance. The terminal-state transition
		// and this acceptance fence are serialized by the session owner.
		if currIncomingSignal != nil {
			sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				sess.acceptIncomingSignalLocked(currIncomingSignal)
			})
		}

		// Process the incoming signal, if any.
		var currRxSdp *WebRtcSdp
		var currRxIce *WebRtcIce
		if currIncomingSignal != nil {
			phase = "process incoming signal"
			switch b := currIncomingSignal.sig.GetBody().(type) {
			case *WebRtcSignal_RequestOffer:
				if !s.offerer {
					return errors.New("remote peer requested offer but we are not the offerer")
				}
				currRemoteSeqno = b.RequestOffer
				// The remote asks for an offer: retransmit the outstanding
				// offer so a restarted answerer re-receives the generation
				// its buffered candidates belong to.
				retransmitted, err := s.retransmitOutstandingOffer(sess, b.RequestOffer, xmitSignal)
				if err != nil {
					return err
				}

				// The answerer asks for an offer only while it holds none, so
				// a request after this session's answer landed comes from a
				// new remote PeerConnection. Offer to it from a fresh session.
				if !retransmitted && sess.pc.RemoteDescription() != nil {
					s.le.Info("remote peer requested a new session: replacing the peer connection")
					if err := replaceSession(); err != nil {
						return err
					}
				}
			case *WebRtcSignal_Sdp:
				// An offer from a different remote PeerConnection starts a
				// new session instead of renegotiating the current one.
				if !s.offerer && replacesRemotePeerConnection(sess.pc.RemoteDescription(), b.Sdp) {
					s.le.Info("remote peer replaced its peer connection: replacing the local one")
					if err := replaceSession(); err != nil {
						return err
					}
				}

				// Process the incoming sdp below.
				currRxSdp = b.Sdp
				currRemoteSeqno = b.Sdp.GetTxSeqno()
			case *WebRtcSignal_Ice:
				currRxIce = b.Ice
			default:
				// Unknown message, ignore it.
				s.le.Warn("recv unknown signal from remote peer")
			}
		}

		// Check the current state.
		var currLocalSeqno uint64
		var currConnState webrtc.PeerConnectionState
		var currFatalErr error
		var currTxICE []*webrtc.ICECandidateInit
		var currLocalIceComplete bool
		var currDcRwc datachannel.ReadWriteCloser
		phase = "snapshot session state"
		sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// check if negotiation is needed
			currConnState = sess.connState
			currLocalSeqno, currFatalErr = sess.localSeqno, sess.fatalErr

			// check ice candidates to tx
			if currLocalSeqno != lastLocalSeqno || lastSentICE > len(sess.localIceCandidates) {
				lastSentICE = 0
				sentIceComplete = false
			}
			currTxICE = sess.localIceCandidates[lastSentICE:]
			currLocalIceComplete = sess.localIceCandidatesComplete

			// check if data channel is open
			if sess.dcOpen {
				currDcRwc = sess.dcRwc
			}

			// get the next wait channel
			waitCh = getWaitCh()
		})
		if currFatalErr != nil {
			return currFatalErr
		}
		if currConnState == webrtc.PeerConnectionStateFailed {
			return errors.New("webrtc connection failed")
		}

		// logger
		le := s.le.WithFields(logrus.Fields{
			"local-seqno":  currLocalSeqno,
			"remote-seqno": currRemoteSeqno,
		})

		// Construct or tear down link as necessary.
		if currDcRwc != currLinkRwc {
			phase = "update link routine"

			// Update the link routine and wait for the old link to exit.
			waitReturn, changed, _, _ := linkRoutine.SetState(currDcRwc)
			if changed && waitReturn != nil {
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-waitReturn:
				}
			}
			currLinkRwc = currDcRwc
		}

		// Handle incoming offer and ICE signals.
		if err := s.ingestRemoteSignal(
			sess,
			currRxSdp,
			currRxIce,
			currLocalSeqno,
			&lastAppliedRemoteSdp,
			&remoteICE,
			&sess.pendingRemoteIce,
			xmitSignal,
			le,
			&phase,
		); err != nil {
			return err
		}

		// If there is a pending outgoing signaling message, wait to send ice candidates or tx an offer.
		if signalSent != nil {
			select {
			case <-signalSent:
				signalSent = nil
			default:
				continue
			}
		}

		// Transmit an offer or a request for one when local seqno changes.
		if currLocalSeqno != lastLocalSeqno {
			phase = "transmit local negotiation"
		}
		nextLocalSeqno, transmitted, err := s.transmitLocalNegotiation(
			sess,
			le,
			currLocalSeqno,
			lastLocalSeqno,
			xmitSignal,
		)
		if err != nil {
			return err
		}
		lastLocalSeqno = nextLocalSeqno
		if transmitted {

			// Restart sending ice candidates & recheck
			lastSentICE = 0
			sentIceComplete = false
			waitCh = nil
			continue
		}

		// Transmit ICE candidates, continue if waitCh is invalidated meanwhile
		// Transmit at most once at a time, we need to make sure to process remote messages in a timely fashion.
		if len(currTxICE) != 0 {
			phase = "transmit local ice"

			// make sure waitCh hasn't proced already
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
				// Wait channel proced, continue immediately
			default:
				// tx ice candidate
				iceCandidate := currTxICE[0]
				if s.w.GetVerbose() {
					le.Debugf("signal tx: ice candidate %v", lastSentICE)
				}
				ice, err := NewWebRtcIce(iceCandidate)
				if err != nil {
					return pkgerrors.Wrap(err, "marshal local ice candidate")
				}
				ice.OfferId = sess.activeOfferID()
				xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: ice}})
				lastSentICE++
			}
		} else if currLocalIceComplete && !sentIceComplete {
			// All gathered candidates are sent and gathering is complete: signal
			// end-of-candidates exactly once. libwebrtc and pion keep the ICE
			// checklist non-final until they receive this marker; without it the
			// selected pair loses consent at the fixed 15s timeout and the link
			// dies (connected -> disconnected -> failed). Verified with a raw
			// two-browser trickle bisection: the identical exchange stays
			// connected only when end-of-candidates is sent. An empty candidate
			// string is the end-of-candidates marker for both pion (native and
			// js) and the browser; sdpMLineIndex 0 keeps the browser's
			// addIceCandidate from rejecting an all-null candidate.
			phase = "transmit end-of-candidates"
			if s.w.GetVerbose() {
				le.Debug("signal tx: end-of-candidates")
			}
			mlineIndex := uint16(0)
			eoc, err := NewWebRtcIce(&webrtc.ICECandidateInit{SDPMLineIndex: &mlineIndex})
			if err != nil {
				return pkgerrors.Wrap(err, "marshal end-of-candidates")
			}
			eoc.OfferId = sess.activeOfferID()
			xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: eoc}})
			sentIceComplete = true
		}

		// If there are still ICE candidates to transmit, recheck next time right away.
		if len(currTxICE) > 1 {
			recheckNext()
		}
	}
}

// pendingRemoteCandidate is a remote ICE candidate buffered before its
// generation could accept it, with the offer id it arrived tagged with.
type pendingRemoteCandidate struct {
	candidate webrtc.ICECandidateInit
	offerID   []byte
}

// splitBufferedCandidates partitions buffered candidates into those tagged
// with offerID, which the active generation can accept now, and the rest,
// which stay buffered for a generation that has not landed yet.
func splitBufferedCandidates(
	pending []pendingRemoteCandidate,
	offerID []byte,
) (matched []webrtc.ICECandidateInit, rest []pendingRemoteCandidate) {
	for _, c := range pending {
		if bytes.Equal(c.offerID, offerID) {
			matched = append(matched, c.candidate)
		} else {
			rest = append(rest, c)
		}
	}
	return matched, rest
}

// remoteICECandidateApplier applies one signaling generation in order.
type remoteICECandidateApplier struct {
	add      func(webrtc.ICECandidateInit) error
	complete bool
}

func (a *remoteICECandidateApplier) apply(candidates []webrtc.ICECandidateInit) error {
	for i := range candidates {
		if a.complete {
			return nil
		}
		if err := a.add(candidates[i]); err != nil {
			return err
		}
		if candidates[i].Candidate == "" {
			a.complete = true
		}
	}
	return nil
}

// isOfferer checks if peer ID A is the offerer or answerer.
func isOfferer(a, b string) bool {
	return strings.Compare(a, b) < 0
}

// transmitAnswer emits the exact answer SDP bytes retained for the session's
// active remote-offer generation.
func (s *sessionTracker) transmitAnswer(
	sess *session,
	answerSDP string,
	currLocalSeqno uint64,
	xmitSignal func(*WebRtcSignal),
) {
	if answerSDP == "" {
		return
	}
	ans := &WebRtcSdp{
		TxSeqno: currLocalSeqno,
		SdpType: webrtc.SDPTypeAnswer.String(),
		Sdp:     answerSDP,
		OfferId: sess.rxOfferID,
	}
	xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: ans}})
}

// ingestRemoteSignal applies one received SDP/candidate signal pair to the
// session.
func (s *sessionTracker) ingestRemoteSignal(
	sess *session,
	currRxSdp *WebRtcSdp,
	currRxIce *WebRtcIce,
	currLocalSeqno uint64,
	lastAppliedRemoteSdp *string,
	remoteICE *remoteICECandidateApplier,
	pendingRemoteIce *[]pendingRemoteCandidate,
	xmitSignal func(*WebRtcSignal),
	le *logrus.Entry,
	phase *string,
) error {
	// Handle incoming offer.
	sdpType := currRxSdp.GetSdpType()
	if sdpType != "" {
		*phase = "handle remote sdp"

		// Enforce offerer always does the offering.
		if s.offerer {
			if sdpType != "answer" {
				return errors.New("expected answer from remote peer but got " + sdpType)
			}
		} else {
			if sdpType != "offer" {
				return errors.New("expected offer from remote peer but got " + sdpType)
			}
		}

		// Generation fence: reject material whose offer_id does not identify
		// its own generation before Pion state is touched.
		if s.offerer {
			if !bytes.Equal(currRxSdp.GetOfferId(), sess.pendingOfferID) {
				le.Debug("dropping stale answer: offer id does not match the pending local offer")
				return nil
			}
		} else {
			offerSum := sha256.Sum256([]byte(currRxSdp.GetSdp()))
			if !bytes.Equal(currRxSdp.GetOfferId(), offerSum[:]) {
				le.Debug("dropping stale sdp: offer id does not match the sdp bytes")
				return nil
			}
			if _, retired := sess.retiredOfferIDs[string(offerSum[:])]; retired {
				le.Debug("dropping retired-generation sdp")
				return nil
			}
		}

		sessDesc := currRxSdp.ToSessionDescription()
		switch {
		case sessDesc == nil:
			// Malformed description; ignore it.
		case currRxSdp.GetSdp() == *lastAppliedRemoteSdp:
			// A byte-identical duplicate of the description we already applied.
			// Ignore it to avoid an unnecessary renegotiation / ICE restart. The
			// offerer re-sends its offer on every request_offer, so the answerer
			// routinely sees the same offer twice. When the retransmitted offer
			// is the generation this session already answered, the offerer may
			// have regenerated and lost the original answer in flight: replay
			// the retained local answer so the outstanding offer still
			// correlates, without touching Pion state a second time.
			if !s.offerer &&
				bytes.Equal(currRxSdp.GetOfferId(), sess.rxOfferID) &&
				sess.pc.RemoteDescription() != nil {
				if s.w.GetVerbose() {
					le.Debug("signal tx: replay retained answer")
				}
				s.transmitAnswer(sess, sess.rxOfferAnswerSDP, currLocalSeqno, xmitSignal)
			}
		case s.offerer && sess.pc.SignalingState() != webrtc.SignalingStateHaveLocalOffer:
			// Drop an answer that arrives with no local offer pending. Applying an
			// answer while signalingState is already "stable" makes pion fail with
			// "set remote answer sdp: called in wrong state: stable", which would
			// tear down an otherwise healthy PeerConnection and cascade into a
			// reconnect storm.
			le.WithField("signaling-state", sess.pc.SignalingState().String()).
				Debug("dropping stale answer: no pending local offer")
		default:
			if err := sess.pc.SetRemoteDescription(*sessDesc); err != nil {
				return pkgerrors.Wrap(err, "set remote description")
			}
			*lastAppliedRemoteSdp = currRxSdp.GetSdp()

			// Record the accepted remote offer digest as the active generation
			// identity. On the offerer the active identity is the pending
			// local offer instead, so the answer digest is not stored here.
			// A previously applied offer digest becomes retired for this
			// session: a replayed copy must never reach Pion again.
			if !s.offerer {
				offerSum := sha256.Sum256([]byte(currRxSdp.GetSdp()))
				if len(sess.rxOfferID) > 0 && !bytes.Equal(sess.rxOfferID, offerSum[:]) {
					if sess.retiredOfferIDs == nil {
						sess.retiredOfferIDs = make(map[string]struct{})
					}
					sess.retiredOfferIDs[string(sess.rxOfferID)] = struct{}{}
				}
				sess.rxOfferID = offerSum[:]
				sess.rxOfferAnswerSDP = ""
			}

			// Flush the buffered candidates tagged with this generation.
			// Candidates buffered under a different offer id stay buffered:
			// the remote may still be trickling them for a generation this
			// session has not seen yet, and replaying them into this
			// generation would poison its candidate set.
			matched, remaining := splitBufferedCandidates(*pendingRemoteIce, sess.activeOfferID())
			*pendingRemoteIce = remaining
			remoteICE.complete = false
			if err := remoteICE.apply(matched); err != nil {
				// Best-effort flush: a rejected buffered candidate is skipped
				// rather than tearing down the freshly applied offer.
				le.WithError(err).Debug("skipping rejected buffered remote ice candidate")
			}

			// Transmit an answer if applicable
			if !s.offerer {
				if s.w.GetVerbose() {
					le.Debug("signal tx: answer sdp")
				}
				answer, err := sess.pc.CreateAnswer(nil)
				if err != nil {
					return pkgerrors.Wrap(err, "create answer")
				}
				if err := sess.pc.SetLocalDescription(answer); err != nil {
					return pkgerrors.Wrap(err, "set local description(answer)")
				}
				sess.rxOfferAnswerSDP = answer.SDP
				s.transmitAnswer(sess, sess.rxOfferAnswerSDP, currLocalSeqno, xmitSignal)
			}
		}
	}

	// Handle incoming ICE.
	if currRxIce.GetCandidate() != "" {
		*phase = "handle remote ice"

		// Generation fence: candidates for the active generation reach Pion;
		// candidates for a retired generation drop; candidates for a
		// generation this session has not seen yet buffer until it lands, so
		// a tracker restart does not discard the in-flight candidate set
		// while the remote keeps trickling under the previous offer id.
		active := sess.rxOfferID
		if s.offerer {
			active = sess.pendingOfferID
		}
		if len(active) != 0 && !bytes.Equal(currRxIce.GetOfferId(), active) {
			le.Debug("dropping stale ice candidate: offer id does not match the active generation")
			return nil
		}

		ice, err := currRxIce.ParseICECandidateInit()
		if err != nil {
			return pkgerrors.Wrap(err, "parse remote ice candidate")
		}

		// Apply the candidate once a remote description exists; otherwise
		// buffer it. pion drops candidates added with no remote description,
		// and the offerer commonly receives the answerer's candidates before
		// the answer SDP lands.
		if ice != nil {
			if sess.pc.RemoteDescription() != nil {
				if err := remoteICE.apply([]webrtc.ICECandidateInit{*ice}); err != nil {
					// A candidate the browser or ICE agent rejects is a
					// best-effort signal, not a negotiation failure: browsers
					// reject candidates for transient reasons (e.g. a passive
					// TCP candidate when the local side never opened a TCP
					// listener). Skipping it keeps the tracker alive; treating
					// it as fatal tears down the negotiation and starves ICE
					// of the remaining candidates.
					le.WithError(err).Debug("skipping rejected remote ice candidate")
				}
			} else {
				*pendingRemoteIce = append(*pendingRemoteIce, pendingRemoteCandidate{
					candidate: *ice,
					offerID:   append([]byte(nil), currRxIce.GetOfferId()...),
				})
			}
		}
	}

	return nil
}
