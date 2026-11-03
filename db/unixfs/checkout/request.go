package unixfs_checkout

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// Request explicitly selects immutable content and its destination. Callers
// with equal destination/roots must use equivalent Codec and Check contracts.
// Root and Directory hold only reproducible, hash-verified cache files;
// application state belongs elsewhere. Remotes must supply at least one source.
type Request struct {
	// Logger records phase timings and the source of fetched bytes.
	Logger *logrus.Entry
	// Root holds the destination's lease coordinator.
	Root string
	// Directory is the final directory, strictly below Root.
	Directory string
	// Description names the checkout in errors and logs.
	Description string
	// Roots lists the trees to assemble in order.
	Roots []*block.BlockRef
	// Codec describes their immutable tree encoding.
	Codec Codec
	// Check validates every file the application requires before publication.
	Check func(string) error
	// Fetch supplies remote access and the subscribing caller's observations.
	Fetch Fetch
}
