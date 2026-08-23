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

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/bun"
	"github.com/aperturerobotics/util/pipesock"
	"github.com/aperturerobotics/util/routine"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	singleton_muxed_conn "github.com/s4wave/spacewave/bldr/util/singleton-muxed-conn"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
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

	mtx     sync.Mutex
	webPkgs []*bldr_web_bundler.WebPkgRefConfig
	client  bldr_vite.SRPCViteBundlerClient
	vite    *routine.RoutineContainer
}

type viteStartResult struct {
	client bldr_vite.SRPCViteBundlerClient
	err    error
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
		vite:        routine.NewRoutineContainerWithLogger(le.WithField("routine", "vite-bundler")),
	}
}

// SetWebPkgs configures web packages for externalization.
func (b *Bundler) SetWebPkgs(pkgs []*bldr_web_bundler.WebPkgRefConfig) {
	b.mtx.Lock()
	b.webPkgs = pkgs
	b.mtx.Unlock()
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

	b.mtx.Lock()
	webPkgs := b.webPkgs
	b.mtx.Unlock()

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
		false,
		true,
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
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.client != nil {
		return b.client, nil
	}
	return b.startViteLocked(ctx)
}

// startViteLocked starts the Vite bun subprocess.
// Caller must hold b.mtx.
func (b *Bundler) startViteLocked(_ context.Context) (bldr_vite.SRPCViteBundlerClient, error) {
	b.le.Debug("starting vite bundler subprocess")
	ready := make(chan viteStartResult, 1)
	b.vite.SetRoutine(func(ctx context.Context) error {
		return b.runVite(ctx, ready)
	})
	// The Vite subprocess is owned by the Bundler and outlives any one Bundle call.
	b.vite.SetContext(context.Background(), true)

	result := <-ready
	if result.err != nil {
		return nil, result.err
	}
	b.client = result.client
	return result.client, nil
}

func (b *Bundler) runVite(viteCtx context.Context, ready chan<- viteStartResult) error {
	if err := os.MkdirAll(b.workingPath, 0o755); err != nil {
		err = errors.Wrap(err, "create working dir")
		ready <- viteStartResult{err: err}
		return err
	}

	// Derive a deterministic pipe UUID from paths, plus a random suffix.
	var bin [32]byte
	blake3.DeriveKey(
		"alpha eval bundler pipe uuid",
		bytes.Join([][]byte{[]byte(b.sourcePath), []byte(b.workingPath)}, []byte(" -- ")),
		bin[:],
	)
	pipeUuid := "eval-" + strings.ToLower(b58.Encode(bin[:]))[:4] + "-" + randstring.RandomIdentifier(4)

	// State remains under .bldr/bun, two levels above the debug working path.
	bunStateDir := filepath.Join(b.workingPath, "..", "..", "bun")
	viteScriptPath := filepath.Join(b.workingPath, "bldr-"+pipeUuid+".mjs")
	if _, err := bldr_vite.BuildServiceScript(
		viteCtx,
		b.le,
		bunStateDir,
		b.sourcePath,
		b.distPath,
		viteScriptPath,
	); err != nil {
		err = errors.Wrap(err, "compile vite.ts")
		ready <- viteStartResult{err: err}
		return err
	}

	// Create pipe listener for IPC.
	pipeListener, err := pipesock.BuildPipeListener(b.le, b.workingPath, pipeUuid)
	if err != nil {
		ready <- viteStartResult{err: errors.Wrap(err, "create pipe listener")}
		return errors.Wrap(err, "create pipe listener")
	}

	smc := singleton_muxed_conn.NewSingletonMuxedConn(viteCtx, true)
	go smc.AcceptPump(pipeListener)

	cmd, err := bun.BunExec(viteCtx, b.le, bunStateDir, viteScriptPath, "--bundle-id", "eval", "--pipe-uuid", pipeUuid)
	if err != nil {
		smc.Close()
		pipeListener.Close()
		ready <- viteStartResult{err: errors.Wrap(err, "create bun command")}
		return errors.Wrap(err, "create bun command")
	}
	cmd.Env = os.Environ()
	cmd.Dir = filepath.Dir(viteScriptPath)
	cmd.Stdout = b.le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = b.le.WriterLevel(logrus.DebugLevel)

	if err := cmd.Start(); err != nil {
		smc.Close()
		pipeListener.Close()
		ready <- viteStartResult{err: errors.Wrap(err, "start bun subprocess")}
		return errors.Wrap(err, "start bun subprocess")
	}

	// Wait for the subprocess to connect via IPC.
	timeoutCtx, timeoutCancel := context.WithTimeout(viteCtx, 30*time.Second)
	defer timeoutCancel()

	b.le.Debug("waiting for vite subprocess to connect")
	_, err = smc.WaitConn(timeoutCtx)
	if err != nil {
		_ = cmd.Wait()
		smc.Close()
		pipeListener.Close()
		ready <- viteStartResult{err: errors.Wrap(err, "vite subprocess did not connect")}
		return errors.Wrap(err, "vite subprocess did not connect")
	}

	client := bldr_vite.NewSRPCViteBundlerClient(srpc.NewClientWithMuxedConn(smc))
	b.le.Debug("vite bundler subprocess connected")

	ready <- viteStartResult{client: client}

	defer pipeListener.Close()
	defer smc.Close()
	_ = cmd.Wait()
	b.mtx.Lock()
	if b.client == client {
		b.client = nil
	}
	b.mtx.Unlock()
	if viteCtx.Err() != nil {
		return context.Canceled
	}
	return nil
}

// Close shuts down the Vite subprocess and waits for cleanup.
func (b *Bundler) Close() {
	b.mtx.Lock()
	b.client = nil
	waitCh, _ := b.vite.SetRoutine(nil)
	b.vite.ClearContext()
	b.mtx.Unlock()

	if waitCh != nil {
		<-waitCh
	}
}
