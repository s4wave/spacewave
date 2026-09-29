package spacewave_launcher_gossip

import (
	"bytes"
	"context"
	"crypto/sha256"

	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// maxMessageSize bounds one gossip frame. A signed DistConfig is a few KiB.
const maxMessageSize = 1024 * 1024

// exchange runs the gossip protocol on one stream until the stream fails or
// ctx ends, then closes the stream.
//
// Each side announces its current DistConfig when the stream opens and
// whenever it changes. When the peer announces a different config with a
// higher revision, the exchange requests it once per announced hash and
// pushes the offered message to the local launcher, which verifies it and
// adopts it only if it is newer. Adoption changes the local config, so every
// other exchange announces it to its own peer.
func exchange(ctx context.Context, le *logrus.Entry, sess *stream_packet.Session, l launcher) error {
	// Closing the stream on return ends the receive goroutine.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer sess.Close()

	// Forward received messages to the exchange loop.
	recvCh := make(chan *Message)
	errCh := make(chan error, 1)
	go func() {
		for {
			msg := &Message{}
			if err := sess.RecvMsg(msg); err != nil {
				errCh <- err
				return
			}
			select {
			case recvCh <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	// sentHash is the hash of the last announced config.
	var sentHash []byte
	// remoteRev and remoteHash describe the peer's last announced config.
	var remoteRev uint64
	var remoteHash []byte
	// requestedHash is the remote hash last requested.
	var requestedHash []byte
	// awaitingOffer is true while a request has no offer in reply.
	var awaitingOffer bool
	for {
		info, changed := l.Snapshot()
		localMsg := info.GetDistConfigMsg()
		localRev := info.GetDistConfig().GetRev()
		localHash := hashDistConfigMsg(localMsg)

		// Announce a changed local config.
		if localMsg != "" && !bytes.Equal(localHash, sentHash) {
			announce := &Announce{Rev: localRev, Hash: localHash}
			if err := sess.SendMsg(&Message{Body: &Message_Announce{Announce: announce}}); err != nil {
				return err
			}
			sentHash = localHash
		}

		// Request a newer remote config once per announced hash.
		if info != nil &&
			remoteRev > localRev &&
			!bytes.Equal(remoteHash, localHash) &&
			!bytes.Equal(remoteHash, requestedHash) {
			if err := sess.SendMsg(&Message{Body: &Message_Request{Request: &Request{}}}); err != nil {
				return err
			}
			requestedHash = remoteHash
			awaitingOffer = true
		}

		// Wait for a local change or a peer message.
		var msg *Message
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case <-changed:
			continue
		case msg = <-recvCh:
		}

		// Handle the peer message.
		switch body := msg.GetBody().(type) {
		case *Message_Announce:
			remoteRev = body.Announce.GetRev()
			remoteHash = body.Announce.GetHash()
		case *Message_Request:
			if localMsg == "" {
				continue
			}
			offer := &Offer{DistConfigMsg: localMsg}
			if err := sess.SendMsg(&Message{Body: &Message_Offer{Offer: offer}}); err != nil {
				return err
			}
		case *Message_Offer:
			if !awaitingOffer {
				continue
			}
			awaitingOffer = false
			adopted, err := l.Push(ctx, body.Offer.GetDistConfigMsg())
			if err != nil {
				le.WithError(err).Debug("rejected dist config offered by peer")
				continue
			}
			if adopted {
				le.Info("adopted dist config from peer")
			}
		}
	}
}

// hashDistConfigMsg returns the SHA-256 of a signed DistConfig message, or
// nil if msg is empty.
func hashDistConfigMsg(msg string) []byte {
	if msg == "" {
		return nil
	}
	h := sha256.Sum256([]byte(msg))
	return h[:]
}
