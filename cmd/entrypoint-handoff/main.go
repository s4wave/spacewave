//go:build !js

package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const usageText = "usage: entrypoint-handoff --version X.Y.Z --platforms p1,p2 [--include-browser|--browser-only] --out-dir /path/to/out"

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("entrypoint-handoff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var version string
	var platformsCSV string
	var outDir string
	var reactDev bool
	var skipNotarize bool
	var includeBrowser bool
	var browserOnly bool
	var skipBuild bool
	var skipPackage bool
	var stageBuildInputs bool
	var remoteOnly bool
	var remoteHandoffDir string
	if err := func() error {
		fs.StringVar(&version, "version", "", "release version")
		fs.StringVar(&platformsCSV, "platforms", "", "comma-separated target platforms")
		fs.StringVar(&outDir, "out-dir", "", "path to the staged handoff output dir")
		fs.BoolVar(&reactDev, "react-dev", false, "build browser entrypoint in dev mode")
		fs.BoolVar(&skipNotarize, "skip-notarize", false, "skip Apple notarization during packaging")
		fs.BoolVar(&includeBrowser, "include-browser", false, "include browser staging tree and static-manifest.ts")
		fs.BoolVar(&browserOnly, "browser-only", false, "build only browser staging tree and static-manifest.ts")
		fs.BoolVar(&skipBuild, "skip-build", false, "skip helper and entrypoint builds and package existing artifacts")
		fs.BoolVar(&skipPackage, "skip-package", false, "skip installer packaging")
		fs.BoolVar(&stageBuildInputs, "stage-build-inputs", false, "stage raw dist/helper/icon inputs into out-dir")
		fs.BoolVar(&remoteOnly, "remote-only", false, "build only shared remote entrypoint outputs")
		fs.StringVar(&remoteHandoffDir, "remote-handoff-dir", "", "validated shared remote entrypoint handoff input dir")
		return fs.Parse(args)
	}(); err != nil {
		return errors.Wrap(err, "parse flags")
	}
	if version == "" || outDir == "" {
		return errors.New(usageText)
	}
	if !browserOnly && !remoteOnly && platformsCSV == "" {
		return errors.New(usageText)
	}
	if browserOnly && platformsCSV != "" {
		return errors.New("--browser-only does not accept --platforms")
	}
	if browserOnly && includeBrowser {
		return errors.New("--browser-only and --include-browser cannot be combined")
	}
	if remoteOnly && platformsCSV != "" {
		return errors.New("--remote-only does not accept --platforms")
	}
	if remoteOnly && (browserOnly || includeBrowser || skipBuild || skipPackage || stageBuildInputs) {
		return errors.New("--remote-only cannot be combined with browser/platform packaging flags")
	}

	repoDir, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "resolve repo dir")
	}
	platforms := splitCSV(platformsCSV)
	if !browserOnly && !remoteOnly && len(platforms) == 0 {
		return errors.New("at least one platform is required")
	}

	le := logrus.NewEntry(logrus.StandardLogger())
	le.WithField("platforms", strings.Join(platforms, ",")).
		WithField("include_browser", includeBrowser).
		Info("building entrypoint handoff slice")

	if err := os.RemoveAll(outDir); err != nil {
		return errors.Wrap(err, "clean out dir")
	}
	if remoteOnly {
		if err := runPhase(le, "build-remote-entrypoints", func() error {
			return buildRemoteEntrypoints(ctx, repoDir)
		}); err != nil {
			return err
		}
		if err := runPhase(le, "stage-remote-handoff", func() error {
			return stageRemoteHandoff(ctx, repoDir, outDir, reactDev)
		}); err != nil {
			return err
		}
		return nil
	}
	if remoteHandoffDir != "" {
		if err := runPhase(le, "restore-remote-handoff", func() error {
			return restoreRemoteHandoff(ctx, repoDir, remoteHandoffDir, reactDev)
		}); err != nil {
			return err
		}
	}
	if browserOnly {
		if remoteHandoffDir == "" {
			if err := runPhase(le, "build-remote-entrypoints", func() error {
				return buildRemoteEntrypoints(ctx, repoDir)
			}); err != nil {
				return err
			}
		}
		if err := runPhase(le, "build-browser", func() error {
			return buildBrowser(ctx, repoDir, reactDev)
		}); err != nil {
			return err
		}
		if err := runPhase(le, "stage-browser-outputs", func() error {
			return stageBrowserOutputs(repoDir, outDir)
		}); err != nil {
			return err
		}
		return nil
	}
	for _, rel := range []string{
		filepath.Join(".tmp", "dist"),
		filepath.Join(".tmp", "Spacewave.app"),
		filepath.Join(".tmp", "Spacewave-amd64.zip"),
		filepath.Join(".tmp", "Spacewave-arm64.zip"),
		filepath.Join(".tmp", "Spacewave.AppDir-amd64"),
		filepath.Join(".tmp", "Spacewave.AppDir-arm64"),
		filepath.Join(".tmp", "msix-layout-amd64"),
		filepath.Join(".tmp", "msix-layout-arm64"),
		filepath.Join(".tmp", "winzip-layout-amd64"),
		filepath.Join(".tmp", "winzip-layout-arm64"),
		filepath.Join(".tmp", "macos-helper-plists"),
		"staging",
		filepath.Join("dist", "installers"),
	} {
		if err := os.RemoveAll(filepath.Join(repoDir, rel)); err != nil {
			return errors.Wrap(err, "clean "+rel)
		}
	}

	if err := runPhase(le, "prepare-support-files", func() error {
		return prepareSupportFiles(ctx, repoDir, platforms)
	}); err != nil {
		return err
	}
	if !skipBuild && needsBuilderImage(runtime.GOOS, platforms) {
		if err := runPhase(le, "ensure-builder-image", func() error {
			return runScript(repoDir, filepath.Join("scripts", "release", "ensure-builder-image.sh"))
		}); err != nil {
			return errors.Wrap(err, "ensure builder image")
		}
	}
	if !skipBuild {
		if err := runPhase(le, "build-helpers", func() error {
			return buildHelpers(repoDir, platforms)
		}); err != nil {
			return err
		}
		if err := runPhase(le, "build-entrypoints", func() error {
			return buildEntrypoints(ctx, repoDir, platforms)
		}); err != nil {
			return err
		}
		if err := runPhase(le, "build-cli-entrypoints", func() error {
			return buildCliEntrypoints(ctx, repoDir, platforms)
		}); err != nil {
			return err
		}
		if err := runPhase(le, "sign-macos-cli-entrypoints", func() error {
			return signMacOSCliEntrypoints(ctx, repoDir, platforms)
		}); err != nil {
			return err
		}
	}
	if stageBuildInputs {
		if err := runPhase(le, "stage-build-inputs", func() error {
			return stageBuildInputsTree(repoDir, outDir, platforms)
		}); err != nil {
			return err
		}
	}
	if skipPackage {
		return nil
	}
	if err := runPhase(le, "build-bundles", func() error {
		return buildBundles(repoDir, platforms)
	}); err != nil {
		return err
	}
	if err := runPhase(le, "package-installers", func() error {
		return packageInstallers(repoDir, version, platforms, skipNotarize)
	}); err != nil {
		return err
	}
	if err := runPhase(le, "package-cli-artifacts", func() error {
		return packageCliArtifacts(repoDir, platforms)
	}); err != nil {
		return err
	}
	if err := runPhase(le, "notarize-macos-cli-archives", func() error {
		return notarizeMacOSCliArchives(ctx, repoDir, platforms, skipNotarize)
	}); err != nil {
		return err
	}
	if includeBrowser {
		if err := runPhase(le, "build-browser", func() error {
			return buildBrowser(ctx, repoDir, reactDev)
		}); err != nil {
			return err
		}
	}
	if err := runPhase(le, "stage-outputs", func() error {
		return stageOutputs(repoDir, outDir, includeBrowser)
	}); err != nil {
		return err
	}
	return nil
}

