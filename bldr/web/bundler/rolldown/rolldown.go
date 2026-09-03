package bldr_web_bundler_rolldown

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	bldr_exec "github.com/s4wave/spacewave/bldr/util/exec"
	"github.com/s4wave/spacewave/bldr/util/npm"
	"github.com/sirupsen/logrus"
)

var (
	validFormats     = map[string]struct{}{"es": {}, "cjs": {}, "iife": {}}
	validPlatforms   = map[string]struct{}{"browser": {}, "node": {}, "neutral": {}}
	validSourcemaps  = map[string]struct{}{"none": {}, "inline": {}, "external": {}, "both": {}}
	validLoaderKinds = map[string]struct{}{
		"js": {}, "jsx": {}, "ts": {}, "tsx": {}, "json": {},
		"text": {}, "dataurl": {}, "base64": {}, "binary": {}, "asset": {},
	}
	validOutputKinds = map[string]struct{}{"javascript": {}, "map": {}, "asset": {}}
)

// ValidateBuildRequest validates the direct Rolldown build contract.
func ValidateBuildRequest(req *BuildRequest) error {
	if req == nil {
		return errors.New("build request is nil")
	}
	for field, value := range map[string]string{
		"working_dir":    req.GetWorkingDir(),
		"source_root":    req.GetSourceRoot(),
		"output_root":    req.GetOutputRoot(),
		"bldr_dist_root": req.GetBldrDistRoot(),
	} {
		if err := validateAbsolutePath(field, value); err != nil {
			return err
		}
	}
	if len(req.GetEntrypoints()) == 0 {
		return errors.New("entrypoints are required")
	}
	seenNames := make(map[string]struct{}, len(req.GetEntrypoints()))
	for i, entrypoint := range req.GetEntrypoints() {
		if entrypoint == nil {
			return errors.Errorf("entrypoints[%d] is nil", i)
		}
		name := entrypoint.GetName()
		if strings.TrimSpace(name) == "" {
			return errors.Errorf("entrypoints[%d].name is required", i)
		}
		if _, ok := seenNames[name]; ok {
			return errors.Errorf("entrypoints[%d].name %q is duplicated", i, name)
		}
		seenNames[name] = struct{}{}
		field := "entrypoints[" + strconv.Itoa(i) + "].input_path"
		if err := validateAbsolutePath(field, entrypoint.GetInputPath()); err != nil {
			return err
		}
	}
	if _, ok := validFormats[req.GetFormat()]; !ok {
		return errors.Errorf("format %q is invalid", req.GetFormat())
	}
	if _, ok := validPlatforms[req.GetPlatform()]; !ok {
		return errors.Errorf("platform %q is invalid", req.GetPlatform())
	}
	if _, ok := validSourcemaps[req.GetSourcemap()]; !ok {
		return errors.Errorf("sourcemap %q is invalid", req.GetSourcemap())
	}
	if req.GetCodeSplitting() && req.GetFormat() != "es" {
		return errors.New("code_splitting requires format es")
	}
	if req.GetFormat() == "iife" && len(req.GetEntrypoints()) != 1 {
		return errors.New("format iife requires exactly one entrypoint")
	}
	if req.GetFormat() == "iife" && strings.TrimSpace(req.GetGlobalName()) == "" {
		return errors.New("format iife requires global_name")
	}
	for field, value := range map[string]string{
		"entry_file_names": req.GetEntryFileNames(),
		"chunk_file_names": req.GetChunkFileNames(),
		"asset_file_names": req.GetAssetFileNames(),
	} {
		if strings.TrimSpace(value) == "" {
			return errors.Errorf("%s is required", field)
		}
		if logicalPathIsAbs(value) || outputNameEscapes(value) {
			return errors.Errorf("%s must be a relative contained path pattern", field)
		}
	}
	for key, loader := range req.GetLoaders() {
		if strings.TrimSpace(key) == "" {
			return errors.New("loaders contains an empty key")
		}
		if _, ok := validLoaderKinds[loader]; !ok {
			return errors.Errorf("loader %q for %q is invalid", loader, key)
		}
	}
	if policy := req.GetGoscript(); policy != nil && policy.GetOutputRoot() != "" {
		if err := validateAbsolutePath("goscript.output_root", policy.GetOutputRoot()); err != nil {
			return err
		}
	}
	for i, injectPath := range req.GetInject() {
		if err := validateAbsolutePath("inject["+strconv.Itoa(i)+"]", injectPath); err != nil {
			return err
		}
	}
	for overridePath := range req.GetSourceOverrides() {
		if err := validateAbsolutePath("source_overrides key", overridePath); err != nil {
			return err
		}
	}
	for prefix, aliasRoot := range req.GetPrefixAliases() {
		if strings.TrimSpace(prefix) == "" {
			return errors.New("prefix_aliases contains an empty prefix")
		}
		if err := validateAbsolutePath("prefix_aliases value", aliasRoot); err != nil {
			return err
		}
	}
	return nil
}

