//go:build linux

package boot

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// borrowedNativeVolume leaves physical Close with Data. Embedding the concrete
// native Volume preserves atomic publication, preparation, retention and stages;
// embedding volume.Volume would silently erase those optional capabilities.
type borrowedNativeVolume struct {
	// Volume supplies the complete native storage and coordination domain.
	*volume_s4db.Volume
}

// Close leaves physical storage open until Data has released all its users.
func (v *borrowedNativeVolume) Close() error {
	return nil
}

// _ checks the borrowed view's storage and publication contracts.
var (
	_ volume.Volume         = (*borrowedNativeVolume)(nil)
	_ block.AtomicPublisher = (*borrowedNativeVolume)(nil)
)
