package bundler

import (
	"os"
	"path/filepath"

	"github.com/aperturerobotics/fastjson"
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
)

// defaultManifestName is the manifest to extract webPkgs from.
const defaultManifestName = "spacewave-web"

// ParseBldrWebPkgs reads bldr.yaml and extracts webPkgs from the spacewave-web manifest.
func ParseBldrWebPkgs(projectRoot string) ([]*bldr_web_bundler.WebPkgRefConfig, error) {
	configPath := filepath.Join(projectRoot, "bldr.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "read %s", configPath)
	}

	jdata, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, errors.Wrapf(err, "parse %s", configPath)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(jdata)
	if err != nil {
		return nil, errors.Wrapf(err, "parse %s", configPath)
	}

	webPkgs := v.GetArray("manifests", defaultManifestName, "builder", "config", "webPkgs")
	var pkgs []*bldr_web_bundler.WebPkgRefConfig
	for _, wp := range webPkgs {
		id := string(wp.GetStringBytes("id"))
		if id != "" {
			pkgs = append(pkgs, &bldr_web_bundler.WebPkgRefConfig{Id: id})
		}
	}
	return pkgs, nil
}

// MergeWebPkgStrings converts string package IDs to WebPkgRefConfig and appends them.
func MergeWebPkgStrings(existing []*bldr_web_bundler.WebPkgRefConfig, ids []string) []*bldr_web_bundler.WebPkgRefConfig {
	for _, id := range ids {
		if id != "" {
			existing = append(existing, &bldr_web_bundler.WebPkgRefConfig{Id: id})
		}
	}
	return existing
}