// Build runs one Bun Rolldown/Oxc build from toolRoot and returns its structured result.
func Build(ctx context.Context, le *logrus.Entry, stateDir, toolRoot string, req *BuildRequest) (*BuildResult, error) {
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	if err := ValidateBuildRequest(req); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(req.GetWorkingDir(), 0o755); err != nil {
		return nil, errors.Wrap(err, "create working directory")
	}
	if err := validateAbsolutePath("tool_root", toolRoot); err != nil {
		return nil, err
	}
	toolRoot = resolveToolRoot(toolRoot)
	dependencyRoot, err := ensureDependencyRoot(ctx, le, stateDir, toolRoot)
	if err != nil {
		return nil, err
	}
	bunPath, err := npm.ResolveBunPath(ctx, le, stateDir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve bun")
	}
	runnerPath := filepath.Join(toolRoot, "web", "bundler", "rolldown", "run-build.mjs")
	if info, statErr := os.Stat(runnerPath); statErr != nil || info.IsDir() {
		if statErr == nil {
			statErr = errors.New("is a directory")
		}
		return nil, errors.Wrapf(statErr, "rolldown runner %s", runnerPath)
	}
	requestFile, err := os.CreateTemp(req.GetWorkingDir(), ".bldr-rolldown-request-*")
	if err != nil {
		return nil, errors.Wrap(err, "create request protocol file")
	}
	requestPath := requestFile.Name()
	defer os.Remove(requestPath)
	resultFile, err := os.CreateTemp(req.GetWorkingDir(), ".bldr-rolldown-result-*")
	if err != nil {
		requestFile.Close()
		os.Remove(requestPath)
		return nil, errors.Wrap(err, "create result protocol file")
	}
	resultPath := resultFile.Name()
	defer os.Remove(resultPath)
	if err := resultFile.Close(); err != nil {
		requestFile.Close()
		return nil, errors.Wrap(err, "close result protocol file")
	}
	requestJSON, err := protojson.Marshal(protojson.MarshalerConfig{}, req)
	if err != nil {
		requestFile.Close()
		return nil, errors.Wrap(err, "marshal build request")
	}
	if _, err := requestFile.Write(requestJSON); err != nil {
		requestFile.Close()
		return nil, errors.Wrap(err, "write build request")
	}
	if err := requestFile.Close(); err != nil {
		return nil, errors.Wrap(err, "close request protocol file")
	}

	cmd := bldr_exec.NewCmd(ctx, bunPath, runnerPath, requestPath, resultPath, dependencyRoot)
	cmd.Dir = req.GetWorkingDir()
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "NODE_DISABLE_COLORS=1", "FORCE_COLOR=0", "CI=1")
	runErr := bldr_exec.StartAndWait(ctx, le, cmd)
	result, parseErr := readBuildResult(resultPath)
	if result != nil {
		sortBuildResult(result)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	if runErr != nil {
		return result, runnerFailure(runErr, parseErr, result)
	}
	if parseErr != nil {
		return nil, errors.Wrap(parseErr, "parse rolldown build result")
	}
	if err := validateBuildResult(result, req.GetOutputRoot()); err != nil {
		return result, err
	}
	return result, nil
}

