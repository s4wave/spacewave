package webrtc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"runtime/debug"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
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

// dataChannelID is the data channel label that carries QUIC over WebRTC.
var dataChannelID = "bifrost-quic"

// handshakeTimeout bounds the QUIC handshake over a ready data channel.
const handshakeTimeout = 30 * time.Second

// errLinkClosedLocally reports that the local peer closed a link while its
// QUIC connection was still up.
var errLinkClosedLocally = errors.New("link closed locally")

// sessionTracker wraps an ongoing connection with a peer.
//
// The tracker runs one PeerConnection session at a time. It learns that the
// remote peer is online from the signaling session: the remote peer is
// present while it holds its end of that session open, and each time it
// joins it starts a new presence epoch.
type sessionTracker struct {
	// w is the transport.
	w *WebRTC
	// le is the logger.
	le *logrus.Entry
	// key is the string encoding of the peer id.
	key string
	// peerID is the parsed remote peer id.
	peerID peer.ID
	// peerPub is the remote peer public key.
	peerPub crypto.PubKey
	// offerer indicates the local peer offers and the remote peer answers.
	offerer bool
	// rxSignal carries the signals received from the remote peer.
	rxSignal chan *WebRtcSignal

	// w.bcast guards the fields below.

	// signals are the attached signaling sessions with the remote peer, in
	// attach order.
	signals []*attachedSignal
	// dialers counts the DialPeer calls waiting for a link.
	dialers int
	// link is the current link, if any.
	link *Link
	// linkRef holds a tracker reference while a published link is live, so the
	// tracker outlives the DialPeer call that started it.
	linkRef *keyed.KeyedRef[string, *sessionTracker]
}

// attachedSignal is a signaling session with the remote peer and the presence
// epoch it last reported.
type attachedSignal struct {
	// id is the directive value id of the session.
	id uint32
	// session is the signaling session.
	session signaling.SignalPeerSession
	// cancel stops the presence watch.
	cancel context.CancelFunc
	// epoch is the last presence epoch the session reported. The tracker's
	// w.bcast guards it.
	epoch uint64
}

// presence identifies one presence epoch of the remote peer on one signaling
// session. The zero value is never present.
type presence struct {
	signal signaling.SignalPeerSession
	epoch  uint64
}

// newSessionTracker constructs a new sessionTracker.
func (w *WebRTC) newSessionTracker(peerIDStr string) (keyed.Routine, *sessionTracker) {
	// addSessionTrackerRef validated the peer id before adding the key.
	peerID, peerPub, _ := peer.ParsePeerIDWithPubKey(peerIDStr)
	localPeerIDStr := w.peerID.String()
	offerer := isOfferer(localPeerIDStr, peerIDStr)
	le := w.le.WithField("remote-peer-id", peerIDStr)

	// Construct the tracker with its remote identity and negotiation role.
	sess := &sessionTracker{
		w:        w,
		le:       le,
		key:      peerIDStr,
		peerID:   peerID,
		peerPub:  peerPub,
		offerer:  offerer,
		rxSignal: make(chan *WebRtcSignal),
	}

	return sess.execute, sess
}

// snapshot returns the remote peer's presence, whether a local caller is
// dialing, and a channel closed when either may have changed. The remote peer
// is present on the first attached signaling session that reports it.
func (s *sessionTracker) snapshot() (curr presence, dialing bool, waitCh <-chan struct{}) {
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		for _, sig := range s.signals {
			if sig.epoch != 0 {
				curr = presence{signal: sig.session, epoch: sig.epoch}
				break
			}
		}
		dialing = s.dialers != 0
		waitCh = getWaitCh()
	})
	return curr, dialing, waitCh
}

// addSignal attaches a signaling session and watches its presence until ctx
// ends or the session is removed.
func (s *sessionTracker) addSignal(ctx context.Context, id uint32, session signaling.SignalPeerSession) {
	// Record the session in attach order.
	s.le.Debug("signaling session attached")
	ctx, cancel := context.WithCancel(ctx)
	sig := &attachedSignal{id: id, session: session, cancel: cancel}
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.signals = append(s.signals, sig)
	})

	// Publish each presence change the session reports.
	go func() {
		for {
			epoch, changed := session.GetPresence()
			s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				if sig.epoch != epoch {
					sig.epoch = epoch
					broadcast()
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-changed:
			}
		}
	}()
}

