package determine_cjs_exports_exec

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	determine_cjs_exports "github.com/aperturerobotics/bldr/web/pkg/esbuild/determine-cjs-exports"
	"github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ExecDetermineCjsExports uses the cjs lexer to determine the list of named exports for a path.
func ExecDetermineCjsExports(
	ctx context.Context,
	le *logrus.Entry,
	codeRootPath,
	importPath string,
) (*determine_cjs_exports.CjsExportsResult, error) {
	// cat determine-cjs-exports.mjs | node --input-type=module - 'node_modules/react'
	args := []string{
		"--input-type=module",
		"-",
		importPath,
	}
	cmd := exec.NewCmd("node", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stdin = strings.NewReader(determine_cjs_exports.GetDetermineCjsExportsScript())
	cmd.Dir = codeRootPath
	if err := exec.StartAndWait(ctx, le, cmd); err != nil {
		return nil, err
	}

	outStr := strings.TrimSpace(out.String())
	if !strings.HasPrefix(outStr, "{") {
		return nil, errors.Errorf("determine-cjs-exports.mjs failed: %s", outStr)
	}
	result := &determine_cjs_exports.CjsExportsResult{}
	if err := json.Unmarshal([]byte(outStr), result); err != nil {
		return nil, errors.Errorf("determine-cjs-exports.mjs failed: %s", err.Error())
	}
	if result.Error != "" {
		return nil, errors.Errorf("determine-cjs-exports.mjs failed: %s", result.Error)
	}

	return result, nil
}
