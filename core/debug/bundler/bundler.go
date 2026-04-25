//go:build !js

package bundler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/bun"
	"github.com/aperturerobotics/util/pipesock"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	singleton_muxed_conn "github.com/s4wave/spacewave/bldr/util/singleton-muxed-conn"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	bldr_web_bundler_vite_compiler "github.com/s4wave/spacewave/bldr/web/bundler/vite/compiler"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// Bundler bundles TypeScript eval scripts using a Vite subprocess.
type Bundler struct {
	le          *logrus.Entry
	distPath    string
	sourcePath  string
	workingPath string

	mu      sync.Mutex
	webPkgs []*bldr_web_bundler.WebPkgRefConfig
	client  bldr_vite.SRPCViteBundlerClient
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewBundler creates a new eval bundler.
//
// distPath is the bldr dist sources directory (.bldr/src/).
// sourcePath is the project root directory.
// workingPath is the working directory for temp files (.bldr/debug/eval/).
func NewBundler(le *logrus.Entry, distPath, sourcePath, workingPath string) *Bundler {
	return &Bundler{
		le:          le,
		distPath:    distPath,
		sourcePath:  sourcePath,
		workingPath: workingPath,
	}
}

// SetWebPkgs configures web packages for externalization.
func (b *Bundler) SetWebPkgs(pkgs []*bldr_web_bundler.WebPkgRefConfig) {
	b.mu.Lock()
	b.webPkgs = pkgs
	b.mu.Unlock()
}

// Bundle bundles a TypeScript file and returns the bundled JS code.
func (b *Bundler) Bundle(ctx context.Context, scriptPath string) (string, error) {
	client, err := b.ensureVite(ctx)
	if err != nil {
		return "", err
	}

	// Make script path absolute then relative to source root.
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return "", errors.Wrap(err, "resolve script path")
	}
	relScript, err := filepath.Rel(b.sourcePath, absScript)
	if err != nil || strings.HasPrefix(relScript, "..") {
		return "", errors.Errorf("script %s is outside project root %s", scriptPath, b.sourcePath)
	}

	// Create output directory.
	outDir := filepath.Join(b.workingPath, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", errors.Wrap(err, "create output dir")
	}

	meta := &bldr_web_bundler_vite_compiler.ViteBundleMeta{
		Id: "default",
		Entrypoints: []*bldr_web_bundler_vite_compiler.ViteBundleEntrypoint{
			{InputPath: relScript},
		},
	}

	b.mu.Lock()
	webPkgs := b.webPkgs
	b.mu.Unlock()

	_, outputMetas, _, err := bldr_web_bundler_vite_compiler.BuildViteBundle(
		ctx,
		b.le,
		b.distPath,
		b.sourcePath,
		b.workingPath,
		nil,
		meta,
		client,
		webPkgs,
		outDir,
		"eval",
		false,
	)
	if err != nil {
		return "", errors.Wrap(err, "vite bundle")
	}

	// Find the JS output for the entrypoint. Vite may emit many chunks, so do
	// not return the first JavaScript file from the output metadata.
	var fallback string
	for _, m := range outputMetas {
		p := m.GetPath()
		if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs") {
			if fallback == "" {
				fallback = p
			}
			if m.GetEntrypointPath() != relScript {
				continue
			}
			outPath := filepath.Join(outDir, p)
			data, err := os.ReadFile(outPath)
			if err != nil {
				return "", errors.Wrapf(err, "read output %s", outPath)
			}
			return rewriteEvalImports(string(data)), nil
		}
	}
	if fallback != "" {
		outPath := filepath.Join(outDir, fallback)
		data, err := os.ReadFile(outPath)
		if err != nil {
			return "", errors.Wrapf(err, "read output %s", outPath)
		}
		return rewriteEvalImports(string(data)), nil
	}

	return "", errors.New("vite build produced no JS output")
}

var evalRelativeImportPattern = regexp.MustCompile(`((?:from\s+|import\s*(?:\(\s*)?)["'])(?:\.\./)+`)

func rewriteEvalImports(code string) string {
	return evalRelativeImportPattern.ReplaceAllString(code, `${1}./`)
}

// ensureVite returns the Vite SRPC client, starting the subprocess if needed.
func (b *Bundler) ensureVite(ctx context.Context) (bldr_vite.SRPCViteBundlerClient, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil {
		return b.client, nil
	}
	return b.startViteLocked(ctx)
}