func runPhase(le *logrus.Entry, name string, fn func() error) error {
	start := time.Now()
	le.WithField("phase", name).Info("entrypoint handoff phase started")
	if err := fn(); err != nil {
		le.WithField("phase", name).
			WithField("duration", time.Since(start).String()).
			WithError(err).
			Error("entrypoint handoff phase failed")
		return err
	}
	le.WithField("phase", name).
		WithField("duration", time.Since(start).String()).
		Info("entrypoint handoff phase completed")
	return nil
}

func splitCSV(v string) []string {
	var out []string
	for item := range strings.SplitSeq(v, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func needsBuilderImage(hostGOOS string, platforms []string) bool {
	for _, platform := range platforms {
		goos, _ := splitPlatform(platform)
		if goos == "linux" {
			return true
		}
		if goos == "windows" && hostGOOS != "windows" {
			return true
		}
	}
	return false
}

func splitPlatform(platform string) (string, string) {
	for i := len(platform) - 1; i >= 0; i-- {
		if platform[i] == '-' {
			return platform[:i], platform[i+1:]
		}
	}
	return platform, ""
}

func prepareSupportFiles(ctx context.Context, repoDir string, platforms []string) error {
	if err := runScript(repoDir, filepath.Join("scripts", "release", "gen-desktop.sh")); err != nil {
		return errors.Wrap(err, "gen-desktop")
	}
	iconPath := filepath.Join(repoDir, "web", "images", "spacewave-icon.png")
	needsDarwin := false
	for _, platform := range platforms {
		goos, _ := splitPlatform(platform)
		if goos == "darwin" {
			needsDarwin = true
			break
		}
	}
	if !needsDarwin {
		return generateHostIcons(ctx, repoDir, iconPath)
	}
	if err := runScript(repoDir, filepath.Join("scripts", "release", "gen-icons.sh"), iconPath); err != nil {
		return errors.Wrap(err, "gen-icons")
	}
	return nil
}

func buildHelpers(repoDir string, platforms []string) error {
	needsDarwin := false
	for _, platform := range platforms {
		goos, goarch := splitPlatform(platform)
		if goos == "darwin" {
			needsDarwin = true
			continue
		}
		if err := runScript(repoDir, filepath.Join("scripts", "release", "build-helper.sh"), goos, goarch); err != nil {
			return errors.Wrap(err, "build helper "+platform)
		}
	}
	if !needsDarwin {
		return nil
	}
	if err := runScript(repoDir, filepath.Join("scripts", "release", "build-helper.sh"), "darwin", "arm64"); err != nil {
		return errors.Wrap(err, "build helper darwin")
	}
	return nil
}

func buildEntrypoints(ctx context.Context, repoDir string, platforms []string) error {
	if strings.TrimSpace(os.Getenv("ENTRYPOINT_HANDOFF_REMOTE_RESTORED")) != "1" {
		if err := buildRemoteEntrypoints(ctx, repoDir); err != nil {
			return err
		}
	}
	for _, platform := range platforms {
		goos, goarch := splitPlatform(platform)
		buildID := "release-desktop-" + goos + "-" + goarch
		if err := runBldr(ctx, repoDir, "--build-type=release", "build", "-b", buildID); err != nil {
			return errors.Wrap(err, "run bldr "+platform)
		}

		binName := "spacewave"
		if goos == "windows" {
			binName += ".exe"
		}
		srcBin := filepath.Join(
			repoDir, ".bldr", "build", "desktop", goos, goarch,
			"spacewave-dist", "dist", binName,
		)
		dstDir := filepath.Join(repoDir, ".tmp", "dist", platform)
		dstBin := filepath.Join(dstDir, binName)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return errors.Wrap(err, "mkdir dist dir "+platform)
		}
		if err := copyFile(srcBin, dstBin); err != nil {
			return errors.Wrap(err, "copy dist binary "+platform)
		}
		if err := os.Chmod(dstBin, 0o755); err != nil {
			return errors.Wrap(err, "chmod dist binary "+platform)
		}
	}
	return nil
}

func buildRemoteEntrypoints(ctx context.Context, repoDir string) error {
	if err := runBldr(ctx, repoDir, "--build-type=release", "build", "-b", "release-remote-web"); err != nil {
		return errors.Wrap(err, "run bldr release-remote-web")
	}
	if err := runBldr(ctx, repoDir, "--build-type=release", "build", "-b", "release-remote-js"); err != nil {
		return errors.Wrap(err, "run bldr release-remote-js")
	}
	return nil
}

var remoteHandoffTargets = []string{"release-remote-web", "release-remote-js"}

type remoteHandoffIdentity struct {
	GitSHA             string
	ReleaseEnv         string
	ReactDev           bool
	RemoteTargetNames  []string
	RemoteFileMetadata []remoteHandoffFile
}

type remoteHandoffFile struct {
	Path   string
	SHA256 string
	Size   int64
}

func stageRemoteHandoff(ctx context.Context, repoDir, outDir string, reactDev bool) error {
	identity, err := currentRemoteHandoffIdentity(ctx, reactDev)
	if err != nil {
		return err
	}
	root := filepath.Join(outDir, "root")
	for _, rel := range []string{
		filepath.Join(".bldr", "build", "js", "spacewave-app", "dist"),
		filepath.Join(".bldr", "build", "js", "spacewave-app", "dist-deps"),
		filepath.Join(".bldr", "build", "js", "spacewave-app", "assets"),
		filepath.Join(".bldr", "build", "js", "spacewave-web", "dist"),
		filepath.Join(".bldr", "build", "js", "spacewave-web", "dist-deps"),
		filepath.Join(".bldr", "build", "js", "spacewave-web", "assets"),
		filepath.Join(".bldr", "src", "sdk"),
		filepath.Join(".bldr", "src", "web"),
		".bldr-dist",
	} {
		src := filepath.Join(repoDir, rel)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return errors.Wrap(err, "stat "+rel)
		}
		if err := copyTree(src, filepath.Join(root, rel)); err != nil {
			return errors.Wrap(err, "stage remote "+rel)
		}
	}
	files, err := hashTree(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("remote handoff contains no files")
	}
	identity.RemoteFileMetadata = files
	if err := os.WriteFile(
		filepath.Join(outDir, "remote-manifest.json"),
		marshalRemoteHandoffManifest(identity),
		0o644,
	); err != nil {
		return errors.Wrap(err, "write remote manifest")
	}
	return nil
}

