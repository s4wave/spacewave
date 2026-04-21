package webrtc

import (
	"context"
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

	// errCh is pushed if a fatal error failed the session or signaling
	errCh chan error
	// rxSignal receives an incoming signaling message
	rxSignal chan *WebRtcSignal
	// xmitRoutine is the routine that manages transmitting a signaling message
	xmitRoutine *routine.StateRoutineContainer[*outgoingSignal]
	// linkRoutine is the routine that manages the Quic link when the session dcOpen.
	linkRoutine *routine.StateRoutineContainer[datachannel.ReadWriteCloser]
	// link contains the current link, if any
	// w.bcast is broadcasted when this changes
	link *transport_quic.Link
}

// newSessionTracker constructs a new sessionTracker.
func (w *WebRTC) newSessionTracker(peerIDStr string) (keyed.Routine, *sessionTracker) {
	// note: we confirmed that parsePeerID is valid before adding the key
	peerID, peerPub, _ := peer.ParsePeerIDWithPubKey(peerIDStr)
	localPeerIDStr := w.peerID.String()
	offerer := isOfferer(localPeerIDStr, peerIDStr)
	le := w.le.WithField("remote-peer-id", peerIDStr)

	sess := &sessionTracker{
		w:       w,
		le:      le,
		key:     peerIDStr,
		peerID:  peerID,
		peerPub: peerPub,
		offerer: offerer,
	}

	sess.errCh = make(chan error, 1)

	sess.linkRoutine = routine.NewStateRoutineContainer(
		func(t1, t2 datachannel.ReadWriteCloser) bool { return t1 == t2 },
		routine.WithExitCb(func(err error) {
			if err != nil {
				sess.failWithErr(pkgerrors.Wrap(err, "link routine"))
			}
		}),
	)
	_, _, _ = sess.linkRoutine.SetStateRoutine(sess.executeLink)

	sess.rxSignal = make(chan *WebRtcSignal)
	sess.xmitRoutine = routine.NewStateRoutineContainer[*outgoingSignal](
		nil,
		routine.WithExitCb(func(err error) {
			if err != nil {
				sess.failWithErr(pkgerrors.Wrap(err, "signal transmit routine"))
			}
		}),
	)
	_, _, _ = sess.xmitRoutine.SetStateRoutine(sess.executeXmitSignal)

	return sess.execute, sess
}

// failWithErr pushes to errCh
func (s *sessionTracker) failWithErr(err error) {
	if err != nil && err != context.Canceled {
		select {
		case s.errCh <- err:
		default:
		}
	}
}

// outgoingSignal contains a signal to transmit
type outgoingSignal struct {
	sess   signaling.SignalPeerSession
	sig    *WebRtcSignal
	sent   atomic.Bool
	sentCh chan struct{}
}

// markSent marks the signal as sent, returns if it was already sent
func (s *outgoingSignal) markSent() bool {
	wasSent := s.sent.Swap(true)
	if !wasSent {
		close(s.sentCh)
	}
	return wasSent
}

// executeXmitSignal executes transmitting a signal to the remote peer.
func (s *sessionTracker) executeXmitSignal(ctx context.Context, sig *outgoingSignal) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = pkgerrors.Errorf("xmit signal panic: %v\n%s", e, debug.Stack())
		}
	}()
	msgEnc, err := EncodeWebRtcSignal(sig.sig, s.peerPub)
	if err != nil {
		return pkgerrors.Wrap(err, "encode web rtc signal")
	}
	defer scrub.Scrub(msgEnc)
	if err := sig.sess.Send(ctx, msgEnc); err != nil {
		return pkgerrors.Wrap(err, "send signaling message")
	}
	sig.markSent()
	return nil
}

