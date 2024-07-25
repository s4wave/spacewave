package app2

import (
	"context"
	"time"

	common "github.com/aperturerobotics/bldr/prototypes/webworker-rpcstream/common"
)

// PrototypeHost implements the prototype server.
type PrototypeHost struct {
	a *App
}

func NewPrototypeHost(a *App) *PrototypeHost {
	return &PrototypeHost{a: a}
}

// Prototype implements the prototype request.
func (h *PrototypeHost) Prototype(req *common.PrototypeRequest, strm common.SRPCPrototypeService_PrototypeStream) error {
	le := h.a.GetLogger()
	le.Infof("got Prototype rpc from app1: %v", req.String())
	defer le.Info("exiting Prototype rpc from app1")

	ctx := strm.Context()
	ticker := time.NewTicker(time.Millisecond * 500)
	var seqno int32
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-ticker.C:
		}

		seqno++
		resp := &common.PrototypeResponse{Body: req.GetBody(), SequenceNumber: seqno}
		le.Infof("sending Prototype rpc message to app1: %v", resp.String())
		if err := strm.Send(resp); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ common.SRPCPrototypeServiceServer = ((*PrototypeHost)(nil))