func restoreRemoteHandoff(ctx context.Context, repoDir, handoffDir string, reactDev bool) error {
	expected, err := currentRemoteHandoffIdentity(ctx, reactDev)
	if err != nil {
		return err
	}
	if err := validateRemoteHandoffManifest(handoffDir, expected); err != nil {
		return err
	}
	root := filepath.Join(handoffDir, "root")
	for _, rel := range []string{".bldr", ".bldr-dist"} {
		src := filepath.Join(root, rel)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return errors.Wrap(err, "stat remote "+rel)
		}
		if err := copyTree(src, filepath.Join(repoDir, rel)); err != nil {
			return errors.Wrap(err, "restore remote "+rel)
		}
	}
	return os.Setenv("ENTRYPOINT_HANDOFF_REMOTE_RESTORED", "1")
}

func currentRemoteHandoffIdentity(ctx context.Context, reactDev bool) (remoteHandoffIdentity, error) {
	sha := strings.TrimSpace(os.Getenv("GITHUB_SHA"))
	if sha == "" {
		out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
		if err != nil {
			return remoteHandoffIdentity{}, errors.Wrap(err, "resolve git sha")
		}
		sha = strings.TrimSpace(string(out))
	}
	return remoteHandoffIdentity{
		GitSHA:            sha,
		ReleaseEnv:        strings.TrimSpace(os.Getenv("SPACEWAVE_RELEASE_ENV")),
		ReactDev:          reactDev,
		RemoteTargetNames: append([]string(nil), remoteHandoffTargets...),
	}, nil
}

