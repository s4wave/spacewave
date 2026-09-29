//go:build !js

package bldr

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

// DistGoMod is the Go module path used for the checked-out Bldr dist sources.
const DistGoMod = "github.com/s4wave/spacewave/bldr-dist"

// distSyncOwnedPaths are the dist root entries that SyncDistSources writes
// itself rather than copying from the embedded dist sources.
var distSyncOwnedPaths = []string{"vendor", "node_modules", "go.mod", "go.sum", ".sync-hash"}

// DistSourceSyncConfig configures Bldr dist-source materialization.
type DistSourceSyncConfig struct {
	// RepoRoot is the project root containing the live go.mod and go.sum.
	RepoRoot string
	// DistRoot is the output directory used as BLDR_DIST_ROOT.
	DistRoot string
	// BldrVersion is the module version to require when BldrSrcPath is empty.
	BldrVersion string
	// BldrSum is the module checksum for BldrVersion.
	BldrSum string
	// BldrSrcPath is an optional replacement path for the source module.
	BldrSrcPath string
}

// SyncDistSources syncs embedded Bldr dist sources into DistRoot and
// materializes the vendor tree used by non-local @go/* TypeScript imports.
func SyncDistSources(ctx context.Context, le *logrus.Entry, conf DistSourceSyncConfig) error {
	// Reject a configuration missing the repository or dist root.
	if conf.RepoRoot == "" {
		return errors.New("repo root is required")
	}
	if conf.DistRoot == "" {
		return errors.New("dist root is required")
	}

	// Build the FS handle over the embedded dist sources.
	distSourcesHandle := BuildDistSourcesFSHandle(ctx, le)
	defer distSourcesHandle.Release()

	// Sync the embedded sources into the dist root, keeping the owned paths.
	if err := os.MkdirAll(conf.DistRoot, 0o755); err != nil {
		return err
	}
	if err := unixfs_sync.Sync(
		ctx,
		conf.DistRoot,
		distSourcesHandle,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		unixfs_sync.NewSkipPathPrefixes(distSyncOwnedPaths),
	); err != nil {
		return err
	}

	// Run each go mod step through a command wired to the debug log.
	runGoMod := func(cmd string) error {
		// Build the go mod command and wire its output to the debug log.
		le.Infof("bldr sources: running go mod %s", cmd)
		goVendorCmd := exec.NewCmd(ctx, "go", "mod", cmd)
		goVendorCmd.Dir = conf.DistRoot
		goModWriter := le.WriterLevel(logrus.DebugLevel)
		goVendorCmd.Stderr = goModWriter
		goVendorCmd.Stdout = goModWriter
		goVendorCmd.Env = os.Environ()

		// Run the command and report the run or writer-close error.
		runErr := goVendorCmd.Run()
		closeErr := goModWriter.Close()
		if runErr != nil {
			return runErr
		}
		return closeErr
	}

	// Parse the repository go.mod and retarget its module path for the dist copy.
	distGoModPath := filepath.Join(conf.DistRoot, "go.mod")
	sourceGoModPath := filepath.Join(conf.RepoRoot, "go.mod")
	sourceGoModData, err := os.ReadFile(sourceGoModPath)
	if err != nil {
		return errors.Wrapf(err, "read repo go.mod at %s", sourceGoModPath)
	}
	distModFile, err := modfile.Parse(sourceGoModPath, sourceGoModData, nil)
	if err != nil {
		return err
	}
	sourceModPath := distModFile.Module.Mod.Path
	distModFile.Module.Mod.Path = DistGoMod

	// Resolve relative replace targets against the repository root.
	if err := absolutizeRelativeReplaces(distModFile, conf.RepoRoot); err != nil {
		return err
	}

	// Declare the dist module and pin the source module by path or version.
	if err := distModFile.AddModuleStmt(DistGoMod); err != nil {
		return err
	}
	if conf.BldrSrcPath != "" {
		if err := distModFile.AddReplace(sourceModPath, "", conf.BldrSrcPath, ""); err != nil {
			return err
		}
	} else {
		if conf.BldrVersion == "" {
			return errors.New("bldr version is required when bldr source path is empty")
		}
		if err := distModFile.AddRequire(sourceModPath, conf.BldrVersion); err != nil {
			return err
		}
	}

	// Clean up and format the dist go.mod.
	distModFile.Cleanup()
	updatedDistGoMod, err := distModFile.Format()
	if err != nil {
		return err
	}

	// Read the repository go.sum to copy into the dist checkout.
	sourceGoSumPath := filepath.Join(conf.RepoRoot, "go.sum")
	sourceGoSumData, err := os.ReadFile(sourceGoSumPath)
	if err != nil {
		return errors.Wrapf(err, "read repo go.sum at %s", sourceGoSumPath)
	}

	// Tidy and vendor read the dist go.mod and go.sum and the imports of the
	// synced dist sources. The vendor tree is reused while all three match.
	hashStr, err := distSyncHash(conf.DistRoot, updatedDistGoMod, sourceGoSumData, conf.BldrSum)
	if err != nil {
		return err
	}
	syncHashPath := filepath.Join(conf.DistRoot, ".sync-hash")
	vendorDir := filepath.Join(conf.DistRoot, "vendor")

	// Skip tidy and vendor when the recorded hash and vendor tree are current.
	existingHash, hashReadErr := os.ReadFile(syncHashPath)
	_, vendorStatErr := os.Stat(vendorDir)
	if hashReadErr == nil && strings.TrimSpace(string(existingHash)) == hashStr && vendorStatErr == nil {
		le.Info("bldr sources: inputs unchanged, skipping tidy+vendor")
		le.Info("done checking out bldr sources")
		return nil
	}

	// #nosec G703 -- this fixed filename is written in the caller-owned dist checkout.
	if err := os.WriteFile(distGoModPath, updatedDistGoMod, 0o644); err != nil {
		return err
	}

	// Write the dist go.sum beside the dist go.mod.
	distGoSumPath := filepath.Join(conf.DistRoot, "go.sum")

	// #nosec G703 -- this fixed filename is written in the caller-owned dist checkout.
	if err := os.WriteFile(distGoSumPath, sourceGoSumData, 0o644); err != nil {
		return err
	}

	// Append the Bldr module checksums when vendoring a pinned version.
	if conf.BldrSum != "" {
		goModSum := sha256.Sum256(sourceGoModData)
		goModInner := hex.EncodeToString(goModSum[:]) + "  go.mod\n"
		goModInnerSum := sha256.Sum256([]byte(goModInner))
		goModSumHash := "h1:" + base64.StdEncoding.EncodeToString(goModInnerSum[:])

		goSumFile, err := os.OpenFile(distGoSumPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err = goSumFile.WriteString(sourceModPath + " " + conf.BldrVersion + " " + conf.BldrSum + "\n"); err != nil {
			_ = goSumFile.Close()
			return err
		}
		if _, err = goSumFile.WriteString(sourceModPath + " " + conf.BldrVersion + "/go.mod " + goModSumHash + "\n"); err != nil {
			_ = goSumFile.Close()
			return err
		}
		if err = goSumFile.Close(); err != nil {
			return err
		}

		if err := runGoMod("download"); err != nil {
			return err
		}
	} else {
		if err := runGoMod("tidy"); err != nil {
			return err
		}
	}

	// Vendor the dependencies into the dist checkout.
	if err := runGoMod("vendor"); err != nil {
		return err
	}

	// #nosec G703 -- this fixed filename is written in the caller-owned dist checkout.
	if err := os.WriteFile(syncHashPath, []byte(hashStr), 0o644); err != nil {
		le.WithError(err).Debug("failed to write bldr sources sync hash")
	}
	le.Info("done checking out bldr sources")
	return nil
}

// distSyncHash digests the inputs of tidy and vendor: the dist go.mod, the repo
// go.sum, the Bldr module checksum and every synced dist source file.
func distSyncHash(distRoot string, distGoMod, goSum []byte, bldrSum string) (string, error) {
	// Digest the fixed inputs: the dist go.mod, the repo go.sum and the Bldr sum.
	h := sha256.New()
	writeField := func(name string, data []byte) {
		_, _ = fmt.Fprintf(h, "%s %d\n", name, len(data))
		_, _ = h.Write(data)
	}
	writeField("go.mod", distGoMod)
	writeField("go.sum", goSum)
	writeField("bldr-sum", []byte(bldrSum))

	// Walk the dist tree and hash every regular file outside the owned paths.
	root, err := os.OpenRoot(distRoot)
	if err != nil {
		return "", err
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		// Bail out on walk errors and skip the root directory entry.
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}

		// Skip the sync-owned metadata files at the dist root.
		if !strings.Contains(p, "/") && slices.Contains(distSyncOwnedPaths, p) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Hash each regular file's contents under its path.
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := root.ReadFile(p)
		if err != nil {
			return err
		}
		writeField(p, data)
		return nil
	})
	if err != nil {
		return "", errors.Wrap(err, "hash dist sources")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func absolutizeRelativeReplaces(modFile *modfile.File, repoRoot string) error {
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	for _, replace := range modFile.Replace {
		if replace.New.Version != "" {
			continue
		}
		replacePath := replace.New.Path
		if filepath.IsAbs(replacePath) {
			continue
		}
		if !strings.HasPrefix(replacePath, ".") {
			continue
		}
		absReplacePath := filepath.Clean(filepath.Join(absRepoRoot, replacePath))
		replace.New.Path = absReplacePath
		if replace.Syntax != nil && len(replace.Syntax.Token) > 0 {
			replace.Syntax.Token[len(replace.Syntax.Token)-1] = absReplacePath
		}
	}
	return nil
}
