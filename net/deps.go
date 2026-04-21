//go:build deps_only

package bifrost

import (
	// _ imports common with the Makefile and tools
	_ "github.com/aperturerobotics/common"
	// _ imports common aptre cli
	_ "github.com/aperturerobotics/common/cmd/aptre"
)