func validateRemoteHandoffManifest(handoffDir string, expected remoteHandoffIdentity) error {
	data, err := os.ReadFile(filepath.Join(handoffDir, "remote-manifest.json"))
	if err != nil {
		return errors.Wrap(err, "read remote manifest")
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(data)
	if err != nil {
		return errors.Wrap(err, "parse remote manifest")
	}
	if got := string(v.GetStringBytes("format")); got != "entrypoint-remote-handoff.v1" {
		return errors.Errorf("remote manifest format mismatch: %s", got)
	}
	if got := string(v.GetStringBytes("git_sha")); got != expected.GitSHA {
		return errors.Errorf("remote manifest git sha mismatch: %s", got)
	}
	if got := string(v.GetStringBytes("release_environment")); got != expected.ReleaseEnv {
		return errors.Errorf("remote manifest release environment mismatch: %s", got)
	}
	if got := v.GetBool("react_dev"); got != expected.ReactDev {
		return errors.Errorf("remote manifest react_dev mismatch: %v", got)
	}
	targets := v.GetArray("remote_targets")
	if len(targets) != len(expected.RemoteTargetNames) {
		return errors.New("remote manifest target count mismatch")
	}
	for i, target := range targets {
		if got := string(target.GetStringBytes()); got != expected.RemoteTargetNames[i] {
			return errors.Errorf("remote manifest target mismatch: %s", got)
		}
	}
	files := v.GetArray("files")
	if len(files) == 0 {
		return errors.New("remote manifest has no files")
	}
	for _, file := range files {
		rel := string(file.GetStringBytes("path"))
		if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
			return errors.Errorf("remote manifest has invalid path: %s", rel)
		}
		path := filepath.Join(handoffDir, "root", filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			return errors.Wrap(err, "stat remote file "+rel)
		}
		if info.Size() != file.GetInt64("size") {
			return errors.Errorf("remote file size mismatch: %s", rel)
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if digest != string(file.GetStringBytes("sha256")) {
			return errors.Errorf("remote file sha256 mismatch: %s", rel)
		}
	}
	return nil
}

func hashTree(root string) ([]remoteHandoffFile, error) {
	var out []remoteHandoffFile
	if err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return errors.Wrap(err, "walk "+path)
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return errors.Wrap(err, "rel remote file")
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return err
		}
		size := info.Size()
		if info.Mode()&os.ModeSymlink != 0 {
			targetInfo, err := os.Stat(path)
			if err != nil {
				return errors.Wrap(err, "stat remote symlink target "+path)
			}
			size = targetInfo.Size()
		}
		out = append(out, remoteHandoffFile{
			Path:   filepath.ToSlash(rel),
			SHA256: digest,
			Size:   size,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "open "+path)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", errors.Wrap(err, "hash "+path)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func marshalRemoteHandoffManifest(identity remoteHandoffIdentity) []byte {
	var b strings.Builder
	b.WriteString("{\n")
	writeJSONField(&b, "format", "entrypoint-remote-handoff.v1", true)
	writeJSONField(&b, "git_sha", identity.GitSHA, true)
	writeJSONField(&b, "release_environment", identity.ReleaseEnv, true)
	b.WriteString("  \"react_dev\": ")
	if identity.ReactDev {
		b.WriteString("true,\n")
	} else {
		b.WriteString("false,\n")
	}
	b.WriteString("  \"remote_targets\": [\n")
	for i, target := range identity.RemoteTargetNames {
		b.WriteString("    ")
		b.WriteString(strconv.Quote(target))
		if i+1 != len(identity.RemoteTargetNames) {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ],\n")
	b.WriteString("  \"files\": [\n")
	for i, file := range identity.RemoteFileMetadata {
		b.WriteString("    {\"path\": ")
		b.WriteString(strconv.Quote(file.Path))
		b.WriteString(", \"sha256\": ")
		b.WriteString(strconv.Quote(file.SHA256))
		b.WriteString(", \"size\": ")
		b.WriteString(strconv.FormatInt(file.Size, 10))
		b.WriteByte('}')
		if i+1 != len(identity.RemoteFileMetadata) {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	return []byte(b.String())
}

func writeJSONField(b *strings.Builder, key, value string, comma bool) {
	b.WriteString("  ")
	b.WriteString(strconv.Quote(key))
	b.WriteString(": ")
	b.WriteString(strconv.Quote(value))
	if comma {
		b.WriteByte(',')
	}
	b.WriteByte('\n')
}

// buildCliEntrypoints cross-compiles the standalone spacewave-cli binary
// for each requested platform and stages the result at
// .tmp/dist-cli/<platform>/spacewave[.exe]. The archive payload uses the
// public command name even though the internal build target is spacewave-cli.
func buildCliEntrypoints(ctx context.Context, repoDir string, platforms []string) error {
	for _, platform := range platforms {
		goos, goarch := splitPlatform(platform)
		buildID := "release-cli-" + goos + "-" + goarch
		if err := runBldr(ctx, repoDir, "--build-type=release", "build", "-b", buildID); err != nil {
			return errors.Wrap(err, "run bldr "+platform)
		}

		srcBinName := "spacewave-cli"
		dstBinName := "spacewave"
		if goos == "windows" {
			srcBinName += ".exe"
			dstBinName += ".exe"
		}
		srcBin := filepath.Join(
			repoDir, ".bldr", "build", "desktop", goos, goarch,
			"spacewave-cli", "dist", srcBinName,
		)
		dstDir := filepath.Join(repoDir, ".tmp", "dist-cli", platform)
		dstBin := filepath.Join(dstDir, dstBinName)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return errors.Wrap(err, "mkdir cli dist dir "+platform)
		}
		if err := copyFile(srcBin, dstBin); err != nil {
			return errors.Wrap(err, "copy cli binary "+platform)
		}
		if err := os.Chmod(dstBin, 0o755); err != nil {
			return errors.Wrap(err, "chmod cli binary "+platform)
		}
	}
	return nil
}

// signMacOSCliEntrypoints applies the same Developer ID signing identity used
// for the desktop .app to standalone macOS CLI binaries before they are
// archived. Windows CLI binaries are signed in the GitHub workflow with Azure
// Trusted Signing after the staged build-input artifact is downloaded.
func signMacOSCliEntrypoints(ctx context.Context, repoDir string, platforms []string) error {
	signingID := strings.TrimSpace(os.Getenv("BLDR_MACOS_SIGN_IDENTITY"))
	for _, platform := range platforms {
		goos, _ := splitPlatform(platform)
		if goos != "darwin" {
			continue
		}
		if signingID == "" {
			return errors.New("BLDR_MACOS_SIGN_IDENTITY is required to sign macOS CLI artifacts")
		}
		binPath := filepath.Join(repoDir, ".tmp", "dist-cli", platform, "spacewave")
		cmd := exec.CommandContext(
			ctx,
			"codesign",
			"--force",
			"--sign", signingID,
			"--options", "runtime",
			binPath,
		)
		cmd.Dir = repoDir
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "codesign cli "+platform)
		}
		verify := exec.CommandContext(ctx, "codesign", "--verify", "--strict", binPath)
		verify.Dir = repoDir
		verify.Env = os.Environ()
		verify.Stdout = os.Stdout
		verify.Stderr = os.Stderr
		if err := verify.Run(); err != nil {
			return errors.Wrap(err, "verify cli signature "+platform)
		}
	}
	return nil
}

func buildBundles(repoDir string, platforms []string) error {
	bundlesDir := filepath.Join(repoDir, ".tmp", "dist", "bundles")
	if err := os.RemoveAll(bundlesDir); err != nil {
		return errors.Wrap(err, "clean bundles dir")
	}
	if err := os.MkdirAll(bundlesDir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir bundles dir")
	}

	for _, platform := range platforms {
		goos, _ := splitPlatform(platform)
		srcDir := filepath.Join(repoDir, ".tmp", "dist", platform)
		archivePath := filepath.Join(bundlesDir, archiveName(goos, platform))
		if goos == "windows" {
			if _, err := archiveDir(srcDir, archivePath, archiveZip); err != nil {
				return errors.Wrap(err, "zip "+platform)
			}
			continue
		}
		if _, err := archiveDir(srcDir, archivePath, archiveTarGz); err != nil {
			return errors.Wrap(err, "tar.gz "+platform)
		}
	}
	return nil
}

func generateHostIcons(ctx context.Context, repoDir, iconPath string) error {
	f, err := os.Open(iconPath)
	if err != nil {
		return errors.Wrap(err, "open icon source")
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return errors.Wrap(err, "decode icon source")
	}

	iconsDir := filepath.Join(repoDir, ".tmp", "icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir icons dir")
	}
	for _, size := range []int{16, 32, 48, 64, 128, 256, 512} {
		dst := resizeImage(src, size)
		outPath := filepath.Join(iconsDir, "icon-"+itoa(size)+".png")
		outFile, err := os.Create(outPath)
		if err != nil {
			return errors.Wrap(err, "create "+outPath)
		}
		if err := png.Encode(outFile, dst); err != nil {
			_ = outFile.Close()
			return errors.Wrap(err, "encode "+outPath)
		}
		if err := outFile.Close(); err != nil {
			return errors.Wrap(err, "close "+outPath)
		}
	}
	icoInputs := []string{
		filepath.Join(iconsDir, "icon-16.png"),
		filepath.Join(iconsDir, "icon-32.png"),
		filepath.Join(iconsDir, "icon-48.png"),
		filepath.Join(iconsDir, "icon-64.png"),
		filepath.Join(iconsDir, "icon-128.png"),
		filepath.Join(iconsDir, "icon-256.png"),
	}
	outFile, err := os.Create(filepath.Join(iconsDir, "icon.ico"))
	if err != nil {
		return errors.Wrap(err, "create icon.ico")
	}
	defer outFile.Close()

	cmd := exec.CommandContext(ctx, "bun", append([]string{"x", "png-to-ico"}, icoInputs...)...)
	cmd.Dir = repoDir
	cmd.Env = os.Environ()
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "png-to-ico")
	}
	return nil
}

