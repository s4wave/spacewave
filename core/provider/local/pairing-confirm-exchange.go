package provider_local

import (
	"context"
	"io"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// ConfirmProtocolID is the protocol ID for pairing confirmation exchange.
const ConfirmProtocolID = protocol.ID("alpha/pairing-confirm")

// confirmationTimeout is the maximum time to wait for remote confirmation.
const confirmationTimeout = 120 * time.Second

// runPairingConfirmExchange runs the mutual SAS confirmation exchange over
// a solicit stream on the session transport's child bus. Both the generator
// and joiner call this: the generator starts it immediately (solicit waits
// for the remote peer to connect), the joiner starts it after the bifrost
// link establishes.
//
// The flow:
// 1. Emit SolicitProtocol directive to match with the remote peer
// 2. On stream match: extract remote peer ID, compute SAS emoji
// 3. Set VERIFYING_EMOJI status with emoji data
// 4. Wait for local user confirmation via confirmCh
// 5. Send local confirmation to remote
// 6. Wait for remote's confirmation
// 7. Update pairing status based on both results
func (a *ProviderAccount) runPairingConfirmExchange(ctx context.Context) {
	st := a.GetSessionTransport()
	if st == nil {
		return
	}
	childBus := st.GetChildBus()
	if childBus == nil {
		return
	}

	localPeerID := st.GetPeerID()
	le := a.le.WithField("phase", "confirm-exchange")

	// Use a solicit protocol to establish a bilateral stream.
	dir := link_solicit.NewSolicitProtocol(
		ConfirmProtocolID,
		[]byte("pairing-confirm"),
		"",
		0,
	)

	streamCh := make(chan link_solicit.SolicitMountedStream, 1)
	_, solicitRef, err := childBus.AddDirective(
		dir,
		directive.NewTypedCallbackHandler(
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				select {
				case streamCh <- v.GetValue():
				default:
				}
			},
			nil, nil, nil,
		),
	)
	if err != nil {
		le.WithError(err).Warn("failed to add confirm solicit directive")
		return
	}
	defer solicitRef.Release()

	// Wait for a bilateral stream match or context cancel.
	var sms link_solicit.SolicitMountedStream
	select {
	case <-ctx.Done():
		return
	case sms = <-streamCh:
	}

	ms, taken, err := sms.AcceptMountedStream()
	if err != nil || taken {
		return
	}

	strm := ms.GetStream()
	defer strm.Close()

	// Extract remote peer from the matched stream.
	remotePeerID := ms.GetPeerID()
	if len(remotePeerID) == 0 {
		le.Warn("matched stream has no remote peer ID")
		return
	}
	le = le.WithField("remote-peer", remotePeerID.String()[:8])
	le.Debug("pairing confirm stream accepted")

	a.runConfirmExchangeOnStream(ctx, strm, remotePeerID, localPeerID, le)
}

// runDirectConfirmExchange opens (or accepts) a stream on the direct bifrost
// link and runs the mutual SAS confirmation exchange over it.
func (a *ProviderAccount) runDirectConfirmExchange(ctx context.Context, lnk link.Link, localPeerID peer.ID, isOfferer bool) {
	le := a.le.WithField("phase", "direct-confirm-exchange")

	// Offerer (QUIC server) accepts streams; answerer (QUIC client) opens.
	var strm stream.Stream
	var err error
	if isOfferer {
		strm, _, err = lnk.AcceptStream()
	} else {
		strm, err = lnk.OpenStream(stream.OpenOpts{})
	}
	if err != nil {
		le.WithError(err).Warn("failed to open/accept confirm stream")
		a.SetPairingFailed("failed to establish confirmation channel")
		return
	}
	defer strm.Close()

	remotePeerID := lnk.GetRemotePeer()
	le = le.WithField("remote-peer", remotePeerID.String()[:8])
	le.Debug("direct pairing confirm stream opened")

	a.runConfirmExchangeOnStream(ctx, strm, remotePeerID, localPeerID, le)
}

// runConfirmExchangeOnStream runs the core SAS emoji computation and bilateral
// confirmation exchange over an established stream. Used by both the
// SolicitProtocol path (cloud relay) and the direct link path (no-cloud).
func (a *ProviderAccount) runConfirmExchangeOnStream(
	ctx context.Context,
	strm io.ReadWriteCloser,
	remotePeerID peer.ID,
	localPeerID peer.ID,
	le *logrus.Entry,
) {
	// Read session key from pairing state.
	var sessionKey crypto.PrivKey
	a.pairingBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.pairing != nil {
			sessionKey = a.pairing.sessionKey
		}
	})
	if sessionKey == nil || len(localPeerID) == 0 {
		le.Warn("missing keys for SAS emoji derivation")
		return
	}

	// Compute SAS emoji from ECDH shared secret.
	remotePub, err := remotePeerID.ExtractPublicKey()
	if err != nil {
		le.WithError(err).Warn("failed to extract remote public key for SAS")
		return
	}
	emoji, err := DeriveSASEmoji(sessionKey, remotePub, localPeerID, remotePeerID)
	if err != nil {
		le.WithError(err).Warn("failed to derive SAS emoji")
		return
	}

	// Store results and transition to VERIFYING_EMOJI.
	a.pairingBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.pairing == nil {
			return
		}
		a.pairing.remotePeerID = remotePeerID
		a.pairing.emoji = emoji
		a.pairing.confirmCh = make(chan bool, 1)
		a.pairing.status = PairingStatusVerifyingEmoji
		bcast()
	})

	sess := stream_packet.NewSession(strm, 1024)

	// Read confirmCh (just created above).
	var confirmCh <-chan bool
	a.pairingBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.pairing != nil {
			confirmCh = a.pairing.confirmCh
		}
	})
	if confirmCh == nil {
		return
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, confirmationTimeout)
	defer timeoutCancel()

	// Wait for local user decision.
	var localConfirmed bool
	select {
	case <-timeoutCtx.Done():
		if ctx.Err() == nil {
			a.setPairingError(PairingStatusConfirmationTimeout, "confirmation timed out")
		}
		return
	case localConfirmed = <-confirmCh:
	}

	// Send local confirmation to remote.
	msg := &PairingConfirmMessage{
		Confirmed: localConfirmed,
		Rejected:  !localConfirmed,
	}
	if err := sess.SendMsg(msg); err != nil {
		le.WithError(err).Warn("failed to send confirm message")
		return
	}

	if !localConfirmed {
		a.setPairingError(PairingStatusPairingRejected, "pairing rejected locally")
		return
	}

	// Local confirmed. Update status to waiting for remote.
	a.setPairingStatus(PairingStatusWaitingForRemote)

	// Read remote confirmation.
	var remoteMsg PairingConfirmMessage
	if err := sess.RecvMsg(&remoteMsg); err != nil {
		if timeoutCtx.Err() != nil && ctx.Err() == nil {
			a.setPairingError(PairingStatusConfirmationTimeout, "remote confirmation timed out")
		}
		return
	}

	if remoteMsg.GetRejected() || !remoteMsg.GetConfirmed() {
		a.setPairingError(PairingStatusPairingRejected, "remote device rejected the pairing")
		return
	}

	// Both confirmed.
	a.setPairingStatus(PairingStatusBothConfirmed)
	le.Debug("both sides confirmed pairing")
}
