//go:build !js

package bldr_project_controller

import (
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	go_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	js_compiler "github.com/s4wave/spacewave/bldr/plugin/compiler/js"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
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
		for _, webPkg := range readCompilerWebPkgs(le, manifestID, manifestConf.GetBuilder()) {
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

// readCompilerWebPkgs reads the web package refs declared by a compiler
// config, returning nil when the compiler is unknown or has none.
func readCompilerWebPkgs(le *logrus.Entry, manifestID string, builder *configset_proto.ControllerConfig) []*bldr_web_bundler.WebPkgRefConfig {
	switch builder.GetId() {
	case js_compiler.ConfigID:
		conf := &js_compiler.Config{}
		if unmarshalBuilderConfig(le, manifestID, "JS", builder.GetConfig(), conf) != nil {
			return nil
		}
		return conf.GetWebPkgs()
	case go_compiler.ConfigID:
		conf := &go_compiler.Config{}
		if unmarshalBuilderConfig(le, manifestID, "Go", builder.GetConfig(), conf) != nil {
			return nil
		}
		return conf.GetWebPkgs()
	default:
		return nil
	}
}

// compilerConfig is implemented by compiler configs that support both
// JSON and binary VT decoding.
type compilerConfig interface {
	UnmarshalJSON([]byte) error
	UnmarshalVT([]byte) error
}

// unmarshalBuilderConfig decodes builder config bytes as JSON or binary
// VT based on the leading byte; an empty payload is a no-op. Failures are
// logged, not returned: dep resolution is best effort.
func unmarshalBuilderConfig(le *logrus.Entry, manifestID, compilerName string, configData []byte, conf compilerConfig) error {
	if len(configData) == 0 {
		return nil
	}
	var err error
	if configData[0] == '{' {
		err = conf.UnmarshalJSON(configData)
	} else {
		err = conf.UnmarshalVT(configData)
	}
	if err != nil {
		le.WithError(err).WithField("manifest-id", manifestID).
			Warnf("failed to unmarshal %s compiler config for webPkg dep resolution", compilerName)
	}
	return err
}
