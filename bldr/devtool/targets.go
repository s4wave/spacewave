//go:build !js

package devtool

import (
	"fmt"
	"slices"
	"strings"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
)

// ListTargets lists available deployment targets.
func (a *DevtoolArgs) ListTargets() error {
	fmt.Println("Available deployment targets:")
	fmt.Println()

	// Collect all target IDs
	targetIDs := bldr_platform.ListBuiltinTargetIDs()
	slices.Sort(targetIDs)

	for _, id := range targetIDs {
		target := bldr_platform.GetBuiltinTarget(id)
		if target == nil {
			continue
		}
		fmt.Printf("  %s\n", target.ID)
		fmt.Printf("    %s\n", target.Description)
		fmt.Printf("    Platforms: %s\n", strings.Join(target.PlatformIDs, ", "))
		fmt.Println()
	}

	fmt.Println("Parameterized targets:")
	fmt.Println()
	fmt.Println("  desktop/{os}/{arch}")
	fmt.Println("    Desktop for a specific OS and architecture")
	fmt.Println("    Example: desktop/darwin/arm64, desktop/linux/amd64")
	fmt.Println()
	fmt.Println("  desktop/cross")
	fmt.Println("    Cross-compile for all common desktop architectures")
	fmt.Println()

	return nil
}
