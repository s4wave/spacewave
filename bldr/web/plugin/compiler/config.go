//go:build !js

package bldr_web_plugin_compiler

import (
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	bldr_web_plugin_controller "github.com/s4wave/spacewave/bldr/web/plugin/controller"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig() *Config {
	return &Config{}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	conf, err := c.ToPluginCompilerConf()
	if err != nil {
		return err
	}
	if err := conf.Validate(); err != nil {
		return err
	}
	if electronPkg := c.GetElectronPkg(); electronPkg != "" {
		// split on version
		chk := strings.TrimSpace(electronPkg)
		verIdx := strings.LastIndex(electronPkg, "@")
		if verIdx != -1 && verIdx > 0 {
			chk = electronPkg[:verIdx]
		}
		if err := web_pkg.ValidateWebPkgId(chk); err != nil {
			return errors.Errorf("electron_pkg: invalid web pkg id: %s", chk)
		}
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
}

// ToPluginCompilerConf converts the Config to a PluginCompilerConf.
func (c *Config) ToPluginCompilerConf() (*bldr_plugin_compiler_go.Config, error) {
	pluginCompilerConf := bldr_plugin_compiler_go.NewConfig()
	pluginCompilerConf.ProjectId = c.GetProjectId()
	pluginCompilerConf.GoPkgs = []string{
		basePkg + "/web/plugin/controller",
	}
	pluginCompilerConf.DisableRpcFetch = true
	pluginCompilerConf.DelveAddr = c.GetDelveAddr()

	// configure running the web plugin controller
	// build config set for the plugin
	pluginCompilerConf.ConfigSet = map[string]*configset_proto.ControllerConfig{}
	_, err := configset_proto.
		ConfigSetMap(pluginCompilerConf.ConfigSet).
		ApplyConfig("web-plugin", &bldr_web_plugin_controller.Config{}, 1, false)
	if err != nil {
		return nil, err
	}

	return pluginCompilerConf, nil
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
