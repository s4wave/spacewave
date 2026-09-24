//go:build js

package spacewave_compose

import "github.com/s4wave/spacewave/bldr/entrypoint/compose"

// composeNative is empty in browser hosts, which have no native process
// controllers or command line.
func composeNative(*compose.Composition) {}
