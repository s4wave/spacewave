//go:build !js

package bldr

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

// DistGoMod is the Go module path used for the checked-out Bldr dist sources.
const DistGoMod = "github.com/s4wave/spacewave/bldr-dist"

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
	if conf.RepoRoot == "" {
		return errors.New("repo root is required")
	}
	if conf.DistRoot == "" {
		return errors.New("dist root is required")
	}

	distSourcesHandle := BuildDistSourcesFSHandle(ctx, le)
	defer distSourcesHandle.Release()

	if err := os.MkdirAll(conf.DistRoot, 0o755); err != nil {
		return err
	}
	if err := unixfs_sync.Sync(
		ctx,
		conf.DistRoot,
		distSourcesHandle,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		unixfs_sync.NewSkipPathPrefixes([]string{"vendor", "node_modules", "go.mod", "go.sum", ".sync-hash"}),
	); err != nil {
		return err
	}

	runGoMod := func(cmd string) error {
		le.Infof("bldr sources: running go mod %s", cmd)
		goVendorCmd := exec.NewCmd(ctx, "go", "mod", cmd)
		goVendorCmd.Dir = conf.DistRoot
		goModWriter := le.WriterLevel(logrus.DebugLevel)
		goVendorCmd.Stderr = goModWriter
		goVendorCmd.Stdout = goModWriter
		goVendorCmd.Env = os.Environ()
		runErr := goVendorCmd.Run()
		closeErr := goModWriter.Close()
		if runErr != nil {
			return runErr
		}
		return closeErr
	}

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

	distModFile.Cleanup()
	updatedDistGoMod, err := distModFile.Format()
	if err != nil {
		return err
	}

	goModHash := sha256.Sum256(updatedDistGoMod)
	hashStr := hex.EncodeToString(goModHash[:])
	syncHashPath := filepath.Join(conf.DistRoot, ".sync-hash")
	vendorDir := filepath.Join(conf.DistRoot, "vendor")

	existingHash, hashReadErr := os.ReadFile(syncHashPath)
	_, vendorStatErr := os.Stat(vendorDir)
	if hashReadErr == nil && strings.TrimSpace(string(existingHash)) == hashStr && vendorStatErr == nil {
		le.Info("bldr sources: inputs unchanged, skipping tidy+vendor")
		le.Info("done checking out bldr sources")
		return nil
	}

	if err := os.WriteFile(distGoModPath, updatedDistGoMod, 0o644); err != nil {
		return err
	}

	distGoSumPath := filepath.Join(conf.DistRoot, "go.sum")
	sourceGoSumPath := filepath.Join(conf.RepoRoot, "go.sum")
	sourceGoSumData, err := os.ReadFile(sourceGoSumPath)
	if err != nil {
		return errors.Wrapf(err, "read repo go.sum at %s", sourceGoSumPath)
	}
	if err := os.WriteFile(distGoSumPath, sourceGoSumData, 0o644); err != nil {
		return err
	}

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

	if err := runGoMod("vendor"); err != nil {
		return err
	}

	if err := os.WriteFile(syncHashPath, []byte(hashStr), 0o644); err != nil {
		le.WithError(err).Debug("failed to write bldr sources sync hash")
	}
	le.Info("done checking out bldr sources")
	return nil
}
