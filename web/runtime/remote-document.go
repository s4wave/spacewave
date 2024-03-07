package web_runtime

import (
	"context"

	web_document "github.com/aperturerobotics/bldr/web/document"
	web_document_controller "github.com/aperturerobotics/bldr/web/document/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/retry"
	"github.com/sirupsen/logrus"
)

// RemoteWebDocument implements the Document page APIs for the runtime.
type RemoteWebDocument struct {
	// ctx is the context for the RemoteWebDocument
	ctx context.Context
	// ctxCancel cancels ctx
	ctxCancel context.CancelFunc
	// r is the remote
	r *Remote
	// id is the identifier for the webdocument
	id string
	// permanent indicates the web document cannot be closed
	permanent bool
	// openStream is the open stream func.
	openStream srpc.OpenStreamFunc
	// remote is the web_document remote.
	remote *web_document.Remote
	// ctrl is the WebDocument controller.
	ctrl *web_document_controller.Controller
}

// NewRemoteWebDocument constructs a new remote WebDocument handle.
//
// if permanent, this web document is the primary and cannot be closed
func NewRemoteWebDocument(ctx context.Context, r *Remote, id string, permanent bool) (*RemoteWebDocument, error) {
	openStream := r.GetWebDocumentOpenStream(id)
	v := &RemoteWebDocument{
		r:          r,
		id:         id,
		permanent:  permanent,
		openStream: openStream,
	}
	var err error
	v.ctx, v.ctxCancel = context.WithCancel(ctx)
	v.ctrl, err = web_document_controller.NewController(
		r.le,
		r.bus,
		id,
		web_document.RemoteVersion,
		func(le *logrus.Entry, b bus.Bus, handler web_document.WebDocumentHandler, id string) (web_document.WebDocument, error) {
			var err error
			v.remote, err = web_document.NewRemote(v.ctx, le, b, handler, id, openStream)
			if err != nil {
				return nil, err
			}
			return v.remote, nil
		},
	)
	if err != nil {
		return nil, err
	}
	go v.Execute()
	return v, nil
}

// Execute is the goroutine to execute the controller.
func (w *RemoteWebDocument) Execute() {
	ctx := w.ctx
	le := w.r.le.WithField("document-id", w.id)
	bo := (&backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval: 120,
			MaxInterval:     1000,
			Multiplier:      2,
		},
	}).Construct()
	_ = retry.Retry(ctx, le, func(ctx context.Context, success func()) error {
		err := w.r.bus.ExecuteController(ctx, w.ctrl)
		if err == nil {
			w.r.bus.RemoveController(w.ctrl)
		}
		if err != nil && (ctx.Err() == nil || err != context.Canceled) {
			w.r.le.
				WithError(err).
				WithField("document-id", w.id).
				Warn("document controller exited with error")
		}
		return err
	}, bo)
	/*
		_, _ = w.r.cstate.Apply(context.Background(), func(ctx context.Context, v *cstate.CStateWriter[*Remote]) (dirty bool, err error) {
			idx, val := w.r.lookupRemoteWebDocument(w.id)
			dirty = val == w
			if dirty {
				_ = w.r.removeRemoteWebDocumentAtIdx(idx)
			}
			return dirty, nil
		})
		w.Close()
	*/
}

// GetWebDocumentUuid returns the web document identifier.
func (w *RemoteWebDocument) GetWebDocumentUuid() string {
	return w.id
}

// OpenRpcStream opens an RPC stream to the WebDocument.
func (w *RemoteWebDocument) OpenRpcStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	return w.openStream(ctx, msgHandler, closeHandler)
}

// Close closes the RemoteWebDocument.
func (w *RemoteWebDocument) Close() {
	w.ctxCancel()
	_ = w.ctrl.Close()
}
