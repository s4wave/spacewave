//go:build !js

package releasewasm

import (
	"context"
	"crypto/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/core/sobject"
	world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// prepareLocalCDN builds minified release artifacts incrementally and exports
// a complete localhost CDN before browser navigation.
func prepareLocalCDN(ctx context.Context, le *logrus.Entry, repoRoot, baseURL string) (releaseWasmDistDirs, error) {
	// Keep build state separate from release artifacts and shared development data.
	stateDir := filepath.Join(repoRoot, localCDNState)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return releaseWasmDistDirs{}, err
	}
	conf, distConfig, err := localCDNProject(repoRoot, baseURL)
	if err != nil {
		return releaseWasmDistDirs{}, err
	}
	configData, err := conf.MarshalJSON()
	if err != nil {
		return releaseWasmDistDirs{}, err
	}
	configPath := filepath.Join(localCDNState, "project.json")
	if err := os.WriteFile(filepath.Join(repoRoot, configPath), configData, 0o644); err != nil {
		return releaseWasmDistDirs{}, err
	}

	// Bldr owns source invalidation and rebuilds only changed manifests.
	le.Info("building local CDN startup artifacts with release JavaScript minification")
	args := []string{
		"run", "bldr", "--", "--config=" + configPath, "--state-path=" + localCDNState,
		"--build-type=release", "--js-minification=enable", "--minify-entrypoint=true",
	}
	if err := runBun(ctx, repoRoot, append(args, "build", "-b", "local-startup-plugins,local-startup-web,release-web")...); err != nil {
		return releaseWasmDistDirs{}, errors.Wrap(err, "build local CDN startup")
	}

	// Each publication contains only the current build, so earlier candidates
	// cannot accumulate historical manifests and change later measurements.
	publicationDir := filepath.Join(stateDir, "publication")
	if err := os.RemoveAll(publicationDir); err != nil {
		return releaseWasmDistDirs{}, err
	}
	if err := os.MkdirAll(publicationDir, 0o755); err != nil {
		return releaseWasmDistDirs{}, err
	}
	// Publish sequentially because both selections write the same local World.
	for _, selection := range []string{"spacewave-release", "spacewave-release-web"} {
		if err := runBun(ctx, repoRoot, append(args, "publish", "-p", selection)...); err != nil {
			return releaseWasmDistDirs{}, errors.Wrap(err, "publish local startup World")
		}
	}
	dirs := releaseWasmDistDirs{
		releaseDist: filepath.Join(stateDir, "build", "js", "spacewave-browser", "dist"),
		prerender:   filepath.Join(repoRoot, prerenderDistRelPath),
	}
	if err := exportLocalCDN(ctx, le, stateDir, dirs.releaseDist); err != nil {
		return releaseWasmDistDirs{}, err
	}
	if err := os.WriteFile(filepath.Join(dirs.releaseDist, "distribution.packedmsg"), []byte(distConfig), 0o644); err != nil {
		return releaseWasmDistDirs{}, err
	}

	// Use the same landing page and hydration composition as browser releases.
	for _, config := range []string{"app/prerender/vite.hydrate.config.ts", "app/prerender/vite.ssr.config.ts"} {
		if err := runBun(ctx, repoRoot, "run", "cross-env", "BLDR_STATE_PATH="+stateDir, "vite", "build", "--config", config); err != nil {
			return releaseWasmDistDirs{}, err
		}
	}
	if err := runBun(ctx, repoRoot, "./app/prerender/ssr-dist/build.js", "--dist-dir", dirs.releaseDist); err != nil {
		return releaseWasmDistDirs{}, err
	}
	return dirs, nil
}

// exportLocalCDN writes real release packs and their signed root pointer to
// the static origin. Its temporary signing key never leaves this process.
func exportLocalCDN(ctx context.Context, le *logrus.Entry, stateDir, distDir string) error {
	// Mount only the dedicated fixture database and stage its current manifests.
	w, err := publisher.OpenLocalWorld(ctx, le, filepath.Join(stateDir, "publication", "release.bdb"), "spacewave-release-world", "spacewave-release")
	if err != nil {
		return err
	}
	defer w.Release()
	var metadata *release.ReleaseMetadata
	err = world.ExecTransaction(ctx, w.Engine, true, func(ctx context.Context, ws world.WorldState) error {
		var err error
		metadata, err = publisher.StageChannel(ctx, ws, "spacewave/release/manifests", &release.ReleaseMetadata{
			ProjectId: "spacewave", Version: "0.0.0-local", ChannelKey: "stable", Rev: 1,
		})
		return err
	})
	if err != nil {
		return err
	}

	// The production exporter verifies every referenced block before emission.
	cdnDir := filepath.Join(distDir, "cdn", localCDNSpaceID)
	head, packs, err := publisher.Export(ctx, w.Engine, metadata, localCDNSpaceID,
		func(ctx context.Context, index int, entry *packfile.PackfileEntry, data []byte) error {
			path := filepath.Join(cdnDir, "packs", entry.Id[:2], entry.Id+".kvf")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(path, data, 0o644)
		})
	if err != nil {
		return errors.Wrap(err, "export local CDN packs")
	}

	// Publish the pointer only after all immutable packs are available.
	head = head.CloneVT()
	head.BucketId = ""
	stateData, err := (&world_engine.InnerState{HeadRef: head}).MarshalVT()
	if err != nil {
		return err
	}
	inner, err := (&sobject.SORootInner{Seqno: 1, StateData: stateData}).MarshalVT()
	if err != nil {
		return err
	}
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return err
	}
	root := &sobject.SORoot{Inner: inner, InnerSeqno: 1}
	if err := root.SignInnerData(key, localCDNSpaceID, 1, hash.RecommendedHashType); err != nil {
		return err
	}
	data, err := (&cdn.CdnRootPointer{SpaceId: localCDNSpaceID, Root: root, Packs: packs}).MarshalVT()
	if err != nil {
		return err
	}
	le.WithField("packs", len(packs)).Info("exported local startup CDN")
	return os.WriteFile(filepath.Join(cdnDir, "root.packedmsg"), []byte(packedmsg.EncodePackedMessage(data)), 0o644)
}

// localCDNHandler observes actual fixture delivery, including requests from
// workers that WebKit does not expose through page request events.
type localCDNHandler struct {
	// next serves the fixture's static assets.
	next http.Handler
	// distribution counts delivered distribution requests.
	distribution atomic.Uint64
	// roots counts delivered CDN root requests.
	roots atomic.Uint64
	// packs counts delivered pack requests.
	packs atomic.Uint64
}

// ServeHTTP counts fixture requests and rejects outbound proxy connections.
func (h *localCDNHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect || req.URL.IsAbs() {
		http.Error(rw, "external network is unavailable", http.StatusForbidden)
		return
	}
	switch {
	case strings.HasSuffix(req.URL.Path, "/distribution.packedmsg"):
		h.distribution.Add(1)
	case strings.HasSuffix(req.URL.Path, "/root.packedmsg"):
		h.roots.Add(1)
	case strings.HasSuffix(req.URL.Path, ".kvf"):
		h.packs.Add(1)
	}
	h.next.ServeHTTP(rw, req)
}
