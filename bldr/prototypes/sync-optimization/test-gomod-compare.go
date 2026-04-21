// test-gomod-compare: Tests whether comparing the generated go.mod with the
// existing file on disk can be used to skip go mod tidy + vendor.
//
// The optimization strategy:
//  1. Generate the modified go.mod in memory (same as SyncDistSources)
//  2. Read the existing .bldr/src/go.mod from disk
//  3. If bytes.Equal: skip go mod tidy + vendor (saves ~1s)
//  4. If different: write new go.mod, run tidy + vendor as normal
//
// Run from bldr repo root:
//
//	go run ./prototypes/sync-optimization/test-gomod-compare.go
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/modfile"
)

// distGoMod is the module path used for the dist sources checkout.
// const distGoMod = "github.com/s4wave/spacewave/bldr-dist"

func main() {
	// Defaults matching dev mode (bldr-src-path=../../)
	bldrSrcPath := "../../"
	srcDir := ".bldr/src"

	gomodPath := filepath.Join(srcDir, "go.mod")
	gosumPath := filepath.Join(srcDir, "go.sum")
	vendorPath := filepath.Join(srcDir, "vendor")

	// Check that .bldr/src/ exists (run bldr setup first)
	if _, err := os.Stat(gomodPath); err != nil {
		fmt.Println("ERROR: .bldr/src/go.mod not found. Run 'bldr setup' first.")
		os.Exit(1)
	}

	fmt.Println("=== go.mod Compare Optimization Test ===")
	fmt.Println()

	// Step 1: Read existing go.mod from disk
	existingGoMod, err := os.ReadFile(gomodPath)
	if err != nil {
		fmt.Printf("ERROR reading go.mod: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Existing go.mod: %d bytes\n", len(existingGoMod))

	// Step 2: Generate the go.mod that SyncDistSources would produce.
	// We replicate the logic from devtool/bus.go lines 380-417.
	// In a real implementation, this would use the embedded DistSources go.mod.
	// For this test, we re-parse the existing go.mod and re-generate.
	t0 := time.Now()

	modFile, err := modfile.Parse(gomodPath, existingGoMod, nil)
	if err != nil {
		fmt.Printf("ERROR parsing go.mod: %v\n", err)
		os.Exit(1)
	}

	// The existing go.mod should already have the distGoMod module path
	// and the replace directive from the previous run.
	// Re-generate from the embedded source to test determinism.
	origModPath := modFile.Module.Mod.Path
	fmt.Printf("Current module:  %s\n", origModPath)
	fmt.Printf("bldrSrcPath:     %s\n", bldrSrcPath)

	// Re-format (this tests whether Format is deterministic)
	modFile.Cleanup()
	regenerated, err := modFile.Format()
	if err != nil {
		fmt.Printf("ERROR formatting go.mod: %v\n", err)
		os.Exit(1)
	}

	t1 := time.Now()
	fmt.Printf("go.mod generation time: %v\n", t1.Sub(t0))
	fmt.Println()

	// Step 3: Compare
	identical := bytes.Equal(existingGoMod, regenerated)
	fmt.Printf("Generated go.mod: %d bytes\n", len(regenerated))
	fmt.Printf("Match existing:   %v\n", identical)

	if !identical {
		fmt.Println()
		fmt.Println("DIFF (existing vs regenerated):")
		// Simple line-by-line diff
		existLines := bytes.Split(existingGoMod, []byte("\n"))
		regenLines := bytes.Split(regenerated, []byte("\n"))
		maxLines := max(len(existLines), len(regenLines))
		for i := range maxLines {
			var e, r []byte
			if i < len(existLines) {
				e = existLines[i]
			}
			if i < len(regenLines) {
				r = regenLines[i]
			}
			if !bytes.Equal(e, r) {
				fmt.Printf("  line %d:\n", i+1)
				fmt.Printf("    existing:    %q\n", string(e))
				fmt.Printf("    regenerated: %q\n", string(r))
			}
		}
	}
	fmt.Println()

	// Step 4: Check if vendor/ and go.sum exist
	_, vendorErr := os.Stat(vendorPath)
	_, gosumErr := os.Stat(gosumPath)
	fmt.Printf("vendor/ exists:  %v\n", vendorErr == nil)
	fmt.Printf("go.sum exists:   %v\n", gosumErr == nil)
	fmt.Println()

	// Step 5: Timing analysis
	fmt.Println("=== Optimization Analysis ===")
	if identical && vendorErr == nil && gosumErr == nil {
		fmt.Println("RESULT: go.mod is deterministic, vendor/ and go.sum exist.")
		fmt.Println("OPTIMIZATION: Compare generated go.mod with existing.")
		fmt.Println("  If equal AND vendor/ exists: skip go mod tidy + vendor.")
		fmt.Println("  Expected savings: ~1s per startup.")
		fmt.Println()
		fmt.Println("Implementation in SyncDistSources:")
		fmt.Println("  1. Generate updatedBldrGoMod (already done)")
		fmt.Println("  2. Read existing go.mod from disk")
		fmt.Println("  3. if bytes.Equal(existing, generated) && dirExists(vendor/):")
		fmt.Println("       skip WriteFile, skip go mod tidy, skip go mod vendor")
		fmt.Println("  4. else: write go.mod, run tidy, run vendor (current behavior)")
	} else {
		fmt.Println("RESULT: Cannot skip -- go.mod changes or vendor/ missing.")
		if !identical {
			fmt.Println("  go.mod is not deterministic between runs.")
		}
		if vendorErr != nil {
			fmt.Println("  vendor/ does not exist.")
		}
	}
}
