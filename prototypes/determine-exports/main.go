package main

import (
	"fmt"
	"os"
	"path/filepath"

	determine_cjs_exports "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports"
)

func main() {
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	codeRootDir := filepath.Join(wd, "../../")

	imp := "react"
	if len(os.Args) > 1 {
		imp = os.Args[1]
	}

	result, err := determine_cjs_exports.AnalyzeCjsExports(codeRootDir, imp, nil)
	if err != nil {
		return err
	}
	fmt.Printf("exports (%d): %v\n", len(result.Exports), result.Exports)
	fmt.Printf("exportDefault: %v\n", result.ExportDefault)
	if result.Reexport != "" {
		fmt.Printf("reexport: %s\n", result.Reexport)
	}

	return nil
}