func resolveToolRoot(root string) string {
	if _, err := os.Stat(filepath.Join(root, "dist", "deps", "package.json")); err == nil {
		return root
	}
	nested := filepath.Join(root, "bldr")
	if _, err := os.Stat(filepath.Join(nested, "dist", "deps", "package.json")); err == nil {
		return nested
	}
	return root
}

func validateAbsolutePath(field, value string) error {
	if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
		return errors.Errorf("%s must be a nonempty absolute path", field)
	}
	return nil
}

func ensureDependencyRoot(ctx context.Context, le *logrus.Entry, stateDir, bldrDistRoot string) (string, error) {
	depsRoot := filepath.Join(bldrDistRoot, "dist", "deps")
	if sourceRolldownCurrent(depsRoot) {
		return depsRoot, nil
	}
	packageJSON := filepath.Join(depsRoot, "package.json")
	installRoot, err := npm.EnsureSharedBunInstall(
		ctx, le, stateDir, packageJSON, filepath.Join(stateDir, "build-web-pkgs"),
	)
	if err != nil {
		return "", errors.Wrap(err, "install rolldown dependencies")
	}
	if info, err := os.Stat(filepath.Join(installRoot, "node_modules", "rolldown", "dist", "index.mjs")); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("is a directory")
		}
		return "", errors.Wrap(err, "rolldown dependency missing after install")
	}
	return installRoot, nil
}

func sourceRolldownCurrent(depsRoot string) bool {
	data, err := os.ReadFile(filepath.Join(depsRoot, "package.json"))
	if err != nil {
		return false
	}
	var parser fastjson.Parser
	manifest, err := parser.ParseBytes(data)
	if err != nil {
		return false
	}
	requiredVersion := string(manifest.GetStringBytes("dependencies", "rolldown"))
	if requiredVersion == "" {
		return false
	}
	data, err = os.ReadFile(filepath.Join(depsRoot, "node_modules", "rolldown", "package.json"))
	if err != nil {
		return false
	}
	manifest, err = parser.ParseBytes(data)
	if err != nil || string(manifest.GetStringBytes("version")) != requiredVersion {
		return false
	}
	info, err := os.Stat(filepath.Join(depsRoot, "node_modules", "rolldown", "dist", "index.mjs"))
	return err == nil && !info.IsDir()
}

func readBuildResult(path string) (*BuildResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := new(BuildResult)
	if err := (protojson.UnmarshalerConfig{}).Unmarshal(data, result); err != nil {
		return nil, err
	}
	return result, nil
}

func runnerFailure(runErr, parseErr error, result *BuildResult) error {
	extra := make([]string, 0, 1)
	if parseErr != nil {
		extra = append(extra, "parse structured result: "+parseErr.Error())
	}
	if result != nil {
		for _, diagnostic := range result.GetDiagnostics() {
			if diagnostic != nil {
				extra = append(extra, formatDiagnostic(diagnostic))
			}
		}
	}
	if len(extra) == 0 {
		return errors.Wrap(runErr, "rolldown runner failed")
	}
	return errors.Wrapf(runErr, "rolldown runner failed: %s", strings.Join(extra, "; "))
}

