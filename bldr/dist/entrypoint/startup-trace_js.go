//go:build js && bldr_startup_trace

package dist_entrypoint

import startuptrace "github.com/s4wave/spacewave/bldr/web/entrypoint/startuptrace"

func init() {
	startuptrace.Install()
}
