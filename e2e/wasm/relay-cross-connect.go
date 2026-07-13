//go:build !js

package wasm

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	e2e_wasm_session "github.com/s4wave/spacewave/e2e/wasm/session"
)

// RelayProgress records one direction's cumulative signaling relay traffic.
type RelayProgress struct {
	Direction string
	Messages  uint64
	Bytes     uint64
}

// RelayCrossConnect forwards signaling messages between two SignalRelay
// streams until the context is canceled or one direction fails.
//
// The progress channel reports cumulative message and byte counts for each
// direction. The error channel receives the first forwarding error.
func RelayCrossConnect(
	ctx context.Context,
	strmA, strmB e2e_wasm_session.SRPCSignalRelayService_SignalRelayClient,
) (<-chan RelayProgress, <-chan error) {
	progressCh := make(chan RelayProgress, 16)
	errCh := make(chan error, 2)

	forward := func(direction string, src, dst e2e_wasm_session.SRPCSignalRelayService_SignalRelayClient) {
		var messages, bytes uint64
		for {
			msg, err := src.Recv()
			if err != nil {
				errCh <- errors.Wrap(err, "relay recv")
				return
			}
			data := slices.Clone(msg.GetData())
			if err := dst.Send(&e2e_wasm_session.SignalRelayMessage{
				Body: &e2e_wasm_session.SignalRelayMessage_Data{
					Data: data,
				},
			}); err != nil {
				errCh <- errors.Wrap(err, "relay send")
				return
			}
			messages++
			bytes += uint64(len(data))
			select {
			case progressCh <- RelayProgress{
				Direction: direction,
				Messages:  messages,
				Bytes:     bytes,
			}:
			default:
			}
		}
	}

	go forward("A->B", strmA, strmB)
	go forward("B->A", strmB, strmA)
	return progressCh, errCh
}
