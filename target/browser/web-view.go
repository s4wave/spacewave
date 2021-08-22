//go:build js
// +build js

package browser

import (
	"context"

	"github.com/aperturerobotics/bldr/runtime"
	broadcast_channel "github.com/aperturerobotics/bldr/runtime/ipc/broadcast-channel"
	ipc_webview "github.com/aperturerobotics/bldr/runtime/ipc/webview"
	"github.com/gogo/protobuf/proto"
)

// WebView implements the browser page APIs for the runtime.
type WebView struct {
	// ctx is the root context
	ctx context.Context
	// id is the identifier for the webview
	id string
	// root indicates if this is the root webview (cannot be closed)
	root bool
	// ch is the broadcast channel to the frontend runtime
	ch *broadcast_channel.BroadcastChannel
}

// NewWebView constructs a new WebView handle.
//
// if isRoot, this web view is the primary and cannot be closed
func NewWebView(ctx context.Context, id string, isRoot bool) *WebView {
	txID := Prefix + "/webview/" + id
	rxID := Prefix + "/runtime"
	ch := broadcast_channel.NewBroadcastChannel(ctx, txID, rxID)
	return &WebView{ctx: ctx, id: id, root: isRoot, ch: ch}
}

// Close shuts down the WebView and closes the window/tab if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *WebView) Close() error {
	if w.root {
		return runtime.ErrWebViewPermanent
	}

	// TODO
	return nil
}

// writeQueryViewStatus writes the query view status command.
func (w *WebView) writeQueryViewStatus() error {
	msg := ipc_webview.NewQueryViewStatus()
	return w.writeMessage(msg)
}

// writeMessage writes a proto message to the stream.
func (w *WebView) writeMessage(msg *ipc_webview.RuntimeToWebView) error {
	dat, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.ch.Write([]byte(dat))
	return err
}

// closeWindow is the internal implementation of Close.
func (w *WebView) closeWindow() {
	if !w.root {
		// TODO
	}
}

// _ is a type assertion
var _ runtime.WebView = ((*WebView)(nil))