// removeSignal detaches the signaling session with the directive value id.
func (s *sessionTracker) removeSignal(id uint32) {
	s.le.Debug("signaling session detached")
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.signals = slices.DeleteFunc(s.signals, func(sig *attachedSignal) bool {
			if sig.id != id {
				return false
			}
			sig.cancel()
			return true
		})
		broadcast()
	})
}

// opensSession reports whether sig asks the local peer for a new session: an
// offer to the answerer, or an offer request to the offerer.
func (s *sessionTracker) opensSession(sig *WebRtcSignal) bool {
	if s.offerer {
		_, ok := sig.GetBody().(*WebRtcSignal_RequestOffer)
		return ok
	}
	return sig.GetSdp().GetSdpType() == webrtc.SDPTypeOffer.String()
}

// execute executes the sessionTracker.
//
// A session starts while the remote peer is present, either because it offers
// or asks for an offer, or because a local caller is dialing and no earlier
// session spent the current presence epoch. A session the remote peer ends
// spends its epoch, so the tracker waits for the remote peer to join again
// instead of redialing a peer that does not want the link. A session the local
// peer ends, or whose PeerConnection fails, leaves the epoch for the next dial.
// A session that could not be constructed leaves the epoch for a dial after a
// pause.
func (s *sessionTracker) execute(ctx context.Context) error {
	// Log the tracker lifetime.
	s.le.Debug("session tracker starting")
	defer s.le.Debug("session tracker exited")

	// Follow the signaling sessions with the remote peer and the presence
	// each one reports. Several signaling clients may serve the peer at once,
	// as when a reconnecting client overlaps the one it replaces.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	_, signalRef, err := s.w.b.AddDirective(
		ctx,
		signaling.NewSignalPeer(s.w.conf.GetSignalingId(), s.w.peerID, s.peerID),
		directive.NewTypedCallbackHandler(
			func(v directive.TypedAttachedValue[signaling.SignalPeerSession]) {
				s.addSignal(watchCtx, v.GetValueID(), v.GetValue())
			},
			func(v directive.TypedAttachedValue[signaling.SignalPeerSession]) {
				s.removeSignal(v.GetValueID())
			},
			nil,
			nil,
		),
	)
	if err != nil {
		return pkgerrors.Wrap(err, "watch signaling session")
	}
	defer signalRef.Release()

	// spent is the presence epoch the remote peer ended a session in. first is
	// the remote signal that opens the next session, if any.
	var spent presence
	var first *WebRtcSignal

	// retry is the pause after the last construction failure, zero after a
	// session that was constructed. retryAt is when the next session may start.
	var retry time.Duration
	var retryAt time.Time
	for {
		// Start a session when the remote peer asks for one or a local caller
		// dials a presence epoch not yet spent, once any retry pause has ended.
		curr, dialing, waitCh := s.snapshot()
		pause := time.Until(retryAt)
		if pause <= 0 && curr.epoch != 0 && (first != nil || (dialing && curr != spent)) {
			le := s.le.WithField("presence-epoch", curr.epoch)
			le.Info("webrtc session starting")
			rearm, next, err := s.runSession(ctx, curr, first)
			if ctx.Err() != nil {
				return context.Canceled
			}
			le = le.WithField("rearm", rearm)
			if err != nil {
				le = le.WithError(err)
			}
			le.Info("webrtc session ended")
			first, spent = next, curr
			if rearm {
				spent = presence{}
			}

			// A session that could not be constructed leaves the epoch to a
			// retry after a growing pause, so one failed credential fetch
			// cannot strand the link until the remote peer rejoins.
			var construct *constructError
			switch {
			case !errors.As(err, &construct):
				retry = 0
			case retry == 0:
				retry = constructRetryMin
			default:
				retry = min(2*retry, constructRetryMax)
			}
			if retry != 0 {
				retryAt = time.Now().Add(retry)
			}
			continue
		}

		// Wait for a dial, a presence change, a remote request, or the end of
		// the retry pause.
		var retryTimer *time.Timer
		var retryCh <-chan time.Time
		if pause > 0 {
			retryTimer = time.NewTimer(pause)
			retryCh = retryTimer.C
		}
		select {
		case <-ctx.Done():
			if retryTimer != nil {
				retryTimer.Stop()
			}
			return context.Canceled
		case <-waitCh:
		case <-retryCh:
		case sig := <-s.rxSignal:
			if s.opensSession(sig) {
				s.le.WithField("presence-epoch", curr.epoch).Debug("remote peer requested a session")
				first = sig
			} else if s.w.GetVerbose() {
				s.le.Debug("dropping remote signal received outside a session")
			}
		}
		if retryTimer != nil {
			retryTimer.Stop()
		}
	}
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

		// Send it, then dequeue it and report it sent. A routine replaced by a
		// new signaling session stops without failing the session.
		if err := s.sendSignal(ctx, sess, next.sig); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
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

