package determine_cjs_exports

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aperturerobotics/esbuild/pkg/cjsexports"
	"github.com/pkg/errors"
)

// identRegexp matches valid JavaScript identifiers.
var identRegexp = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$]*$`)

// reservedWords is the set of JavaScript reserved words to exclude from exports.
var reservedWords = map[string]bool{
	"abstract":     true,
	"arguments":    true,
	"await":        true,
	"boolean":      true,
	"break":        true,
	"byte":         true,
	"case":         true,
	"catch":        true,
	"char":         true,
	"class":        true,
	"const":        true,
	"continue":     true,
	"debugger":     true,
	"default":      true,
	"delete":       true,
	"do":           true,
	"double":       true,
	"else":         true,
	"enum":         true,
	"eval":         true,
	"export":       true,
	"extends":      true,
	"false":        true,
	"final":        true,
	"finally":      true,
	"float":        true,
	"for":          true,
	"function":     true,
	"goto":         true,
	"if":           true,
	"implements":   true,
	"import":       true,
	"in":           true,
	"instanceof":   true,
	"int":          true,
	"interface":    true,
	"let":          true,
	"long":         true,
	"native":       true,
	"new":          true,
	"null":         true,
	"package":      true,
	"private":      true,
	"protected":    true,
	"public":       true,
	"return":       true,
	"short":        true,
	"static":       true,
	"super":        true,
	"switch":       true,
	"synchronized": true,
	"this":         true,
	"throw":        true,
	"throws":       true,
	"transient":    true,
	"true":         true,
	"try":          true,
	"typeof":       true,
	"var":          true,
	"void":         true,
	"volatile":     true,
	"while":        true,
	"with":         true,
	"yield":        true,
}

// builtinNodeModules is the set of built-in Node.js modules.
var builtinNodeModules = map[string]bool{
	"assert":              true,
	"async_hooks":         true,
	"child_process":       true,
	"cluster":             true,
	"buffer":              true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"dns":                 true,
	"domain":              true,
	"events":              true,
	"fs":                  true,
	"fs/promises":         true,
	"http":                true,
	"http2":               true,
	"https":               true,
	"inspector":           true,
	"module":              true,
	"net":                 true,
	"os":                  true,
	"path":                true,
	"path/posix":          true,
	"path/win32":          true,
	"perf_hooks":          true,
	"process":             true,
	"punycode":            true,
	"querystring":         true,
	"readline":            true,
	"repl":                true,
	"stream":              true,
	"stream/promises":     true,
	"stream/web":          true,
	"_stream_duplex":      true,
	"_stream_passthrough": true,
	"_stream_readable":    true,
	"_stream_transform":   true,
	"_stream_writable":    true,
	"string_decoder":      true,
	"sys":                 true,
	"timers":              true,
	"tls":                 true,
	"trace_events":        true,
	"tty":                 true,
	"url":                 true,
	"util":                true,
	"v8":                  true,
	"vm":                  true,
	"worker_threads":      true,
	"zlib":                true,
}

// requireItem is a queued file to parse for CJS exports.
type requireItem struct {
	path     string
	callMode bool
}

// AnalyzeCjsExports analyzes a module's CJS exports purely in Go.
// codeRootPath is the directory to resolve from.
// importPath is the module to analyze (e.g., "./index.js", "react").
// nodePaths are additional directories containing node_modules.
func AnalyzeCjsExports(codeRootPath, importPath string, nodePaths []string) (*CjsExportsResult, error) {
	// Resolve the entry file.
	entry, err := ResolveModuleWithNodePaths(codeRootPath, importPath, nodePaths)
	if err != nil {
		return nil, errors.Wrap(err, "resolve "+importPath)
	}

	// Handle JSON files: extract top-level object keys.
	if strings.HasSuffix(entry, ".json") {
		keys, err := getJSONKeys(entry)
		if err != nil {
			return nil, errors.Wrap(err, "read json "+entry)
		}
		return verifyExports(keys), nil
	}

	// Only process .js, .cjs, .mjs files.
	if !strings.HasSuffix(entry, ".js") && !strings.HasSuffix(entry, ".cjs") && !strings.HasSuffix(entry, ".mjs") {
		return verifyExports(nil), nil
	}

	// Process the requires queue.
	var collected []string
	requires := []requireItem{{path: entry, callMode: false}}
	for len(requires) > 0 {
		// Pop from queue.
		req := requires[len(requires)-1]
		requires = requires[:len(requires)-1]

		code, readErr := os.ReadFile(req.path)
		if readErr != nil {
			return nil, errors.Wrap(readErr, "read "+req.path)
		}

		result, parseErr := cjsexports.Parse(string(code), req.path, cjsexports.Options{
			NodeEnv:  "production",
			CallMode: req.callMode,
		})
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, "parse "+req.path)
		}

		// Optimization: single reexport with no local exports and nothing collected yet.
		if len(result.Reexports) == 1 &&
			len(result.Exports) == 0 &&
			len(collected) == 0 {
			reexp := result.Reexports[0]
			if !strings.HasSuffix(reexp, "()") &&
				!builtinNodeModules[reexp] &&
				len(reexp) > 0 &&
				(reexp[0] >= 'a' && reexp[0] <= 'z' || reexp[0] >= 'A' && reexp[0] <= 'Z' || reexp[0] == '@') {
				return &CjsExportsResult{
					Reexport:      reexp,
					ExportDefault: false,
					Exports:       []string{},
				}, nil
			}
		}

		collected = append(collected, result.Exports...)

		for _, reexp := range result.Reexports {
			callMode := strings.HasSuffix(reexp, "()")
			if callMode {
				reexp = reexp[:len(reexp)-2]
			}

			// Skip built-in Node modules for web bundles.
			if builtinNodeModules[reexp] {
				continue
			}

			resolved, resolveErr := ResolveModuleWithNodePaths(filepath.Dir(req.path), reexp, nodePaths)
			if resolveErr != nil {
				return nil, errors.Wrap(resolveErr, "resolve reexport "+reexp)
			}

			if strings.HasSuffix(resolved, ".json") {
				keys, jsonErr := getJSONKeys(resolved)
				if jsonErr != nil {
					return nil, errors.Wrap(jsonErr, "read json "+resolved)
				}
				collected = append(collected, keys...)
				continue
			}

			requires = append(requires, requireItem{path: resolved, callMode: callMode})
		}
	}

	return verifyExports(collected), nil
}

// verifyExports filters and deduplicates export names.
func verifyExports(names []string) *CjsExportsResult {
	exportDefault := false
	seen := make(map[string]bool)
	var exports []string

	for _, name := range names {
		if name == "default" {
			exportDefault = true
		}
		if !identRegexp.MatchString(name) {
			continue
		}
		if reservedWords[name] {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		exports = append(exports, name)
	}

	if exports == nil {
		exports = []string{}
	}

	return &CjsExportsResult{
		ExportDefault: exportDefault,
		Exports:       exports,
	}
}

// getJSONKeys reads a JSON file and returns the top-level object keys.
func getJSONKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		// Not an object (could be array, string, etc.).
		return nil, nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys, nil
}