// executeLink executes the quic link with a data channel.
func (s *sessionTracker) executeLink(ctx context.Context, dcRwc datachannel.ReadWriteCloser) error {
	// Packet conn: maximum packet size should be larger than the MTU quic uses.
	// Use one that aligns with one memory page (4096 bytes)
	// Buffer 8 packets at a time.
	localAddr := peer.NewNetAddr(s.w.peerID)
	remoteAddr := peer.NewNetAddr(s.peerID)
	pc := rwc.NewRwcPacketConn(dcRwc, localAddr, remoteAddr)

	// Configure quic with settings specific to webRTC
	linkOpts := s.w.conf.GetQuic().CloneVT()
	if linkOpts == nil {
		linkOpts = &transport_quic.Opts{}
	}
	linkOpts.DisableDatagrams = true
	linkOpts.DisableKeepAlive = true
	linkOpts.DisablePathMtuDiscovery = true
	linkOpts.MaxIdleTimeoutDur = "60s"

	// Invert it so that the answerer dials the Quic link.
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
		return pkgerrors.Wrap(err, "construct quic session")
	}

	errCh := make(chan error, 1)
	var nextLink *transport_quic.Link
	var wasClosed atomic.Bool
	closed := func() {
		if wasClosed.Swap(true) {
			return
		}
		s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if s.link == nextLink {
				s.link = nil
				broadcast()
			}
		})
		go s.w.handler.HandleLinkLost(nextLink)
		_ = dcRwc.Close()
		errCh <- io.EOF
	}

	nextLink, err = transport_quic.NewLink(
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

	// Link established.
	s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.link = nextLink
		broadcast()
	})
	s.w.handler.HandleLinkEstablished(nextLink)

	// Cleanup link on exit
	defer func() {
		if !wasClosed.Load() {
			go nextLink.Close()
		}
	}()

	// Wait for the context to be canceled or routine canceled
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
func (s *sessionTracker) newSession() (*session, <-chan struct{}, error) {
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
	pc, err := s.w.webrtcApi.NewPeerConnection(*s.w.webrtcConf)
	if err != nil {
		return nil, nil, pkgerrors.Wrap(err, "create peer connection")
	}

	// Create the data channel in advance.
	negotiated := true
	protocol := dataChannelID
	ordered := false // Allow unordered data since Quic can handle it.
	var channelID uint16 = 1
	dc, err := pc.CreateDataChannel(dataChannelID, &webrtc.DataChannelInit{
		// We use the same channel label on both sides and set Negotiated: true.
		// This avoids sending redundant info via the OnDataChannel callback.
		Negotiated: &negotiated,
		Protocol:   &protocol,
		ID:         &channelID,
		Ordered:    &ordered,
	})
	if err != nil {
		_ = pc.Close()
		return nil, nil, pkgerrors.Wrap(err, "create data channel")
	}

	sess := &session{t: s, pc: pc, dc: dc}

	var waitCh <-chan struct{}
	sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

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
	if err := setCallback("register peer connection onnegotiationneeded", func() {
		pc.OnNegotiationNeeded(sess.onNegotiationNeeded)
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

func (s *session) onIceCandidate(c *webrtc.ICECandidate) {
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
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

// close closes the session.
func (s *session) close() {
	_ = s.pc.Close()
}

// execute executes the sessionTracker.
func (s *sessionTracker) execute(ctx context.Context) (err error) {
	defer s.le.Warn("session tracker exited")
	phase := "startup"
	defer func() {
		if e := recover(); e != nil {
			err = pkgerrors.Errorf("%s: recovered panic: %v", phase, e)
		}
	}()
	s.le.Info("session tracker starting")

	// Construct the PeerConnection and attach the callbacks.
	phase = "construct session"
	sess, waitCh, err := s.newSession()
	if err != nil {
		return pkgerrors.Wrap(err, phase)
	}
	defer sess.close()

	// Set the context for the link routine.
	phase = "bind routines"
	s.linkRoutine.SetContext(ctx, true)
	s.xmitRoutine.SetContext(ctx, true)

	// When exiting, clear any references to this tracker from incoming signaling.
	// The incoming signaling will trigger re-adding a reference if any new messages arrive.
	defer func() {
		peerIDStr := s.peerID.String()
		s.linkRoutine.SetState(nil)
		s.xmitRoutine.SetState(nil)
		s.w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if ref := s.w.incomingSessions[peerIDStr]; ref != nil {
				ref.Release()
				broadcast()
				delete(s.w.incomingSessions, peerIDStr)
			}
		})
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

	// xmitSignal transmits a signal to the remote peer.
	// returns a channel that is closed when the signal is sent successfully
	// clobbers any existing message that was pending send
	var signalSent <-chan struct{}
	xmitSignal := func(msg *WebRtcSignal) {
		sentCh := make(chan struct{})
		_, _, _, _ = s.xmitRoutine.SetState(&outgoingSignal{
			sess:   signal,
			sig:    msg,
			sentCh: sentCh,
		})
		signalSent = sentCh
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
	_ = currRemoteSeqno // TODO: remote restarted SDP?
	// Which ICE candidate index did we send last?
	var lastSentICE int

	for {
		phase = "wait for session change"
		// Wait for something to change or for an incoming signal.
		var currRxSignal *WebRtcSignal

		// Prioritize receiving an incoming signal first.
		select {
		case <-ctx.Done():
			return context.Canceled
		case currRxSignal = <-s.rxSignal:
		default:
		}

		// Then allow also re-checking in case we need to transmit ice candidates.
		if currRxSignal == nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case err := <-s.errCh:
				return err
			case currRxSignal = <-s.rxSignal:
			case <-signalSent:
				signalSent = nil
			case <-waitCh:
			case <-recheck:
			}
		}

		// Process the incoming signal, if any.
		var currRxSdp *WebRtcSdp
		var currRxIce *WebRtcIce
		if currRxSignal != nil {
			phase = "process incoming signal"
			switch b := currRxSignal.GetBody().(type) {
			case *WebRtcSignal_RequestOffer:
				if !s.offerer {
					return errors.New("remote peer requested offer but we are not the offerer")
				}
				currRemoteSeqno = b.RequestOffer
			case *WebRtcSignal_Sdp:
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
		var currDcRwc datachannel.ReadWriteCloser
		phase = "snapshot session state"
		sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// check if negotiation is needed
			currConnState = sess.connState
			currLocalSeqno, currFatalErr = sess.localSeqno, sess.fatalErr

			// check ice candidates to tx
			if currLocalSeqno != lastLocalSeqno || lastSentICE > len(sess.localIceCandidates) {
				lastSentICE = 0
			}
			currTxICE = sess.localIceCandidates[lastSentICE:]

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
			waitReturn, changed, _, _ := s.linkRoutine.SetState(currDcRwc)
			if changed && waitReturn != nil {
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-waitReturn:
				}
			}
			currLinkRwc = currDcRwc
		}

		// Handle incoming offer.
		sdpType := currRxSdp.GetSdpType()
		if sdpType != "" {
			phase = "handle remote sdp"
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

			sessDesc := currRxSdp.ToSessionDescription()
			if sessDesc != nil {
				// Set the remote description
				if err := sess.pc.SetRemoteDescription(*sessDesc); err != nil {
					return pkgerrors.Wrap(err, "set remote description")
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
					xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Sdp{NewWebRtcSdp(
						currLocalSeqno,
						&answer,
					)}})
				}
			}
		}

		// Handle incoming ICE.
		if currRxIce.GetCandidate() != "" {
			phase = "handle remote ice"
			ice, err := currRxIce.ParseICECandidateInit()
			if err != nil {
				return pkgerrors.Wrap(err, "parse remote ice candidate")
			}
			// If there is no remote description, drop the ICE candidate.
			if ice != nil && sess.pc.RemoteDescription() != nil {
				if err := sess.pc.AddICECandidate(*ice); err != nil {
					return pkgerrors.Wrap(err, "add remote ice candidate")
				}
			}
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
			var xmit *WebRtcSignal

			if s.offerer {
				if s.w.GetVerbose() {
					le.Debug("signal tx: offer sdp")
				}
				localDesc, err := sess.pc.CreateOffer(nil)
				if err != nil {
					return pkgerrors.Wrap(err, "create offer")
				}
				if err := sess.pc.SetLocalDescription(localDesc); err != nil {
					return pkgerrors.Wrap(err, "set local description(offer)")
				}
				xmit = &WebRtcSignal{
					Body: &WebRtcSignal_Sdp{Sdp: NewWebRtcSdp(
						currLocalSeqno,
						&localDesc,
					)},
				}
			} else {
				if s.w.GetVerbose() {
					le.Debug("signal tx: offer request")
				}
				xmit = &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: currLocalSeqno}}
			}

			// Encrypt and transmit the message.
			xmitSignal(xmit)

			// Mark as sent
			lastLocalSeqno = currLocalSeqno

			// Restart sending ice candidates & recheck
			lastSentICE = 0
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
				xmitSignal(&WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: ice}})
				lastSentICE++
			}
		}

		// If there are still ICE candidates to transmit, recheck next time right away.
		if len(currTxICE) > 1 {
			recheckNext()
		}
	}
}

// isOfferer checks if peer ID A is the offerer or answerer.
func isOfferer(a, b string) bool {
	return strings.Compare(a, b) < 0
}