// executeLink executes the quic link over a data channel of sess. It returns
// errLinkClosedLocally when the local peer closed the link, and io.EOF when the
// remote peer or the path closed it.
func (s *sessionTracker) executeLink(
	ctx context.Context,
	sess *session,
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
	// than MaxIdleTimeout. Without keepalive PINGs quic-go reaps it with an idle
	// timeout ("no recent network activity") and the link has to re-establish,
	// stalling traffic that depended on it. Keepalive (MaxIdleTimeout/2) holds an
	// otherwise-healthy idle link open. Unlike the websocket transport, the
	// datachannel has no lower-layer keepalive of its own.
	linkOpts.DisableKeepAlive = false
	linkOpts.DisablePathMtuDiscovery = true
	linkOpts.MaxIdleTimeoutDur = "60s"

	// Negotiate QUIC with the offerer listening and answerer dialing.
	// The QUIC roles evenly split responsibilities between the peers.
	//
	// Assuming peer A is the offerer and B the answerer:
	// 1. A -> B: offer SDP
	// 2. B -> A: answer SDP
	// 3. A -> B: ICE candidate
	// 4. B -> A: ICE candidate
	// 5. B -> A: Dial quic (mTLS)
	// 6. A -> B: Answer dial quic
	//
	// Bound the handshake. The listener waits in Accept until a handshake
	// completes, and a dial whose packet writes block on the data channel
	// never reaches its handshake idle timeout, so either side can otherwise
	// wait forever and hold the tracker that every later link to the peer
	// reuses. Closing the data channel at the deadline unblocks both.
	hsCtx, hsCancel := context.WithTimeout(ctx, handshakeTimeout)
	defer hsCancel()
	stopClose := context.AfterFunc(hsCtx, func() { _ = dcRwc.Close() })

	// Listen as the offerer or dial as the answerer.
	var conn *quic.Conn
	var err error
	s.le.WithField("quic-role", role).Info("webrtc quic phase: handshake starting")
	if s.offerer {
		conn, err = transport_quic.ListenSession(
			hsCtx,
			s.le,
			linkOpts,
			pc,
			s.w.identity,
			s.peerID,
		)
	} else {
		conn, _, err = transport_quic.DialSession(
			hsCtx,
			s.le,
			linkOpts,
			pc,
			s.w.identity,
			remoteAddr,
			s.peerID,
		)
	}
	if !stopClose() && err == nil {
		_ = conn.CloseWithError(0, "handshake ended")
		err = context.Cause(hsCtx)
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

	// publishedRef is the tracker reference this publication established or
	// reused. The link closure releases it only while the tracker still owns it.
	var publishedRef *keyed.KeyedRef[string, *sessionTracker]
	closed := func() {
		// The link closes before its QUIC connection, so a live connection
		// means the local peer closed the link.
		if wasClosed.Swap(true) {
			return
		}
		cause := io.EOF
		if conn.Context().Err() == nil {
			cause = errLinkClosedLocally
		}

		// Retire the published link and its tracker reference.
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

		// Report the cause before closing the data channel: the close ends
		// the link routine, which must find the cause waiting.
		errCh <- cause
		_ = dcRwc.Close()
	}

	// Construct the link and report any construction failure.
	quicLink, err := transport_quic.NewLink(
		ctx,
		s.le,
		&transport_quic.Opts{},
		s.w.GetUUID(),
		s.w.peerID,
		localAddr,
		conn,
		closed,
	)
	if err != nil {
		return pkgerrors.Wrap(err, "construct quic link")
	}
	nextLink = &Link{Link: quicLink, pc: sess.pc}
	s.le.WithField("quic-role", role).Info("webrtc quic phase: link constructed")

	// Publish the link and hold the tracker while it is live, then notify the
	// handler.
	sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		sess.linked = true
		broadcast()
	})
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.link = nextLink
		if s.linkRef == nil {
			ref, _, err := s.w.addSessionTrackerRef(s.key)
			if err != nil {
				s.le.WithError(err).Warn("webrtc quic phase: link tracker reference not acquired")
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

	// Wait for cancellation or link closure. A cause reported before the
	// cancellation wins: the data channel closes right after the link does.
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		select {
		case err := <-errCh:
			return err
		default:
			return context.Canceled
		}
	}
}

