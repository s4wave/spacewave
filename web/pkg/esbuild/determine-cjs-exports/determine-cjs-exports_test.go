//go:build !js

package determine_cjs_exports

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

func TestGetDetermineCjsExportsScript(t *testing.T) {
	exportsScript := GetDetermineCjsExportsScript()
	if !strings.Contains(exportsScript, "enhanced-resolve") {
		t.FailNow()
	}
}

func TestSupportsExtension(t *testing.T) {
	tests := []bool{
		SupportsExtension("test.js"),
		!SupportsExtension("test.png"),
		!SupportsExtension("png"),
		SupportsExtension("js"),
		!SupportsExtension("jpg"),
		!SupportsExtension(".jpg"),
		!SupportsExtension("test.jpg"),
		SupportsExtension(".js"),
		// SupportsExtension(".mjs"),
		SupportsExtension(""),
	}
	for _, tr := range tests {
		if !tr {
			t.Fatalf("tests failed: %v", tests)
		}
	}
}

func TestDetermineCjsExports_React(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Normally we call passing the script to stdin.
	// Calling directly from the disk here allows us to debug the script.
	args := []string{
		"./determine-cjs-exports.mjs",
		"react",
	}
	cmd := exec.NewCmd(ctx, "node", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Dir = wd
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		t.Fatal(err.Error())
	}

	outStr := strings.TrimSpace(out.String())
	result := &CjsExportsResult{}
	err = json.Unmarshal([]byte(outStr), &result)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(result.Exports) < 10 {
		t.Fatal("expected more exports from react")
	}
	t.Logf("react: exports: %v", result.Exports)
}

func TestDetermineCjsExports_ProtobufEsLite(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}

	scriptPath := filepath.Join(wd, "determine-cjs-exports.mjs")
	cmdDir := filepath.Join(wd, "../../../../node_modules/@aptre/protobuf-es-lite/dist")
	relScriptPath, err := filepath.Rel(cmdDir, scriptPath)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Normally we call passing the script to stdin.
	// Calling directly from the disk here allows us to debug the script.
	args := []string{
		relScriptPath,
		"./index.js",
	}
	cmd := exec.NewCmd(ctx, "node", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Dir = cmdDir
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		t.Fatal(err.Error())
	}

	outStr := strings.TrimSpace(out.String())
	result := &CjsExportsResult{}
	err = json.Unmarshal([]byte(outStr), &result)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("@aptre/protobuf-es-lite: exports: %v", result.Exports)
	if len(result.Exports) != 0 {
		t.Fatal("expected no cjs exports from @aptre/protobuf-es-lite")
	}
}
