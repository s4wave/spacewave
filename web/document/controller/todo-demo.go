package web_document_controller

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/sirupsen/logrus"
)

func loadTestComponent(ctx context.Context, le *logrus.Entry, wv web_view.WebView) {
	le.Infof("DEMO: loading test component in web view: %s", wv.GetId())
	_, err := wv.SetRenderMode(ctx, &web_view.SetRenderModeRequest{
		RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
		Wait:       true,
		// /b/test.js
		// TODO
		// ScriptPath: "/b/test.js",
	})
	if err != nil {
		if err != context.Canceled {
			le.WithError(err).Error("unable to set render mode")
		}
	} else {
		le.Infof("DEMO: done setting test component in view: %s", wv.GetId())
	}
}