// session contains the state for a single ongoing PeerConnection.
type session struct {
	// t is the session tracker.
	t *sessionTracker
	// pc is the peer connection.
	pc *webrtc.PeerConnection

	// bcast guards the fields below.
	bcast broadcast.Broadcast

	// NOTE: these fields are managed by pion-webrtc.

	// fatalErr is the first fatal session error.
	fatalErr error
	// connState is the current connection state.
	connState webrtc.PeerConnectionState

	// localSeqno is the local session sequence number.
	// The number is incremented when negotiation is needed.
	// if offerer: transmit sdp offer when changed
	// if !offerer: transmit request_offer=localSeqno when changed.
	localSeqno uint64

	// localIceCandidates contains the current list of local ice candidates.
	localIceCandidates []*webrtc.ICECandidateInit
	// localIceCandidatesComplete indicates the ice candidate list is complete.
	localIceCandidatesComplete bool

	// dc is the data channel.
	dc *webrtc.DataChannel
	// dcOpen indicates the data channel is open.
	dcOpen bool
	// dcRwc is the detached data channel, nil unless dcOpen.
	dcRwc datachannel.ReadWriteCloser

	// linked indicates the session published its link.
	linked bool

	// NOTE: these fields are managed by runSession.

	// rxOfferID is the SHA-256 digest of the exact offer SDP bytes of the
	// remote offer applied to this session, when set. The digest identifies
	// the active negotiation generation.
	rxOfferID []byte

	// rxOfferAnswerSDP holds the exact local answer SDP bytes transmitted for
	// rxOfferID. Pion augments LocalDescription with gathered ICE candidates,
	// so duplicate-offer replay uses these bytes to preserve the generation.
	rxOfferAnswerSDP string

	// pendingOfferID is the SHA-256 digest of the local offer SDP bytes most
	// recently transmitted by this session, when the local peer is the offerer.
	// Answers whose offer_id does not match pendingOfferID are dropped before
	// Pion.
	pendingOfferID []byte

	// pendingOfferSDP holds the exact local offer SDP bytes whose digest is
	// pendingOfferID. Pion augments LocalDescription with gathered ICE
	// candidates after the initial transmission, so a retransmitted offer
	// replays these bytes to keep its generation identity.
	pendingOfferSDP string

	// retiredOfferIDs holds the digests of remote offers this session already
	// applied and replaced with a newer generation. A replayed copy of a
	// retired offer is dropped before Pion so a stale description can never
	// re-enter negotiation.
	retiredOfferIDs map[string]struct{}

	// lastAppliedRemoteSdp is the SDP string last applied with
	// SetRemoteDescription. A byte-identical duplicate is ignored to avoid an
	// unnecessary renegotiation and ICE restart.
	lastAppliedRemoteSdp string

	// remoteICE applies remote ICE candidates of the active generation.
	remoteICE remoteICECandidateApplier

	// pendingRemoteIce holds remote ICE candidates received before their
	// generation could accept them, tagged with the offer id they arrived
	// under.
	pendingRemoteIce []pendingRemoteCandidate
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
	sess := &session{
		t:         s,
		pc:        pc,
		remoteICE: remoteICECandidateApplier{add: pc.AddICECandidate},
	}

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
	// The ICE candidate callback begins being called once
	// SetRemoteDescription is called.
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
		// Both peers use the same channel label and set Negotiated: true.
		// Negotiated mode avoids redundant info via the OnDataChannel callback.
		Negotiated:     &negotiated,
		Protocol:       &protocol,
		ID:             &channelID,
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
}

// onNegotiationNeeded advances the local sequence so the session negotiates.
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

// failWithErr records the first fatal session error.
func (s *session) failWithErr(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if s.fatalErr == nil {
			s.fatalErr = err
			broadcast()
		}
	})
}

// onIceCandidate records a gathered local ICE candidate, or the end of
// gathering when c is nil.
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

