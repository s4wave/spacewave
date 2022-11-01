package web_view_handler

import (
	"context"
	"errors"

	web_view "github.com/aperturerobotics/bldr/web/view"
)

// NewHandleWebViewRequest constructs a request from a web view.
func NewHandleWebViewRequest(id, parent string, permanent bool) *HandleWebViewRequest {
	return &HandleWebViewRequest{
		Id:        id,
		ParentId:  parent,
		Permanent: permanent,
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
		NewHandleWebViewRequest(webView.GetId(), webView.GetParentId(), webView.GetPermanent()),
	)
	if err == nil {
		errStr := resp.GetError()
		if errStr != "" {
			err = errors.New(errStr)
		}
	}
	return err
}
