//go:build !js

package web_pkg_vite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/bun"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	bldr_exec "github.com/s4wave/spacewave/bldr/util/exec"
	bldr_pipesock "github.com/s4wave/spacewave/bldr/util/pipesock"
	singleton_muxed_conn "github.com/s4wave/spacewave/bldr/util/singleton-muxed-conn"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// RunOneShot starts a ViteBundler process, calls the provided function with the
// client, and tears down the process when done. Use this for call sites that do
// not have a long-lived ViteBundler process (esbuild compiler, plugin compiler,
// browser entrypoint).
func RunOneShot(
	ctx context.Context,
	le *logrus.Entry,
	distSourcePath string,
	sourcePath string,
	workingPath string,
	fn func(ctx context.Context, client bldr_vite.SRPCViteBundlerClient) error,
) error {
	// Name the one-shot Vite bundle.
	bundleID := "web-pkg-oneshot"

	// Derive a pipe UUID for IPC.
	var pipeUuidBin [32]byte
	blake3.DeriveKey(
		"bldr vite-compiler pipe uuid",
		bytes.Join([][]byte{[]byte(sourcePath), []byte(workingPath), []byte(bundleID)},
			[]byte(" -- "),
		),
		pipeUuidBin[:],
	)
	pipeUuid := "vite-" + strings.ToLower(b58.Encode(pipeUuidBin[:]))[:4] + "-" + randstring.RandomIdentifier(4)

	// Compile the Vite service script through the direct internal owner.
	if err := os.MkdirAll(workingPath, 0o755); err != nil {
		return err
	}
	bunStateDir := filepath.Join(workingPath, "..", "..", "bun")
	viteScriptPath := filepath.Join(workingPath, "bldr-"+pipeUuid+".mjs")
	if _, err := bldr_vite.BuildServiceScript(
		ctx,
		le,
		bunStateDir,
		sourcePath,
		distSourcePath,
		viteScriptPath,
	); err != nil {
		return errors.Wrap(err, "compile Vite service script")
	}
	defer os.Remove(viteScriptPath)
	defer os.Remove(viteScriptPath + ".map")

	// Set up the IPC pipe.
	pipeListener, err := bldr_pipesock.Listen(le, workingPath, pipeUuid)
	if err != nil {
		return err
	}
	defer pipeListener.Close()

	// Pump accepted connections through the singleton muxed connection.
	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, true)
	go smc.AcceptPump(pipeListener)
	defer smc.Close()

	// Start the bun process.
	cmd, err := bun.BunExec(ctx, le, bunStateDir, viteScriptPath, "--bundle-id", bundleID, "--pipe-uuid", pipeUuid, "--pipe-root", pipeListener.GetRootDir())
	if err != nil {
		return err
	}
	cmd.Env = slices.Clone(os.Environ())
	cmd.Dir = filepath.Dir(viteScriptPath)
	cmd.Stdout = bldr_exec.NewLogWriter(le, logrus.DebugLevel)
	cmd.Stderr = bldr_exec.NewLogWriter(le, logrus.DebugLevel)

	// Configure the color and root environment variables.
	cmd.Env = append(
		cmd.Env,
		"NO_COLOR=1",
		"NODE_DISABLE_COLORS=1",
		"FORCE_COLOR=0",
		"BLDR_PROJECT_ROOT="+sourcePath,
		"BLDR_DIST_ROOT="+distSourcePath,
	)

	// Stop before starting when the context is already done.
	if ctx.Err() != nil {
		return context.Canceled
	}

	// A script outside the repository cannot see the project node_modules.
	cleanupNodeModules, err := linkProjectNodeModules(filepath.Dir(viteScriptPath), sourcePath, distSourcePath)
	if err != nil {
		return err
	}
	defer cleanupNodeModules()

	// Start the bun process.
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start vite process")
	}

	// Wait for vite to connect, killing the process on timeout.
	timeoutCtx, timeoutCancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.New("timeout waiting for vite to connect"))
	defer timeoutCancel()

	// Log the wait and block until the Vite process connects.
	le.Debug("waiting for vite oneshot to connect")
	_, err = smc.WaitConn(timeoutCtx)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	// Create the budgeted SRPC client over the muxed connection.
	srpcClient := srpc.NewClientWithMuxedConn(smc)
	client, err := bldr_vite.NewBudgetClient(bldr_vite.NewSRPCViteBundlerClient(srpcClient))
	if err != nil {
		// Failed admission setup still closes the process started above.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	// Report the connected Vite process.
	le.Debug("vite oneshot connected")

	// Run the caller's function with the budgeted Vite client.
	fnErr := fn(ctx, client)

	// Tear down the process.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	return fnErr
}

// linkProjectNodeModules exposes project node_modules to a script directory
// that cannot walk to one, and returns cleanup for any symlink it creates.
// Bun resolves imports from the script directory, and a script under a temp dir
// otherwise loads packages from the global cache, where dependencies may be
// absent.
func linkProjectNodeModules(scriptDir, sourcePath, distSourcePath string) (func(), error) {
	// Replace an old one-shot symlink so relocated projects use their current dependencies.
	link := filepath.Join(scriptDir, "node_modules")
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return func() {}, nil
		}
		if err := os.Remove(link); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Leave a script that can already see a parent node_modules tree alone.
	if projectNodeModules(scriptDir) != "" {
		return func() {}, nil
	}

	// Prefer the project tree, then the distribution tree.
	modules := projectNodeModules(sourcePath)
	if modules == "" {
		modules = projectNodeModules(distSourcePath)
	}
	if modules == "" {
		return func() {}, nil
	}
	if err := os.Symlink(modules, link); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(link) }, nil
}

// projectNodeModules returns the node_modules directory visible from dir.
func projectNodeModules(dir string) string {
	// An empty start has no tree to walk.
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)

	// Stop at the first ancestor that contains node_modules.
	for {
		candidate := filepath.Join(dir, "node_modules")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