func resizeImage(src image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	bounds := src.Bounds()
	sw := bounds.Dx()
	sh := bounds.Dy()
	for y := range size {
		sy := bounds.Min.Y + ((2*y+1)*sh)/(2*size)
		for x := range size {
			sx := bounds.Min.X + ((2*x+1)*sw)/(2*size)
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func archiveName(goos, platform string) string {
	if goos == "windows" {
		return "spacewave-" + platform + ".zip"
	}
	return "spacewave-" + platform + ".tar.gz"
}

// cliArchiveName returns the per-platform CLI archive filename. macOS and
// Windows targets get zip archives; Linux targets get tar.gz. macOS uses the
// user-facing "macos" label rather than the Go GOOS "darwin" so the filename
// matches the URLs declared in app/download/manifest.ts and the existing
// installer naming (spacewave-macos-*.dmg).
func cliArchiveName(goos, platform string) string {
	label := userFacingPlatformLabel(goos, platform)
	if goos == "darwin" || goos == "windows" {
		return "spacewave-cli-" + label + ".zip"
	}
	return "spacewave-cli-" + label + ".tar.gz"
}

// userFacingPlatformLabel returns the human-facing platform label for a
// given goos/platform tuple. macOS gets "macos" everywhere it appears
// in user-visible filenames; other platforms keep their Go GOOS name.
func userFacingPlatformLabel(goos, platform string) string {
	if goos != "darwin" {
		return platform
	}
	_, goarch := splitPlatform(platform)
	return "macos-" + goarch
}

// packageCliArtifacts archives each per-platform CLI binary staged by
// buildCliEntrypoints into the named archive expected by the public
// /download manifest. Output lands in dist/cli/.
func packageCliArtifacts(repoDir string, platforms []string) error {
	cliDir := filepath.Join(repoDir, "dist", "cli")
	if err := os.RemoveAll(cliDir); err != nil {
		return errors.Wrap(err, "clean cli dir")
	}
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		return errors.Wrap(err, "mkdir cli dir")
	}

	for _, platform := range platforms {
		goos, _ := splitPlatform(platform)
		srcDir := filepath.Join(repoDir, ".tmp", "dist-cli", platform)
		archivePath := filepath.Join(cliDir, cliArchiveName(goos, platform))
		if goos == "darwin" || goos == "windows" {
			if _, err := archiveDir(srcDir, archivePath, archiveZip); err != nil {
				return errors.Wrap(err, "zip cli "+platform)
			}
			continue
		}
		if _, err := archiveDir(srcDir, archivePath, archiveTarGz); err != nil {
			return errors.Wrap(err, "tar.gz cli "+platform)
		}
	}
	return nil
}

// notarizeMacOSCliArchives submits the signed macOS CLI zip archives to Apple
// notarization. Plain command-line tools cannot be stapled like .app/.dmg/.pkg
// payloads, so the distributable zip is the notarized artifact.
func notarizeMacOSCliArchives(ctx context.Context, repoDir string, platforms []string, skipNotarize bool) error {
	if skipNotarize {
		return nil
	}
	profile := strings.TrimSpace(os.Getenv("BLDR_MACOS_NOTARIZE_PROFILE"))
	if profile == "" {
		profile = "spacewave-notarize"
	}
	for _, platform := range platforms {
		goos, _ := splitPlatform(platform)
		if goos != "darwin" {
			continue
		}
		archivePath := filepath.Join(repoDir, "dist", "cli", cliArchiveName(goos, platform))
		cmd := exec.CommandContext(
			ctx,
			"xcrun",
			"notarytool",
			"submit",
			archivePath,
			"--keychain-profile", profile,
			"--wait",
		)
		cmd.Dir = repoDir
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "notarize cli "+platform)
		}
	}
	return nil
}