// onConnectionStateChange records the PeerConnection state.
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

	// NewWebRTC sets DetachDataChannels in the WebRTC settings engine.
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

// close closes the peer connection.
func (s *session) close() {
	_ = s.pc.Close()
}

// activeOfferID returns the digest identifying the negotiation generation
// this session transmits signals for: the pending local offer while offering,
// otherwise the remote offer applied to this session.
func (s *session) activeOfferID() []byte {
	if s.t.offerer {
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
	s.le.WithField("offer-id", shortOfferID(sess.pendingOfferID)).
		Info("signal tx: retransmit outstanding offer")
	xmitSdp := &WebRtcSdp{
		TxSeqno: currLocalSeqno,
		SdpType: webrtc.SDPTypeOffer.String(),
		Sdp:     sess.pendingOfferSDP,
		OfferId: sess.pendingOfferID,
	}
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
		// Replay an offer still waiting for its answer instead of minting a
		// new generation.
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
		le.WithField("offer-id", shortOfferID(sess.pendingOfferID)).Info("signal tx: offer")
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

// constructRetryMin and constructRetryMax bound the pause before the next
// session after one could not be constructed.
const (
	constructRetryMin = time.Second
	constructRetryMax = 30 * time.Second
)

// constructError reports a session that ended before its PeerConnection
// existed, as when the ICE credential fetch fails. The remote peer never saw
// the session, so the session spends no presence epoch.
type constructError struct {
	err error
}

// Error returns the construction failure.
func (e *constructError) Error() string { return e.err.Error() }

// Unwrap returns the construction failure.
func (e *constructError) Unwrap() error { return e.err }

// runSession runs one PeerConnection session with the remote peer at pres,
// starting from the remote signal first when set. rearm reports that a later
// dial may start another session in the same presence epoch: the local peer
// closed the link, the PeerConnection failed, or the PeerConnection could not
// be constructed. next is a remote signal that opens the following session,
// when the remote peer replaced its PeerConnection.
func (s *sessionTracker) runSession(
	ctx context.Context,
	pres presence,
	first *WebRtcSignal,
) (rearm bool, next *WebRtcSignal, err error) {
	// Report a panic as a session error naming the phase it happened in.
	phase := "construct session"
	defer func() {
		if e := recover(); e != nil {
			err = pkgerrors.Errorf("%s: recovered panic: %v", phase, e)
		}
	}()

	// Construct the PeerConnection and attach the callbacks.
	sess, waitCh, err := s.newSession(ctx)
	if err != nil {
		return true, nil, &constructError{err: pkgerrors.Wrap(err, phase)}
	}
	defer sess.close()

	// Construct the QUIC link routine and route failures into session state.
	linkRoutine := routine.NewStateRoutineContainer(
		func(t1, t2 datachannel.ReadWriteCloser) bool { return t1 == t2 },
		routine.WithExitCb(func(err error) {
			// The routine is also cancelled when its data channel closes
			// while the session lives. That is the remote peer closing the
			// link, which spends the presence epoch.
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				err = io.EOF
			}
			sess.failWithErr(pkgerrors.Wrap(err, "link routine"))
		}),
	)
	_, _, _ = linkRoutine.SetStateRoutine(func(ctx context.Context, dcRwc datachannel.ReadWriteCloser) error {
		return s.executeLink(ctx, sess, dcRwc)
	})

	// Construct the signal transmit routine. It sends the outbox over the
	// current signaling session.
	var outbox signalOutbox
	xmitRoutine := routine.NewStateRoutineContainer(
		func(t1, t2 signaling.SignalPeerSession) bool { return t1 == t2 },
		routine.WithExitCb(func(err error) {
			sess.failWithErr(pkgerrors.Wrap(err, "signal transmit routine"))
		}),
	)
	_, _, _ = xmitRoutine.SetStateRoutine(func(ctx context.Context, signal signaling.SignalPeerSession) error {
		return s.executeOutbox(ctx, &outbox, signal)
	})

	// Run both routines until the session ends, then wait for them to exit
	// before the PeerConnection closes.
	linkRoutine.SetContext(ctx, true)
	xmitRoutine.SetContext(ctx, true)
	defer func() {
		linkDone, _, _, _ := linkRoutine.SetState(nil)
		xmitDone, _, _ := xmitRoutine.SetStateRoutine(nil)
		if linkDone != nil {
			<-linkDone
		}
		if xmitDone != nil {
			<-xmitDone
		}
	}()

	// xmitSignal queues a signal for the remote peer. signalSent is closed
	// once the most recently queued signal is sent.
	var signalSent <-chan struct{}
	xmitSignal := func(msg *WebRtcSignal) {
		signalSent = outbox.push(msg)
	}

	// recheck wakes the loop to transmit the next local ICE candidate.
	recheck := make(chan struct{}, 1)
	recheckNext := func() {
		select {
		case recheck <- struct{}{}:
		default:
		}
	}

	// Local sequence number processed by the session tracker.
	var lastLocalSeqno, currRemoteSeqno uint64
	var currLinkRwc datachannel.ReadWriteCloser
	var xmitSession signaling.SignalPeerSession

	// lastSentICE is the index of the last ICE candidate sent.
	var lastSentICE int

	// sentIceComplete records whether the end-of-candidates marker has been
	// transmitted for the current negotiation generation. It is reset whenever
	// lastSentICE resets (a new local offer/answer regathers candidates).
	var sentIceComplete bool

	// React to session notifications and incoming signaling messages.
	rxSignal := first
	for {
		phase = "wait for session change"

		// Send over the current signaling session. Before its link is up, the
		// session belongs to the presence epoch it started in.
		curr, _, trackerWaitCh := s.snapshot()
		if curr.signal != xmitSession {
			xmitSession = curr.signal
			_, _, _, _ = xmitRoutine.SetState(xmitSession)
		}
		var linked bool
		sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			linked = sess.linked
		})
		if !linked && curr != pres {
			return false, nil, errors.New("remote peer presence changed before the link came up")
		}

		// Prioritize receiving an incoming signal first.
		if rxSignal == nil {
			select {
			case <-ctx.Done():
				return false, nil, context.Canceled
			case rxSignal = <-s.rxSignal:
			default:
			}
		}

		// Then allow also re-checking in case the tracker needs to transmit ice
		// candidates.
		if rxSignal == nil {
			select {
			case <-ctx.Done():
				return false, nil, context.Canceled
			case rxSignal = <-s.rxSignal:
			case <-signalSent:
				signalSent = nil
			case <-waitCh:
			case <-trackerWaitCh:
			case <-recheck:
			}
		}

		// Process the incoming signal, if any.
		var currRxSdp *WebRtcSdp
		var currRxIce *WebRtcIce
		if rxSignal != nil {
			phase = "process incoming signal"
			sig := rxSignal
			rxSignal = nil
			switch b := sig.GetBody().(type) {
			case *WebRtcSignal_RequestOffer:
				if !s.offerer {
					return false, nil, errors.New("remote peer requested offer but we are not the offerer")
				}
				currRemoteSeqno = b.RequestOffer

				// The remote asks for an offer: retransmit the outstanding
				// offer in case the remote never received it.
				retransmitted, err := s.retransmitOutstandingOffer(sess, b.RequestOffer, xmitSignal)
				if err != nil {
					return false, nil, err
				}

				// The answerer asks for an offer only while it holds none, so
				// a request after this session's answer landed comes from a
				// new remote PeerConnection. Offer to it from a new session.
				if !retransmitted && sess.pc.RemoteDescription() != nil {
					s.le.Info("remote peer requested a new session: replacing the peer connection")
					return false, sig, nil
				}
			case *WebRtcSignal_Sdp:
				// An offer from a different remote PeerConnection starts a
				// new session instead of renegotiating the current one.
				if !s.offerer && replacesRemotePeerConnection(sess.pc.RemoteDescription(), b.Sdp) {
					s.le.Info("remote peer replaced its peer connection: replacing the local one")
					return false, sig, nil
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

		// A failed PeerConnection lost its path, which a new session may
		// regain. A link the local peer closed leaves the epoch for the next
		// dial. Any other failure, such as the remote peer closing the link,
		// spends the epoch.
		if currConnState == webrtc.PeerConnectionStateFailed {
			return true, nil, errors.New("webrtc connection failed")
		}
		if currFatalErr != nil {
			return errors.Is(currFatalErr, errLinkClosedLocally), nil, currFatalErr
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
					return false, nil, context.Canceled
				case <-waitReturn:
				}
			}
			currLinkRwc = currDcRwc
		}

		// Handle incoming offer and ICE signals.
		if err := s.ingestRemoteSignal(sess, currRxSdp, currRxIce, currLocalSeqno, xmitSignal, le, &phase); err != nil {
			return false, nil, err
		}

		// If there is a pending outgoing signaling message, wait to send ice
		// candidates or tx an offer.
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
			return false, nil, err
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
		// Transmit at most once at a time so the tracker processes remote
		// messages in a timely fashion.
		if len(currTxICE) != 0 {
			phase = "transmit local ice"

			// make sure waitCh hasn't proced already
			select {
			case <-ctx.Done():
				return false, nil, context.Canceled
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
					return false, nil, pkgerrors.Wrap(err, "marshal local ice candidate")
				}
				ice.OfferId = sess.activeOfferID()
				xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: ice}})
				lastSentICE++
			}
		} else if currLocalIceComplete && !sentIceComplete {
			// All gathered candidates are sent and gathering is complete: signal
			// end-of-candidates exactly once. libwebrtc and pion keep the ICE
			// checklist non-final until they receive this marker. Without the marker the
			// selected pair loses consent at the fixed 15s timeout and the link
			// dies (connected -> disconnected -> failed). A raw two-browser trickle
			// bisection shows that the identical exchange stays
			// connected only when end-of-candidates is sent. An empty candidate
			// string is the end-of-candidates marker for both pion (native and
			// js) and the browser. sdpMLineIndex 0 keeps the browser's
			// addIceCandidate from rejecting an all-null candidate.
			phase = "transmit end-of-candidates"
			if s.w.GetVerbose() {
				le.Debug("signal tx: end-of-candidates")
			}
			mlineIndex := uint16(0)
			eoc, err := NewWebRtcIce(&webrtc.ICECandidateInit{SDPMLineIndex: &mlineIndex})
			if err != nil {
				return false, nil, pkgerrors.Wrap(err, "marshal end-of-candidates")
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

// shortOfferID returns the leading hex digits of an offer digest, enough to
// match the offers and answers of one negotiation in the logs.
func shortOfferID(offerID []byte) string {
	return hex.EncodeToString(offerID[:min(len(offerID), 4)])
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
			// Malformed description. Ignore it.
		case currRxSdp.GetSdp() == sess.lastAppliedRemoteSdp:
			// A byte-identical duplicate of the description the session already
			// applied.
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
			sess.lastAppliedRemoteSdp = currRxSdp.GetSdp()

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
			matched, remaining := splitBufferedCandidates(sess.pendingRemoteIce, sess.activeOfferID())
			sess.pendingRemoteIce = remaining
			sess.remoteICE.complete = false
			if err := sess.remoteICE.apply(matched); err != nil {
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

		// Generation fence: candidates for the active generation reach Pion.
		// Candidates for another generation drop. Candidates that arrive
		// before any generation is active buffer until one lands.
		active := sess.activeOfferID()
		if len(active) != 0 && !bytes.Equal(currRxIce.GetOfferId(), active) {
			le.Debug("dropping stale ice candidate: offer id does not match the active generation")
			return nil
		}

		ice, err := currRxIce.ParseICECandidateInit()
		if err != nil {
			return pkgerrors.Wrap(err, "parse remote ice candidate")
		}

		// Apply the candidate once a remote description exists. Otherwise
		// buffer it. pion drops candidates added with no remote description,
		// and the offerer commonly receives the answerer's candidates before
		// the answer SDP lands.
		if ice != nil {
			if sess.pc.RemoteDescription() != nil {
				if err := sess.remoteICE.apply([]webrtc.ICECandidateInit{*ice}); err != nil {
					// A candidate the browser or ICE agent rejects is a
					// best-effort signal, not a negotiation failure: browsers
					// reject candidates for transient reasons (e.g. a passive
					// TCP candidate when the local side never opened a TCP
					// listener). Skipping it keeps the tracker alive. Treating
					// it as fatal tears down the negotiation and starves ICE
					// of the remaining candidates.
					le.WithError(err).Debug("skipping rejected remote ice candidate")
				}
			} else {
				sess.pendingRemoteIce = append(sess.pendingRemoteIce, pendingRemoteCandidate{
					candidate: *ice,
					offerID:   append([]byte(nil), currRxIce.GetOfferId()...),
				})
			}
		}
	}

	return nil
}
