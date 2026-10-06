//go:build !js

package releasewasm

import (
	"context"
	"crypto/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
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
//
// Like the production CDN, the root keeps the previous publication's packs
// live, so a returning visitor can still read the release it cached.
func exportLocalCDN(ctx context.Context, le *logrus.Entry, stateDir, distDir string) error {
	// Mount only the dedicated fixture database and stage its current manifests.
	w, err := publisher.OpenLocalWorld(ctx, le, filepath.Join(stateDir, "publication", "release.s4wave"), "spacewave-release-world", "spacewave-release")
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
	// Packs live outside the dist directory, which each build replaces.
	packsDir := filepath.Join(stateDir, "cdn-packs")
	if err := os.MkdirAll(packsDir, 0o755); err != nil {
		return err
	}
	head, packs, err := publisher.Export(ctx, w.Engine, metadata, localCDNSpaceID,
		func(ctx context.Context, index int, entry *packfile.PackfileEntry, data []byte) error {
			return os.WriteFile(filepath.Join(packsDir, entry.Id+".kvf"), data, 0o644)
		})
	if err != nil {
		return errors.Wrap(err, "export local CDN packs")
	}
	live, err := retainPreviousPacks(packsDir, packs)
	if err != nil {
		return err
	}

	// Link every live pack into the static origin.
	cdnDir := filepath.Join(distDir, "cdn", localCDNSpaceID)
	for _, entry := range live {
		path := filepath.Join(cdnDir, "packs", entry.Id[:2], entry.Id+".kvf")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Link(filepath.Join(packsDir, entry.Id+".kvf"), path); err != nil {
			return err
		}
	}

	// Publish the pointer only after all immutable packs are available.
	stateData, err := cdn_publish.EncodeHeadState(head)
	if err != nil {
		return err
	}
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return err
	}

	// Sign a genesis checkpoint of the head under the key's owner config and
	// write the pointer. The CDN head is public, so the state stays plaintext.
	genesis, _, err := sobject.BuildGenesisSOState(le, nil, localCDNSpaceID, key, nil)
	if err != nil {
		return err
	}
	checkpoint, err := sobject.BuildGenesisSOCheckpoint(key, localCDNSpaceID, genesis.GetConfig().GetConfigChainHash(), stateData)
	if err != nil {
		return err
	}
	data, err := (&cdn.CdnRootPointer{SpaceId: localCDNSpaceID, Checkpoint: checkpoint, Packs: live}).MarshalVT()
	if err != nil {
		return err
	}
	le.WithField("packs", len(live)).Info("exported local startup CDN")
	return os.WriteFile(filepath.Join(cdnDir, "root.packedmsg"), []byte(packedmsg.EncodePackedMessage(data)), 0o644)
}

// retainPreviousPacks returns the current packs followed by the previous
// publication's packs, records the current packs as the next previous set,
// and deletes every other pack file in packsDir.
func retainPreviousPacks(packsDir string, current []*packfile.PackfileEntry) ([]*packfile.PackfileEntry, error) {
	// The previous set is stored as a pack list in a root pointer message.
	prevPath := filepath.Join(packsDir, "previous.pb")
	prev := &cdn.CdnRootPointer{}
	prevData, err := os.ReadFile(prevPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := prev.UnmarshalVT(prevData); err != nil {
		return nil, err
	}

	// Keep each pack once, preferring the current entry.
	live := slices.Clone(current)
	ids := make(map[string]struct{}, len(current)+len(prev.GetPacks()))
	for _, entry := range current {
		ids[entry.GetId()] = struct{}{}
	}
	for _, entry := range prev.GetPacks() {
		if _, ok := ids[entry.GetId()]; !ok {
			ids[entry.GetId()] = struct{}{}
			live = append(live, entry)
		}
	}

	// Record the current set, then delete packs neither set references.
	nextData, err := (&cdn.CdnRootPointer{Packs: current}).MarshalVT()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(prevPath, nextData, 0o644); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(packsDir, "*.kvf"))
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if _, ok := ids[strings.TrimSuffix(filepath.Base(file), ".kvf")]; !ok {
			if err := os.Remove(file); err != nil {
				return nil, err
			}
		}
	}
	return live, nil
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
	// packBytes counts delivered pack body bytes.
	packBytes atomic.Int64
	// cdnDown drops every CDN connection, as when the CDN is unreachable.
	cdnDown atomic.Bool
}

// ServeHTTP counts fixture requests and rejects outbound proxy connections.
func (h *localCDNHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect || req.URL.IsAbs() {
		http.Error(rw, "external network is unavailable", http.StatusForbidden)
		return
	}
	if h.cdnDown.Load() && strings.Contains(req.URL.Path, "/cdn/") {
		if conn, _, err := http.NewResponseController(rw).Hijack(); err == nil {
			_ = conn.Close()
		}
		return
	}
	switch {
	case strings.HasSuffix(req.URL.Path, "/distribution.packedmsg"):
		h.distribution.Add(1)
	case strings.HasSuffix(req.URL.Path, "/root.packedmsg"):
		h.roots.Add(1)
	case strings.HasSuffix(req.URL.Path, ".kvf"):
		h.packs.Add(1)
		rw = &countingResponseWriter{ResponseWriter: rw, n: &h.packBytes}
	}
	h.next.ServeHTTP(rw, req)
}

// countingResponseWriter adds each delivered body byte to n.
type countingResponseWriter struct {
	http.ResponseWriter
	n *atomic.Int64
}

// Write delivers p and counts the bytes written.
func (w *countingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.n.Add(int64(n))
	return n, err
}