func packageInstallers(repoDir, version string, platforms []string, skipNotarize bool) error {
	for _, platform := range platforms {
		goos, goarch := splitPlatform(platform)
		switch goos {
		case "darwin":
			args := []string{filepath.Join("scripts", "release", "build-macos.sh"), goarch, version}
			if skipNotarize {
				args = append(args, "--skip-notarize")
			}
			if err := runScript(repoDir, args[0], args[1:]...); err != nil {
				return errors.Wrap(err, "build-macos "+platform)
			}
		case "windows":
			if err := runScript(repoDir, filepath.Join("scripts", "release", "build-msix.sh"), goarch, version); err != nil {
				return errors.Wrap(err, "build-msix "+platform)
			}
			if err := runScript(repoDir, filepath.Join("scripts", "release", "build-winzip.sh"), goarch, version); err != nil {
				return errors.Wrap(err, "build-winzip "+platform)
			}
		case "linux":
			if err := runScript(repoDir, filepath.Join("scripts", "release", "build-appimage.sh"), goarch); err != nil {
				return errors.Wrap(err, "build-appimage "+platform)
			}
		default:
			return errors.New("unknown platform " + platform)
		}
	}
	return nil
}

func buildBrowser(ctx context.Context, repoDir string, reactDev bool) error {
	for _, rel := range []string{".bldr-dist", filepath.Join("app", "prerender", "dist")} {
		if err := os.RemoveAll(filepath.Join(repoDir, rel)); err != nil {
			return errors.Wrap(err, "clean "+rel)
		}
	}

	buildScript := "build:release:web"
	if reactDev {
		buildScript = "build:release:web:debug"
	}
	if err := runBun(ctx, repoDir, "run", buildScript); err != nil {
		return errors.Wrap(err, "build release web")
	}
	if err := runBun(ctx, repoDir, "run", "vite", "build", "--config", "app/prerender/vite.hydrate.config.ts"); err != nil {
		return errors.Wrap(err, "build hydrate bundle")
	}
	if err := runBun(ctx, repoDir, "run", "vite", "build", "--config", "app/prerender/vite.ssr.config.ts"); err != nil {
		return errors.Wrap(err, "build prerender bundle")
	}

	bldrDistDir := filepath.Join(repoDir, ".bldr-dist", "build", "js", "spacewave-dist", "dist")
	if err := runBun(ctx, repoDir, "./app/prerender/ssr-dist/build.js", "--dist-dir", bldrDistDir); err != nil {
		return errors.Wrap(err, "prerender")
	}

	stagingDir := filepath.Join(repoDir, "staging")
	if err := os.RemoveAll(stagingDir); err != nil {
		return errors.Wrap(err, "clean staging")
	}
	if err := stageWebDist(bldrDistDir, stagingDir); err != nil {
		return errors.Wrap(err, "stage web dist")
	}
	if err := stageStaticHTML(filepath.Join(repoDir, "app", "prerender", "dist"), stagingDir); err != nil {
		return errors.Wrap(err, "stage static html")
	}
	return nil
}

