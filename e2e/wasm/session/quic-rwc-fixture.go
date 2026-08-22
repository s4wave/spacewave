//go:build !tinygo

package e2e_wasm_session

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// RunQuicRwcFixture establishes two direct browser WebRTC peers in this worker,
// runs both QUIC roles over their detached data channels, and echoes one stream
// payload. It intentionally bypasses the signaling and link-establishment
// controllers so failures remain localized to the bridged packet path.
func (c *Controller) RunQuicRwcFixture(
	ctx context.Context,
	req *RunQuicRwcFixtureRequest,
) (*RunQuicRwcFixtureResponse, error) {
	payload := slices.Clone(req.GetPayload())
	if len(payload) == 0 {
		return nil, errors.New("fixture payload is empty")
	}
	le := c.GetLogger()
	le.Info("quic fixture phase: peer setup starting")

	privA, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate peer A key")
	}
	privB, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate peer B key")
	}
	peerA, err := peer.IDFromPrivateKey(privA)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer A ID")
	}
	peerB, err := peer.IDFromPrivateKey(privB)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer B ID")
	}
	identityA, err := p2ptls.NewIdentity(privA)
	if err != nil {
		return nil, errors.Wrap(err, "construct peer A identity")
	}
	identityB, err := p2ptls.NewIdentity(privB)
	if err != nil {
		return nil, errors.Wrap(err, "construct peer B identity")
	}

	tptA, err := s4wave_session.NewManualSignalTransport(
		le.WithField("fixture-peer", "A"), identityA, peerA, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "construct peer A transport")
	}
	defer tptA.Close()
	tptB, err := s4wave_session.NewManualSignalTransport(
		le.WithField("fixture-peer", "B"), identityB, peerB, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "construct peer B transport")
	}
	defer tptB.Close()

	offer, err := tptA.CreateOffer(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create fixture offer")
	}
	answer, err := tptB.AcceptOffer(ctx, offer)
	if err != nil {
		return nil, errors.Wrap(err, "accept fixture offer")
	}
	if err := tptA.AcceptAnswer(answer); err != nil {
		return nil, errors.Wrap(err, "accept fixture answer")
	}
	le.Info("quic fixture phase: data channel negotiation complete")

	type linkResult struct {
		link *transport_quic.Link
		err  error
	}
	linkCh := make(chan linkResult, 2)
	go func() {
		link, err := tptA.WaitLink(ctx, ctx, peerB)
		linkCh <- linkResult{link: link, err: err}
	}()
	go func() {
		link, err := tptB.WaitLink(ctx, ctx, peerA)
		linkCh <- linkResult{link: link, err: err}
	}()

	var linkA, linkB *transport_quic.Link
	for range 2 {
		result := <-linkCh
		if result.err != nil {
			return nil, errors.Wrap(result.err, "establish fixture QUIC link")
		}
		if result.link.GetLocalPeer() == peerA {
			linkA = result.link
		} else {
			linkB = result.link
		}
	}
	if linkA == nil || linkB == nil {
		return nil, errors.New("fixture QUIC links did not identify both peers")
	}
	defer linkA.Close()
	defer linkB.Close()
	le.Info("quic fixture phase: handshake complete")

	echoCh := make(chan error, 1)
	go func() {
		strm, _, err := linkA.AcceptStream()
		if err != nil {
			echoCh <- errors.Wrap(err, "accept fixture stream")
			return
		}
		defer strm.Close()
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(strm, buf); err != nil {
			echoCh <- errors.Wrap(err, "read fixture payload")
			return
		}
		_, err = io.Copy(strm, bytes.NewReader(buf))
		echoCh <- errors.Wrap(err, "echo fixture payload")
	}()

	strm, err := linkB.OpenStream(stream.OpenOpts{})
	if err != nil {
		return nil, errors.Wrap(err, "open fixture stream")
	}
	defer strm.Close()
	if _, err := io.Copy(strm, bytes.NewReader(payload)); err != nil {
		return nil, errors.Wrap(err, "write fixture payload")
	}
	echoed := make([]byte, len(payload))
	if _, err := io.ReadFull(strm, echoed); err != nil {
		return nil, errors.Wrap(err, "read echoed fixture payload")
	}
	if err := <-echoCh; err != nil {
		return nil, err
	}
	if !bytes.Equal(echoed, payload) {
		return nil, errors.New("fixture echo payload mismatch")
	}
	le.Info("quic fixture phase: stream echo complete")
	return &RunQuicRwcFixtureResponse{EchoedPayload: echoed}, nil
}
