//go:build !js

package bldr_web_bundler_vite

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"slices"

	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	bldr_util_buildcache "github.com/s4wave/spacewave/bldr/util/buildcache"
	bldr_web_bundler_rolldown "github.com/s4wave/spacewave/bldr/web/bundler/rolldown"
	"github.com/sirupsen/logrus"
)

// buildCacheVersion changes when the cached result or its key changes shape.
const buildCacheVersion = "bldr vite build v3"

// BuildFunc runs one Vite build.
type BuildFunc func(ctx context.Context, req *BuildRequest) (*BuildResponse, error)

// buildFailure carries a failed build's response through the cache, which
// records only successful builds.
type buildFailure struct {
	resp *BuildResponse
}

// Error returns the failed build's message.
func (f *buildFailure) Error() string {
	return f.resp.GetError()
}

// BuildCached returns the response of an earlier build of req whose inputs all
// still hold the same content, after restoring its outputs under req's OutDir.
// Otherwise it runs build and records a successful response in cache. The key
// leaves out OutDir and CacheDir, which name per platform working directories
// without changing the output, so platforms that build the same bundle share
// one build. sqlSchema is the schema the Vite process serves to plugins as
// DATABASE_URL, which the request does not carry. Callers start Vite inside
// build, so a replay starts nothing.
func BuildCached(
	ctx context.Context,
	le *logrus.Entry,
	cache *bldr_util_buildcache.Cache,
	sqlSchema string,
	req *BuildRequest,
	build BuildFunc,
) (*BuildResponse, error) {
	// Key the build by its SQL schema and its request without its working
	// directories. The JSON form orders map fields, so equal requests share a
	// key.
	keyReq := req.CloneVT()
	keyReq.OutDir, keyReq.CacheDir = "", ""
	requestJSON, err := protojson.Marshal(protojson.MarshalerConfig{}, keyReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal build request")
	}
	key := sha256.Sum256(slices.Concat(
		[]byte(buildCacheVersion), []byte{0},
		[]byte(sqlSchema), []byte{0},
		requestJSON,
	))

	// Record the files the build read, which Vite reports relative to the
	// root directory, the toolchain that ran it, and the files it wrote.
	value, err := cache.Do(le, key[:], req.GetOutDir(), func() (*bldr_util_buildcache.Step, error) {
		// Run the build and encode a successful response.
		resp, err := build(ctx, req)
		if err != nil {
			return nil, err
		}
		if !resp.GetSuccess() {
			return nil, &buildFailure{resp: resp}
		}
		value, err := resp.MarshalVT()
		if err != nil {
			return nil, err
		}

		// Record what the build read and wrote.
		step := &bldr_util_buildcache.Step{Outputs: resp.GetOutputFiles(), Value: value}
		for _, input := range resp.GetInputFiles() {
			if !filepath.IsAbs(input) {
				input = filepath.Join(req.GetRootDir(), input)
			}
			step.Inputs = append(step.Inputs, input)
		}
		distDir := req.GetDistDir()
		scriptDir := filepath.Dir(filepath.Join(distDir, ResolveViteEntrypointPath(distDir)))
		step.Inputs = append(step.Inputs, bldr_web_bundler_rolldown.ToolchainInputs(distDir, scriptDir)...)
		return step, nil
	})
	if failure := (*buildFailure)(nil); errors.As(err, &failure) {
		return failure.resp, nil
	}
	if err != nil {
		return nil, err
	}
	resp := &BuildResponse{}
	if err := resp.UnmarshalVT(value); err != nil {
		return nil, errors.Wrap(err, "decode cached build response")
	}
	return resp, nil
}