func stageOutputs(repoDir, outDir string, includeBrowser bool) error {
	installersOut := filepath.Join(outDir, "installers")
	bundlesOut := filepath.Join(outDir, "bundles")
	cliOut := filepath.Join(outDir, "cli")
	if err := os.MkdirAll(installersOut, 0o755); err != nil {
		return errors.Wrap(err, "mkdir installers out")
	}
	if err := os.MkdirAll(bundlesOut, 0o755); err != nil {
		return errors.Wrap(err, "mkdir bundles out")
	}
	if err := os.MkdirAll(cliOut, 0o755); err != nil {
		return errors.Wrap(err, "mkdir cli out")
	}
	if err := copyDirContents(filepath.Join(repoDir, "dist", "installers"), installersOut); err != nil {
		return errors.Wrap(err, "stage installers")
	}
	if err := copyDirContents(filepath.Join(repoDir, ".tmp", "dist", "bundles"), bundlesOut); err != nil {
		return errors.Wrap(err, "stage bundles")
	}
	cliDir := filepath.Join(repoDir, "dist", "cli")
	if _, err := os.Stat(cliDir); err == nil {
		if err := copyDirContents(cliDir, cliOut); err != nil {
			return errors.Wrap(err, "stage cli")
		}
	}
	if !includeBrowser {
		return nil
	}
	return stageBrowserOutputs(repoDir, outDir)
}

func stageBrowserOutputs(repoDir, outDir string) error {
	if err := copyTree(filepath.Join(repoDir, "staging"), filepath.Join(outDir, "browser-staging")); err != nil {
		return errors.Wrap(err, "stage browser tree")
	}
	if err := copyFile(
		filepath.Join(repoDir, "app", "prerender", "dist", "static-manifest.ts"),
		filepath.Join(outDir, "static-manifest.ts"),
	); err != nil {
		return errors.Wrap(err, "stage static-manifest.ts")
	}
	return nil
}

func stageBuildInputsTree(repoDir, outDir string, platforms []string) error {
	for _, platform := range platforms {
		if err := copyTree(
			filepath.Join(repoDir, ".tmp", "dist", platform),
			filepath.Join(outDir, ".tmp", "dist", platform),
		); err != nil {
			return errors.Wrap(err, "stage dist "+platform)
		}
		if err := copyTree(
			filepath.Join(repoDir, ".tmp", "dist-cli", platform),
			filepath.Join(outDir, ".tmp", "dist-cli", platform),
		); err != nil {
			return errors.Wrap(err, "stage dist-cli "+platform)
		}
		if err := copyTree(
			filepath.Join(repoDir, "dist", "helper", platform),
			filepath.Join(outDir, "dist", "helper", platform),
		); err != nil {
			return errors.Wrap(err, "stage helper "+platform)
		}
	}
	if err := copyTree(
		filepath.Join(repoDir, ".tmp", "icons"),
		filepath.Join(outDir, ".tmp", "icons"),
	); err != nil {
		return errors.Wrap(err, "stage icons")
	}
	return nil
}

func runScript(repoDir, script string, args ...string) error {
	cmdArgs := append([]string{script}, args...)
	logCommandStart("script", append([]string{"bash"}, cmdArgs...))
	start := time.Now()
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Dir = repoDir
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	logCommandFinish("script", append([]string{"bash"}, cmdArgs...), start, err)
	return err
}

func runBldr(ctx context.Context, repoDir string, args ...string) error {
	cmdArgs := append([]string{"run", "github.com/s4wave/spacewave/bldr/cmd/bldr"}, args...)
	logCommandStart("bldr", append([]string{"go"}, cmdArgs...))
	start := time.Now()
	cmd := exec.CommandContext(ctx, "go", cmdArgs...)
	cmd.Dir = repoDir
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	logCommandFinish("bldr", append([]string{"go"}, cmdArgs...), start, err)
	return err
}

func runBun(ctx context.Context, repoDir string, args ...string) error {
	logCommandStart("bun", append([]string{"bun"}, args...))
	start := time.Now()
	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Dir = repoDir
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	logCommandFinish("bun", append([]string{"bun"}, args...), start, err)
	return err
}

func logCommandStart(kind string, args []string) {
	logrus.WithField("kind", kind).
		WithField("cmd", strings.Join(args, " ")).
		Info("entrypoint handoff command started")
}

func logCommandFinish(kind string, args []string, start time.Time, err error) {
	ent := logrus.WithField("kind", kind).
		WithField("cmd", strings.Join(args, " ")).
		WithField("duration", time.Since(start).String())
	if err != nil {
		ent.WithError(err).Error("entrypoint handoff command failed")
		return
	}
	ent.Info("entrypoint handoff command completed")
}