func validateBuildResult(result *BuildResult, outputRoot string) error {
	if result == nil {
		return errors.New("rolldown build result is nil")
	}
	tool := result.GetTool()
	if tool == nil || tool.GetRolldownVersion() == "" || tool.GetBunVersion() == "" || tool.GetPlatform() == "" || tool.GetArch() == "" {
		return errors.New("rolldown build result has incomplete tool identity")
	}
	for i, input := range result.GetInputs() {
		if strings.TrimSpace(input) == "" || !filepath.IsAbs(input) || filepath.Clean(input) != input {
			return errors.Errorf("inputs[%d] is not a normalized absolute path: %q", i, input)
		}
	}
	for i, output := range result.GetOutputs() {
		if output == nil {
			return errors.Errorf("outputs[%d] is nil", i)
		}
		if err := validateContainedOutput(output.GetPath(), outputRoot); err != nil {
			return errors.Wrapf(err, "outputs[%d]", i)
		}
		if _, ok := validOutputKinds[output.GetType()]; !ok {
			return errors.Errorf("outputs[%d] has invalid type %q", i, output.GetType())
		}
		if output.GetBytes() < 0 || output.GetGzipBytes() < 0 {
			return errors.Errorf("outputs[%d] has negative byte count", i)
		}
	}
	for name, path := range result.GetEntrypointOutputs() {
		if strings.TrimSpace(name) == "" {
			return errors.New("entrypoint_outputs contains an empty name")
		}
		if err := validateContainedOutput(path, outputRoot); err != nil {
			return errors.Wrapf(err, "entrypoint_outputs[%q]", name)
		}
	}
	return nil
}

func validateContainedOutput(path, outputRoot string) error {
	if strings.TrimSpace(path) == "" || logicalPathIsAbs(path) || outputNameEscapes(path) {
		return errors.Errorf("path %q is not a relative output-root-contained path", path)
	}
	joined := filepath.Join(outputRoot, filepath.FromSlash(path))
	rel, err := filepath.Rel(outputRoot, joined)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.Errorf("path %q escapes output root", path)
	}
	if clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))); clean != path {
		return errors.Errorf("path %q is not normalized", path)
	}
	return nil
}

func logicalPathIsAbs(value string) bool {
	return filepath.IsAbs(value) || path.IsAbs(filepath.ToSlash(value))
}

func outputNameEscapes(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func sortBuildResult(result *BuildResult) {
	slices.Sort(result.Inputs)
	slices.SortFunc(result.Outputs, func(a, b *BuildOutput) int {
		if a == nil && b == nil {
			return 0
		}
		if a == nil {
			return -1
		}
		if b == nil {
			return 1
		}
		for _, pair := range [][2]string{{a.GetPath(), b.GetPath()}, {a.GetType(), b.GetType()}, {a.GetEntrypointName(), b.GetEntrypointName()}, {a.GetSha256(), b.GetSha256()}} {
			if c := strings.Compare(pair[0], pair[1]); c != 0 {
				return c
			}
		}
		if a.GetBytes() < b.GetBytes() {
			return -1
		}
		if a.GetBytes() > b.GetBytes() {
			return 1
		}
		if a.GetGzipBytes() < b.GetGzipBytes() {
			return -1
		}
		if a.GetGzipBytes() > b.GetGzipBytes() {
			return 1
		}
		return 0
	})
	slices.SortFunc(result.Diagnostics, func(a, b *Diagnostic) int {
		if a == nil && b == nil {
			return 0
		}
		if a == nil {
			return -1
		}
		if b == nil {
			return 1
		}
		for _, pair := range [][2]string{{a.GetSeverity(), b.GetSeverity()}, {a.GetMessage(), b.GetMessage()}, {a.GetCode(), b.GetCode()}, {a.GetFile(), b.GetFile()}, {a.GetLineText(), b.GetLineText()}} {
			if c := strings.Compare(pair[0], pair[1]); c != 0 {
				return c
			}
		}
		if a.GetLine() < b.GetLine() {
			return -1
		}
		if a.GetLine() > b.GetLine() {
			return 1
		}
		if a.GetColumn() < b.GetColumn() {
			return -1
		}
		if a.GetColumn() > b.GetColumn() {
			return 1
		}
		return 0
	})
}

func formatDiagnostic(diagnostic *Diagnostic) string {
	location := diagnostic.GetFile()
	if diagnostic.GetLine() != 0 {
		location = location + ":" + strconv.Itoa(int(diagnostic.GetLine())) + ":" +
			strconv.Itoa(int(diagnostic.GetColumn()))
	}
	if location != "" {
		return "[" + diagnostic.GetSeverity() + "] " + location + ": " +
			diagnostic.GetMessage()
	}
	return "[" + diagnostic.GetSeverity() + "] " + diagnostic.GetMessage()
}
