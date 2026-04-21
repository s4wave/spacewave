//go:build !js

package bldr_project_controller

import (
	js_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/sirupsen/logrus"
)

// resolveWebPkgDeps computes the webPkg dependency graph from manifest configs.
//
// Returns a map of consumer manifest ID -> provider manifest IDs.
// A consumer is a manifest with excluded webPkgs. A provider is a
// manifest that declares the same webPkg ID without exclude.
func resolveWebPkgDeps(le *logrus.Entry, manifests map[string]*bldr_project.ManifestConfig) map[string][]string {
	// providers maps webPkg ID -> manifest ID that provides it.
	providers := make(map[string]string)
	// consumers maps manifest ID -> list of excluded webPkg IDs.
	consumers := make(map[string][]string)

	for manifestID, manifestConf := range manifests {
		builder := manifestConf.GetBuilder()
		if builder.GetId() != js_compiler.ConfigID {
			continue
		}

		conf := &js_compiler.Config{}
		configData := builder.GetConfig()
		if len(configData) == 0 {
			continue
		}
		// Config data may be JSON (from bldr.yaml) or protobuf binary.
		var err error
		if len(configData) > 0 && configData[0] == '{' {
			err = conf.UnmarshalJSON(configData)
		} else {
			err = conf.UnmarshalVT(configData)
		}
		if err != nil {
			le.WithError(err).WithField("manifest-id", manifestID).
				Warn("failed to unmarshal JS compiler config for webPkg dep resolution")
			continue
		}

		for _, webPkg := range conf.GetWebPkgs() {
			pkgID := webPkg.GetId()
			if pkgID == "" {
				continue
			}
			if webPkg.GetExclude() {
				consumers[manifestID] = append(consumers[manifestID], pkgID)
			} else {
				providers[pkgID] = manifestID
			}
		}
	}

	// Resolve consumers to provider manifest IDs.
	result := make(map[string][]string)
	for consumerID, excludedPkgs := range consumers {
		seen := make(map[string]struct{})
		for _, pkgID := range excludedPkgs {
			providerID, ok := providers[pkgID]
			if !ok || providerID == consumerID {
				continue
			}
			if _, dup := seen[providerID]; dup {
				continue
			}
			seen[providerID] = struct{}{}
			result[consumerID] = append(result[consumerID], providerID)
		}
	}

	return result
}
