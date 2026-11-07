// Command spacewave-pack-copy copies what a set of roots reaches from one pack
// store to a clean one. See packcopy_cli.
package main

import packcopy_cli "github.com/s4wave/spacewave/db/block/packcopy/cli"

// main runs the command the arguments name.
func main() {
	packcopy_cli.Main("spacewave-pack-copy", nil)
}
