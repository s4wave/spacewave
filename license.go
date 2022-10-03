// Bldr is primarily used with the devtool:
//
// $ go install github.com/aperturerobotics/bldr/cmd/bldr
// $ bldr start electron
package bldr

import "embed"

// License contains the contents of the LICENSE file.
//
//go:embed LICENSE
var License embed.FS
