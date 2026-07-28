//go:build !js

package releasewasm

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	e2eharness "github.com/s4wave/spacewave/e2e/harness"
	"github.com/s4wave/spacewave/e2e/releasewasm/artifact"
	"github.com/sirupsen/logrus"
)

type releaseArtifact struct {
	releaseDir   string
	prerenderDir string
}

// identityFields carries every digest that determines an artifact identity into
// one log entry.
func identityFields(identity *artifact.Identity) logrus.Fields {
	fields := make(logrus.Fields, 8)
	for key, value := range identity.Summary() {
		fields[key] = value
	}
	return fields
}

type releaseShape struct {
	le       *logrus.Entry
	repoRoot string
	storeDir string
	identity *artifact.Identity
}

func newReleaseShape(le *logrus.Entry, repoRoot, storeDir string, identity *artifact.Identity) *releaseShape {
	return &releaseShape{le: le, repoRoot: repoRoot, storeDir: storeDir, identity: identity}
}

func (s *releaseShape) ContentKey(context.Context) (string, error) {
	return s.identity.Digest, nil
}

func (s *releaseShape) Lookup(ctx context.Context, key string) ([]e2eharness.Generation[releaseArtifact], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	generations, err := artifact.ValidGenerations(s.storeDir, s.identity)
	if err != nil {
		return nil, err
	}
	if len(generations) == 0 {
		// A miss makes the caller rebuild, which is the expensive path and is
		// impossible wherever no compiler is available. Report the identity
		// being looked for alongside the generations that were present, since
		// the two digests together name the input that differs.
		present, err := artifact.GenerationIDs(s.storeDir)
		if err != nil {
			return nil, err
		}
		s.le.WithFields(identityFields(s.identity)).
			WithField("store", s.storeDir).
			WithField("present", present).
			Warn("no published release artifact matches the current identity")
	}
	results := make([]e2eharness.Generation[releaseArtifact], 0, len(generations))
	for _, generation := range generations {
		results = append(results, e2eharness.Generation[releaseArtifact]{
			Token: generation.ID,
			Artifact: releaseArtifact{
				releaseDir:   generation.ReleaseDir,
				prerenderDir: generation.PrerenderDir,
			},
		})
	}
	return results, nil
}

func (s *releaseShape) Build(ctx context.Context, key string) (e2eharness.Generation[releaseArtifact], error) {
	if err := os.RemoveAll(filepath.Join(s.repoRoot, prerenderDistRelPath)); err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "clean prerender dist")
	}
	if err := os.RemoveAll(filepath.Join(s.repoRoot, ".bldr-dist")); err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "clean release dist state")
	}

	s.le.WithFields(identityFields(s.identity)).Info("building release web bundle")
	if err := buildReleaseWeb(ctx, s.repoRoot); err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "build release web bundle")
	}

	distDir := filepath.Join(s.repoRoot, releaseDistRelPath)
	s.le.Info("building prerender hydrate bundle")
	if err := runBun(ctx, s.repoRoot, "run", "vite", "build", "--config", "app/prerender/vite.hydrate.config.ts"); err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "build prerender hydrate bundle")
	}
	s.le.Info("building prerender ssr bundle")
	if err := runBun(ctx, s.repoRoot, "run", "vite", "build", "--config", "app/prerender/vite.ssr.config.ts"); err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "build prerender ssr bundle")
	}
	s.le.Info("running prerender build")
	if err := runBun(ctx, s.repoRoot, "./app/prerender/ssr-dist/build.js", "--dist-dir", distDir); err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "run prerender build")
	}

	generation, err := artifact.PublishGeneration(
		s.storeDir,
		distDir,
		filepath.Join(s.repoRoot, prerenderDistRelPath),
		s.identity,
	)
	if err != nil {
		return e2eharness.Generation[releaseArtifact]{}, errors.Wrap(err, "publish release artifact")
	}
	s.le.WithField("identity", s.identity.Digest).Info("release artifact rebuilt and published")
	return e2eharness.Generation[releaseArtifact]{
		Token: generation.ID,
		Artifact: releaseArtifact{
			releaseDir:   generation.ReleaseDir,
			prerenderDir: generation.PrerenderDir,
		},
	}, nil
}
