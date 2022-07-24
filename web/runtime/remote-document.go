package web_runtime

import (
	"context"

	web_document "github.com/aperturerobotics/bldr/web/document"
	web_view "github.com/aperturerobotics/bldr/web/document/view"
	"github.com/aperturerobotics/starpc/srpc"
)

// RemoteWebDocument implements the Document page APIs for the runtime.
type RemoteWebDocument struct {
	// ctx is the root context
	ctx context.Context
	// r is the remote
	r *Remote
	// id is the identifier for the webdocument
	id string
	// permanent indicates the web document cannot be closed
	permanent bool
	// doc is the remote WebDocument instance
	doc *web_document.Remote
	// openStream is the function to open a stream to the WebDocument.
	openStream srpc.OpenStreamFunc
}

// NewRemoteWebDocument constructs a new remote WebDocument handle.
//
// if permanent, this web document is the primary and cannot be closed
func NewRemoteWebDocument(ctx context.Context, r *Remote, id string, permanent bool) (*RemoteWebDocument, error) {
	le := r.le.WithField("web-document", id)
	b := r.bus
	v := &RemoteWebDocument{
		ctx:       ctx,
		r:         r,
		id:        id,
		permanent: permanent,
	}
	var err error
	v.doc, err = web_document.NewRemote(le, b, v, id)
	if err != nil {
		return nil, err
	}
	v.openStream = v.r.GetWebDocumentOpenStream(id)
	return v, nil
}

// GetWebDocumentUuid returns the web document identifier.
func (w *RemoteWebDocument) GetWebDocumentUuid() string {
	return w.id
}

// HandleWebView handles an incoming WebView on a new Goroutine.
func (w *RemoteWebDocument) HandleWebView(view web_view.WebView) {
	// TODO
	id := view.GetWebViewUuid()
	le := w.r.le.WithField("web-document", w.id)
	le.WithField("web-view", id).Info("TODO WebRuntime: handle incoming web view")
}

// OpenRpcStream opens an RPC stream to the WebDocument.
func (w *RemoteWebDocument) OpenRpcStream(
	ctx context.Context,
	msgHandler srpc.PacketHandler,
	closeHandler srpc.CloseHandler,
) (srpc.Writer, error) {
	return w.openStream(ctx, msgHandler, closeHandler)
}

// _ is a type assertion
var _ web_document.WebDocumentHandler = ((*RemoteWebDocument)(nil))
