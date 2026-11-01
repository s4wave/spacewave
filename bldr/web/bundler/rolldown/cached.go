//go:build !js

package bldr_web_bundler_rolldown

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strings"

	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	bldr_util_buildcache "github.com/s4wave/spacewave/bldr/util/buildcache"
	"github.com/s4wave/spacewave/bldr/util/npm"
	"github.com/sirupsen/logrus"
)

// buildCacheVersion changes when the cached result or its key changes shape.
const buildCacheVersion = "bldr rolldown build v2"

// BuildCached returns the result of an earlier build of req from toolRoot
// whose inputs all still hold the same content, after restoring its outputs
// under the output root. Otherwise it runs build, which builds req, and
// records the result in cache. Callers acquire build resources inside build,
// so a replay costs none.
func BuildCached(
	le *logrus.Entry,
	cache *bldr_util_buildcache.Cache,
	toolRoot string,
	req *BuildRequest,
	build func() (*BuildResult, error),
) (*BuildResult, error) {
	// Key the build by its whole request and tool root. The request's JSON
	// form orders map fields, so equal requests share a key.
	requestJSON, err := protojson.Marshal(protojson.MarshalerConfig{}, req)
	if err != nil {
		return nil, errors.Wrap(err, "marshal build request")
	}
	toolRoot = resolveToolRoot(toolRoot)
	key := sha256.Sum256(slices.Concat(
		[]byte(buildCacheVersion), []byte{0},
		[]byte(toolRoot), []byte{0},
		requestJSON,
	))

	// A clean build empties the output root first, and so does a replay.
	if req.GetCleanOutputDir() {
		if err := os.RemoveAll(req.GetOutputRoot()); err != nil {
			return nil, errors.Wrap(err, "clean output root")
		}
	}

	// Record the build's inputs, plus the toolchain that ran it, and its
	// outputs.
	value, err := cache.Do(le, key[:], req.GetOutputRoot(), func() (*bldr_util_buildcache.Step, error) {
		// Run the build and encode its result.
		result, err := build()
		if err != nil {
			return nil, err
		}
		value, err := result.MarshalVT()
		if err != nil {
			return nil, err
		}

		// Record what the build read and wrote.
		step := &bldr_util_buildcache.Step{
			Inputs: slices.Concat(
				result.GetInputs(),
				ToolchainInputs(toolRoot, filepath.Join(toolRoot, "web", "bundler", "rolldown")),
			),
			Value: value,
		}
		for _, output := range result.GetOutputs() {
			step.Outputs = append(step.Outputs, output.GetPath())
		}
		return step, nil
	})
	if err != nil {
		return nil, err
	}
	result := &BuildResult{}
	if err := result.UnmarshalVT(value); err != nil {
		return nil, errors.Wrap(err, "decode cached build result")
	}
	return result, nil
}

// ToolchainInputs returns the files that pin a bldr bundler toolchain: the
// dependency manifests and their declared patches under toolRoot, and the
// driver scripts directly in scriptDir. A build cache records them as inputs,
// so a toolchain change rebuilds instead of replaying an older toolchain's
// output.
func ToolchainInputs(toolRoot, scriptDir string) []string {
	// Collect the dependency manifests that exist.
	var inputs []string
	depsDir := filepath.Join(resolveToolRoot(toolRoot), "dist", "deps")
	for _, name := range []string{"package.json", "bun.lock"} {
		path := filepath.Join(depsDir, name)
		if _, err := os.Stat(path); err == nil {
			inputs = append(inputs, path)
		}
	}

	// Compiler patch contents invalidate the CSS and script outputs they can change.
	if data, err := os.ReadFile(filepath.Join(depsDir, "package.json")); err == nil {
		if paths, err := npm.ParseBunPatchPaths(data); err == nil {
			for _, name := range paths {
				inputs = append(inputs, filepath.Join(depsDir, name))
			}
		}
	}

	// Collect the driver scripts, leaving out their tests.
	dirents, _ := os.ReadDir(scriptDir)
	for _, dirent := range dirents {
		name := dirent.Name()
		switch {
		case !dirent.Type().IsRegular(), strings.HasSuffix(name, ".test.ts"):
		case slices.Contains([]string{".ts", ".mjs", ".js"}, filepath.Ext(name)):
			inputs = append(inputs, filepath.Join(scriptDir, name))
		}
	}
	return inputs
}
