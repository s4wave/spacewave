//go:build !js

package wasm

import (
	"context"
	"log"
	"slices"

	"github.com/pkg/errors"
	e2e_wasm_session "github.com/s4wave/spacewave/e2e/wasm/session"
)

// RelayCrossConnect forwards signaling messages between two SignalRelay
// streams bidirectionally. It runs two goroutines that forward A.Recv to
// B.Send and B.Recv to A.Send until the context is canceled or an error
// occurs.
//
// The returned channel receives the first error from either goroutine.
// The caller should cancel the context to stop the cross-connect.
func RelayCrossConnect(
	ctx context.Context,
	strmA, strmB e2e_wasm_session.SRPCSignalRelayService_SignalRelayClient,
) <-chan error {
	errCh := make(chan error, 2)

	forward := func(src, dst e2e_wasm_session.SRPCSignalRelayService_SignalRelayClient) {
		for {
			msg, err := src.Recv()
			if err != nil {
				errCh <- errors.Wrap(err, "relay recv")
				return
			}
			data := slices.Clone(msg.GetData())
			log.Printf("e2e relay cross-connect forward bytes=%d", len(data))
			if err := dst.Send(&e2e_wasm_session.SignalRelayMessage{
				Body: &e2e_wasm_session.SignalRelayMessage_Data{
					Data: data,
				},
			}); err != nil {
				errCh <- errors.Wrap(err, "relay send")
				return
			}
		}
	}

	go forward(strmA, strmB)
	go forward(strmB, strmA)
	return errCh
}
