package simulate

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/stream"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
)

// TestConnectivity tests for basic connectivity between two peers.
// Uses an echo test: dials the Echo controller.
func TestConnectivity(ctx context.Context, px0, px1 *Peer) error {
	tb0 := px0.testbed

	_, esRef, err := tb0.Bus.AddDirective(link.NewEstablishLinkWithPeer(
		px0.GetPeerID(),
		px1.GetPeerID(),
	), nil)
	if err != nil {
		return err
	}
	defer esRef.Release()

	ms1, ms1Rel, err := link.OpenStreamWithPeerEx(
		ctx,
		tb0.Bus,
		stream_echo.DefaultProtocolID,
		px0.GetPeerID(),
		px1.GetPeerID(),
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		return err
	}
	defer ms1Rel()

	// expect px0 stream remote peer to equal px1
	mns1rp := ms1.GetLink().GetRemotePeer().String()
	if px1p := px1.GetPeerID().String(); px1p != mns1rp {
		return errors.Errorf(
			"stream on p0 remote peer id %s != expected %s",
			mns1rp,
			px1p,
		)
	}
	// expect px0 stream local peer to equal px0
	mns1lp := ms1.GetLink().GetLocalPeer().String()
	if px0p := px0.GetPeerID().String(); px0p != mns1lp {
		return errors.Errorf(
			"stream on p0 local peer id %s != expected %s",
			mns1lp,
			px0p,
		)
	}

	data := []byte("testing 1234")
	_, err = ms1.GetStream().Write(data)
	if err != nil {
		return err
	}

	// expect remote to echo back exactly len(data) bytes
	outData := make([]byte, len(data)*2)
	on, oe := ms1.GetStream().Read(outData)
	if oe != nil {
		return oe
	}

	outData = outData[:on]
	if on != len(data) {
		return errors.Errorf(
			"length incorrect received %v != %v data: %v",
			on,
			len(data),
			string(outData),
		)
	}
	return nil
}