// startViteLocked starts the Vite bun subprocess.
// Caller must hold b.mu.
func (b *Bundler) startViteLocked(_ context.Context) (bldr_vite.SRPCViteBundlerClient, error) {
	b.le.Debug("starting vite bundler subprocess")

	if err := os.MkdirAll(b.workingPath, 0o755); err != nil {
		return nil, errors.Wrap(err, "create working dir")
	}

	// Derive a deterministic pipe UUID from paths, plus a random suffix.
	var bin [32]byte
	blake3.DeriveKey(
		"alpha eval bundler pipe uuid",
		bytes.Join([][]byte{[]byte(b.sourcePath), []byte(b.workingPath)}, []byte(" -- ")),
		bin[:],
	)
	pipeUuid := "eval-" + strings.ToLower(b58.Encode(bin[:]))[:4] + "-" + randstring.RandomIdentifier(4)

	// Compile vite.ts with esbuild.
	viteScriptPath := filepath.Join(b.workingPath, "bldr-"+pipeUuid+".mjs")
	result := esbuild.Build(esbuild.BuildOptions{
		AbsWorkingDir: b.distPath,
		SourceRoot:    b.workingPath,
		Outfile:       viteScriptPath,
		EntryPoints:   []string{"./web/bundler/vite/vite.ts"},
		Target:        esbuild.ES2022,
		Format:        esbuild.FormatESModule,
		Platform:      esbuild.PlatformNode,
		LogLevel:      esbuild.LogLevelWarning,
		TreeShaking:   esbuild.TreeShakingTrue,
		Sourcemap:     esbuild.SourceMapLinked,
		Drop:          esbuild.DropDebugger,
		Define:        map[string]string{"BLDR_IS_NODE": "true"},
		Plugins:       []esbuild.Plugin{bldr_esbuild_build.ExternalNodeModulesPlugin()},
		External:      []string{"starpc", "vite"},
		Bundle:        true,
		Write:         true,
	})
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
		return nil, errors.Wrap(err, "compile vite.ts")
	}

	// Create pipe listener for IPC.
	pipeListener, err := pipesock.BuildPipeListener(b.le, b.workingPath, pipeUuid)
	if err != nil {
		return nil, errors.Wrap(err, "create pipe listener")
	}

	// Create a long-lived context for the subprocess. Uses context.Background()
	// intentionally: the Vite subprocess persists across multiple Bundle() calls
	// and must outlive any individual caller's context.
	viteCtx, viteCancel := context.WithCancel(context.Background())

	smc := singleton_muxed_conn.NewSingletonMuxedConn(viteCtx, true)
	go smc.AcceptPump(pipeListener)

	// Bun state dir at .bldr/bun (two levels up from .bldr/debug/eval/).
	bunStateDir := filepath.Join(b.workingPath, "..", "..", "bun")

	cmd, err := bun.BunExec(viteCtx, b.le, bunStateDir, viteScriptPath, "--bundle-id", "eval", "--pipe-uuid", pipeUuid)
	if err != nil {
		smc.Close()
		pipeListener.Close()
		viteCancel()
		return nil, errors.Wrap(err, "create bun command")
	}
	cmd.Env = os.Environ()
	cmd.Dir = filepath.Dir(viteScriptPath)
	cmd.Stdout = b.le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = b.le.WriterLevel(logrus.DebugLevel)

	if err := cmd.Start(); err != nil {
		smc.Close()
		pipeListener.Close()
		viteCancel()
		return nil, errors.Wrap(err, "start bun subprocess")
	}

	// Wait for the subprocess to connect via IPC.
	timeoutCtx, timeoutCancel := context.WithTimeout(viteCtx, 30*time.Second)
	defer timeoutCancel()

	b.le.Debug("waiting for vite subprocess to connect")
	_, err = smc.WaitConn(timeoutCtx)
	if err != nil {
		viteCancel()
		_ = cmd.Wait()
		smc.Close()
		pipeListener.Close()
		return nil, errors.Wrap(err, "vite subprocess did not connect")
	}

	client := bldr_vite.NewSRPCViteBundlerClient(srpc.NewClientWithMuxedConn(smc))
	b.le.Debug("vite bundler subprocess connected")

	b.client = client
	b.cancel = viteCancel
	b.done = make(chan struct{})

	// Background goroutine: waits for process exit, clears client.
	go func() {
		defer close(b.done)
		defer pipeListener.Close()
		defer smc.Close()
		_ = cmd.Wait()
		b.mu.Lock()
		if b.client == client {
			b.client = nil
		}
		b.mu.Unlock()
	}()

	return client, nil
}

// Close shuts down the Vite subprocess and waits for cleanup.
func (b *Bundler) Close() {
	b.mu.Lock()
	cancel := b.cancel
	done := b.done
	b.client = nil
	b.cancel = nil
	b.done = nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
