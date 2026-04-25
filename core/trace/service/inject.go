//go:build !js

package trace_service

import (
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// InjectTraceConfig adds the trace service controller to all Go compiler
// manifests in the project config so the trace service is available in
// plugin processes without modifying the production project config.
func InjectTraceConfig(projectConfig *bldr_project.ProjectConfig) error {
	for _, manifest := range projectConfig.GetManifests() {
		builder := manifest.GetBuilder()
		if builder == nil || builder.GetId() != bldr_plugin_compiler_go.ConfigID {
			continue
		}

		goConf := &bldr_plugin_compiler_go.Config{}
		if data := builder.GetConfig(); len(data) != 0 {
			if err := goConf.UnmarshalJSON(data); err != nil {
				return err
			}
		}

		goConf.GoPkgs = append(goConf.GoPkgs, "./core/trace/service")
		if goConf.ConfigSet == nil {
			goConf.ConfigSet = make(map[string]*configset_proto.ControllerConfig)
		}
		goConf.ConfigSet["trace-service"] = &configset_proto.ControllerConfig{
			Id:     ConfigID,
			Config: []byte("{}"),
		}

		data, err := goConf.MarshalJSON()
		if err != nil {
			return err
		}
		builder.Config = data
	}
	return nil
}
