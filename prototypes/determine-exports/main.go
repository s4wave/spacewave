package main

import (
	"context"
	"os"
	"path/filepath"

	determine_cjs_exports_exec "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports/exec"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := run(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, le *logrus.Entry) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	codeRootDir := filepath.Join(wd, "../../")

	imp := "react"
	if len(os.Args) > 1 {
		imp = os.Args[1]
	}

	exports, err := determine_cjs_exports_exec.ExecDetermineCjsExports(ctx, le, codeRootDir, imp)
	if err != nil {
		return err
	}
	le.Info(exports)

	return nil
}
