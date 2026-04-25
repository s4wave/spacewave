//go:build deps_only

package aperture_alpha

import (
	// _ imports common
	_ "github.com/aperturerobotics/common"
	// _ imports common with aptre cli
	_ "github.com/aperturerobotics/common/cmd/aptre"
	// _ imports the bldr cli
	_ "github.com/s4wave/spacewave/bldr/cmd/bldr"
	// _ imports the world manifest fetcher
	_ "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	// _ imports the bldr plugin entrypoint
	_ "github.com/s4wave/spacewave/bldr/plugin/entrypoint"
	// _ imports the bldr dist entrypoint
	_ "github.com/s4wave/spacewave/bldr/dist/entrypoint"
	// _ imports the object store peer controller
	_ "github.com/s4wave/spacewave/db/object/peer"
)
