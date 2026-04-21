package web_view_handler

import (
	"context"
	"errors"

	web_view "github.com/s4wave/spacewave/bldr/web/view"
)

// NewHandleWebViewRequest constructs a request from a web view.
func NewHandleWebViewRequest(id, parent, documentID string, permanent bool) *HandleWebViewRequest {
	return &HandleWebViewRequest{
		Id:         id,
		ParentId:   parent,
		DocumentId: documentID,
		Permanent:  permanent,
	}
}

// HandleWebViewViaClient handles the web view via the SRPC client.
func HandleWebViewViaClient(
	ctx context.Context,
	client SRPCHandleWebViewServiceClient,
	webView web_view.WebView,
) error {
	resp, err := client.HandleWebView(
		ctx,
		NewHandleWebViewRequest(
			webView.GetId(),
			webView.GetParentId(),
			webView.GetDocumentId(),
			webView.GetPermanent(),
		),
	)
	if err == nil {
		errStr := resp.GetError()
		if errStr != "" {
			err = errors.New(errStr)
		}
	}
	return err
}
