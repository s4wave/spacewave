//go:build !js

package s4wave_core_e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/sirupsen/logrus"
)

// LoadProjectConfig loads the E2E project config and merges the configs.
func LoadProjectConfig(bldrRootPath string) (*bldr_project.ProjectConfig, error) {
	rootProjectConfig, err := loadRootProjectConfig(bldrRootPath)
	if err != nil {
		return nil, err
	}

	// load the E2E overlay config from the test dir
	testProjectConfigData, err := os.ReadFile(filepath.Join(bldrRootPath, "core/e2e/bldr.yaml"))
	if err != nil {
		return nil, err
	}
	testProjectConfig := &bldr_project.ProjectConfig{}
	err = bldr_project.UnmarshalProjectConfig(testProjectConfigData, testProjectConfig)
	if err != nil {
		return nil, err
	}

	// merge test project config into root project config
	projectConfig := rootProjectConfig.CloneVT()
	err = bldr_project.MergeProjectConfigs(projectConfig, testProjectConfig)
	if err != nil {
		return nil, err
	}

	// override the start config
	projectConfig.Start.Plugins = []string{"spacewave-core", "spacewave-e2e", "spacewave-web", "spacewave-app"}

	// done
	return projectConfig, nil
}

func loadRootProjectConfig(bldrRootPath string) (*bldr_project.ProjectConfig, error) {
	yamlPath := filepath.Join(bldrRootPath, "bldr.yaml")
	starPath := filepath.Join(bldrRootPath, "bldr.star")

	rootProjectConfig := &bldr_project.ProjectConfig{}
	yamlData, yamlErr := os.ReadFile(yamlPath)
	_, starErr := os.Stat(starPath)
	if yamlErr != nil && starErr != nil {
		return nil, errors.Wrap(yamlErr, "read bldr.yaml")
	}

	if yamlErr == nil {
		yamlConfig := &bldr_project.ProjectConfig{}
		if err := bldr_project.UnmarshalProjectConfig(yamlData, yamlConfig); err != nil {
			return nil, errors.Wrap(err, "unmarshal bldr.yaml")
		}
		if err := mergeExtendedConfigs(rootProjectConfig, bldrRootPath, yamlConfig.GetExtends()); err != nil {
			return nil, err
		}
		if err := bldr_project.MergeProjectConfigs(rootProjectConfig, yamlConfig); err != nil {
			return nil, errors.Wrap(err, "merge bldr.yaml config")
		}
	}

	if starErr == nil {
		result, err := bldr_project_starlark.Evaluate(starPath)
		if err != nil {
			return nil, errors.Wrap(err, "evaluate bldr.star")
		}
		if err := mergeExtendedConfigs(rootProjectConfig, bldrRootPath, result.Config.GetExtends()); err != nil {
			return nil, err
		}
		result.Config.Extends = nil
		if err := bldr_project.MergeProjectConfigs(rootProjectConfig, result.Config); err != nil {
			return nil, errors.Wrap(err, "merge bldr.star config")
		}
	}

	return rootProjectConfig, nil
}

func mergeExtendedConfigs(
	projectConfig *bldr_project.ProjectConfig,
	bldrRootPath string,
	modulePaths []string,
) error {
	for _, modulePath := range modulePaths {
		extConfig, _, err := bldr_project.LoadExtendedProjectConfig(bldrRootPath, modulePath)
		if err != nil {
			return errors.Wrapf(err, "extends %s", modulePath)
		}
		if err := bldr_project.MergeProjectConfigs(projectConfig, extConfig); err != nil {
			return errors.Wrapf(err, "merge extends %s", modulePath)
		}
	}
	return nil
}

// CheckoutWebDistSources checks out the web dist sources to the specified directory.
func CheckoutWebDistSources(ctx context.Context, le *logrus.Entry, distDir string) error {
	distSourcesHandle := bldr.BuildDistSourcesFSHandle(ctx, le)
	defer distSourcesHandle.Release()

	// sync the entrypoint sources to the path
	err := os.MkdirAll(distDir, 0o755)
	if err != nil {
		return err
	}
	err = unixfs_sync.Sync(
		ctx,
		distDir,
		distSourcesHandle,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		unixfs_sync.NewSkipPathPrefixes([]string{"vendor", "node_modules"}),
	)
	if err != nil {
		return err
	}

	// patch tsconfig.json to use host project vendor/
	tsconfigPath := filepath.Join(distDir, "tsconfig.json")
	tsconfigData, err := os.ReadFile(tsconfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Replace the @go/* path mapping
	updatedConfig := strings.Replace(string(tsconfigData), `"@go/*": ["./vendor/*"]`, `"@go/*": ["../../../../vendor/*"]`, 1)

	err = os.WriteFile(tsconfigPath, []byte(updatedConfig), 0o644)
	if err != nil {
		return err
	}

	return nil
}
