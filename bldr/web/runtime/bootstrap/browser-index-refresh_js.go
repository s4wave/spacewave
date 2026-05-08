//go:build js

package web_runtime_bootstrap

import (
	"syscall/js"

	"github.com/sirupsen/logrus"
)

const browserIndexPath = "/b/__index.html"

func triggerBrowserIndexCacheRefresh(le *logrus.Entry) {
	global := js.Global()
	navigator := global.Get("navigator")
	if !navigator.IsUndefined() && !navigator.IsNull() {
		serviceWorker := navigator.Get("serviceWorker")
		if !serviceWorker.IsUndefined() && !serviceWorker.IsNull() {
			controller := serviceWorker.Get("controller")
			if !controller.IsUndefined() && !controller.IsNull() {
				controller.Call("postMessage", map[string]interface{}{
					"bldrRefreshBrowserIndex": true,
				})
				return
			}
		}
	}

	fetchFn := global.Get("fetch")
	if fetchFn.IsUndefined() || fetchFn.IsNull() {
		return
	}

	promise := fetchFn.Invoke(browserIndexPath, map[string]interface{}{
		"cache": "reload",
	})
	if !promise.Truthy() || promise.Get("catch").IsUndefined() {
		return
	}

	var catchFn js.Func
	var finallyFn js.Func
	catchFn = js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
		if le != nil && len(args) > 0 {
			le.Debugf("browser index cache refresh failed: %s", args[0].String())
		}
		return nil
	})
	finallyFn = js.FuncOf(func(js.Value, []js.Value) interface{} {
		catchFn.Release()
		finallyFn.Release()
		return nil
	})
	promise.Call("catch", catchFn).Call("finally", finallyFn)
}
