// -go:build node_tests
// - +build node_tests
package determine_cjs_exports_exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	determine_cjs_exports "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports"
	"github.com/sirupsen/logrus"
)

func TestExecDetermineCjsExports(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}

	importPath := "react"
	codeRootDir := filepath.Join(wd, "../../../../..")
	exports, err := ExecDetermineCjsExports(ctx, le, codeRootDir, importPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("%#v", exports)

	t.Log(determine_cjs_exports.GenerateRemapExports(importPath, exports))

	if len(exports.Exports) < 10 {
		t.Fatal("expected more exports from react")
	}
}
