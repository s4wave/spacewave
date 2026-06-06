//go:build js || goscript

package spacewave_launcher_controller

// Browser and GoScript launcher builds do not need the native release-world
// CDN bootstrap packages. Browser release entrypoints embed the app manifests
// directly and the launcher still fetches signed DistConfig updates.
import (
	_ "github.com/s4wave/spacewave/db/block/store/overlay"
	_ "github.com/s4wave/spacewave/db/block/store/rpc/server"
	_ "github.com/s4wave/spacewave/db/object/peer"
)