func itoa(v int) string { return strconv.Itoa(v) }

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "open "+src)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return errors.Wrap(err, "mkdir "+filepath.Dir(dst))
	}
	out, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "create "+dst)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return errors.Wrap(err, "copy "+src)
	}
	return out.Close()
}

func copyDirContents(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return errors.Wrap(err, "read "+srcDir)
	}
	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			if err := copyTree(src, dst); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(srcRoot, dstRoot string) error {
	return filepath.Walk(srcRoot, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return errors.Wrap(err, "walk "+path)
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return errors.Wrap(err, "rel path")
		}
		if rel == "." {
			return os.MkdirAll(dstRoot, 0o755)
		}
		dst := filepath.Join(dstRoot, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return errors.Wrap(err, "readlink "+path)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return errors.Wrap(err, "mkdir "+filepath.Dir(dst))
			}
			if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
				return errors.Wrap(err, "remove "+dst)
			}
			if err := os.Symlink(target, dst); err != nil {
				return errors.Wrap(err, "symlink "+dst)
			}
			return nil
		}
		if err := copyFile(path, dst); err != nil {
			return err
		}
		return os.Chmod(dst, info.Mode())
	})
}

// archiveFormat identifies the archive format produced by archiveDir.
type archiveFormat int

const (
	archiveTarGz archiveFormat = iota
	archiveZip
)

// archiveDir streams srcDir into destPath in the requested format and
// returns the hex-encoded sha256 of the archive bytes. Both tar.gz and
// zip share the same walk + per-entry contract; only the writer
// construction and entry-header layout differ.
func archiveDir(srcDir, destPath string, format archiveFormat) (string, error) {
	outFile, err := os.Create(destPath)
	if err != nil {
		return "", errors.Wrap(err, "create output")
	}
	defer outFile.Close()

	hash := sha256.New()
	mw := io.MultiWriter(outFile, hash)

	var writeEntry func(relPath string, info fs.FileInfo, path string) error
	var closeArchive func() error

	switch format {
	case archiveTarGz:
		gw := gzip.NewWriter(mw)
		tw := tar.NewWriter(gw)
		writeEntry = func(relPath string, info fs.FileInfo, path string) error {
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return errors.Wrap(err, "file info header")
			}
			header.Name = relPath
			if info.IsDir() {
				header.Name += "/"
			}
			if err := tw.WriteHeader(header); err != nil {
				return errors.Wrap(err, "write header")
			}
			if info.IsDir() {
				return nil
			}
			return copyFileTo(tw, path, relPath)
		}
		closeArchive = func() error {
			if err := tw.Close(); err != nil {
				return errors.Wrap(err, "close tar")
			}
			if err := gw.Close(); err != nil {
				return errors.Wrap(err, "close gzip")
			}
			return nil
		}
	case archiveZip:
		zw := zip.NewWriter(mw)
		writeEntry = func(relPath string, info fs.FileInfo, path string) error {
			if info.IsDir() {
				_, err := zw.Create(relPath + "/")
				return err
			}
			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return errors.Wrap(err, "file info header")
			}
			header.Name = relPath
			header.Method = zip.Deflate
			w, err := zw.CreateHeader(header)
			if err != nil {
				return errors.Wrap(err, "create header")
			}
			return copyFileTo(w, path, relPath)
		}
		closeArchive = func() error {
			if err := zw.Close(); err != nil {
				return errors.Wrap(err, "close zip")
			}
			return nil
		}
	default:
		return "", errors.Errorf("unsupported archive format: %d", format)
	}

	err = filepath.Walk(srcDir, func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		return writeEntry(filepath.ToSlash(relPath), info, path)
	})
	if err != nil {
		return "", errors.Wrap(err, "walk")
	}
	if err := closeArchive(); err != nil {
		return "", err
	}
	if err := outFile.Close(); err != nil {
		return "", errors.Wrap(err, "close file")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// copyFileTo opens path and copies its bytes into w with wrapped error context.
func copyFileTo(w io.Writer, path, relPath string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "open "+relPath)
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return errors.Wrap(err, "copy "+relPath)
	}
	return nil
}

func stageWebDist(distDir, stagingDir string) error {
	return filepath.WalkDir(distDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return errors.Wrap(err, "walk "+path)
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(distDir, path)
		if err != nil {
			return errors.Wrap(err, "rel path")
		}
		relPath = filepath.ToSlash(relPath)

		destPath := filepath.Join(stagingDir, "app", relPath)
		if strings.HasSuffix(d.Name(), ".packedmsg") {
			destPath = filepath.Join(stagingDir, "dist", d.Name())
		}
		return copyFile(path, destPath)
	})
}

func stageStaticHTML(prerenderDir, stagingDir string) error {
	return filepath.WalkDir(prerenderDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return errors.Wrap(err, "walk "+path)
		}
		if d.IsDir() {
			return nil
		}

		switch filepath.Ext(d.Name()) {
		case ".html", ".css", ".js", ".woff2", ".png", ".svg", ".ico":
		default:
			return nil
		}

		relPath, err := filepath.Rel(prerenderDir, path)
		if err != nil {
			return errors.Wrap(err, "rel path")
		}
		return copyFile(path, filepath.Join(stagingDir, "static", relPath))
	})
}
