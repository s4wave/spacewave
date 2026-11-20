package frametrack

import (
	"io"

	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// ReadInput reads one display input event from an admitted control stream.
func ReadInput(control io.ReadWriteCloser) (*Input, error) {
	// Decode one bounded packet so a malformed event cannot reach the source.
	input := new(Input)
	if err := stream_packet.NewSession(control, maxControlMessageSize).RecvMsg(input); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return input, nil
}

// WriteInput sends an event after the subscription on a display control stream.
// The caller serializes writes to the control stream.
func WriteInput(control io.ReadWriteCloser, input *Input) error {
	// Keep invalid events and oversized packets off the stream.
	if err := input.Validate(); err != nil {
		return err
	}
	if input.SizeVT() > maxControlMessageSize {
		return errors.New("display input is too large")
	}

	// Use the same packet framing as the subscription.
	return stream_packet.NewSession(control, maxControlMessageSize).SendMsg(input)
}

// Validate checks that the event addresses a point in the picture or a key.
func (i *Input) Validate() error {
	switch event := i.GetEvent().(type) {
	case *Input_Pointer:
		if p := event.Pointer; p != nil && p.X >= 0 && p.X <= 1 && p.Y >= 0 && p.Y <= 1 {
			return nil
		}
		return errors.New("pointer is outside the display")
	case *Input_Key:
		if k := event.Key; k != nil && (k.Code != "" || k.Key != "") {
			return nil
		}
		return errors.New("display input is missing its key")
	default:
		return errors.New("display input is missing its event")
	}
}
