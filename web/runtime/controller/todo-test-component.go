package web_runtime_controller

import (
	"context"

	demo "github.com/aperturerobotics/bldr/toys/test-component"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	view "github.com/aperturerobotics/bldr/web/runtime/view"
	"github.com/sirupsen/logrus"
)

func getTestComponentJS() string {
	return demo.TestComponentJS
}

func loadTestComponent(ctx context.Context, le *logrus.Entry, wv web_runtime.WebView) {
	le.Infof("DEMO: loading test component in web view: %s", wv.GetWebViewUuid())
	_, err := wv.SetRenderMode(ctx, &view.SetRenderModeRequest{
		RenderMode: view.RenderMode_RenderMode_REACT_COMPONENT,
		Wait:       true,
		// /b/test.js
		ScriptPath: "/b/test.js",
	})
	if err != nil {
		le.WithError(err).Error("unable to set render mode")
	} else {
		le.Infof("DEMO: done setting test component in view: %s", wv.GetWebViewUuid())
	}
}
