//go:build !js

package e2e_wasm_session

import (
	"slices"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	bldr_plugin_compiler_go "github.com/s4wave/spacewave/bldr/plugin/compiler/go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
)

// InjectSessionHarnessConfig adds the session harness controller to all Go
// compiler manifests in the project config so the session harness is available
// in plugin processes without modifying the production project config.
func InjectSessionHarnessConfig(projectConfig *bldr_project.ProjectConfig) error {
	webrtcConfBytes, err := (&webrtc.Config{
		SignalingId: "webrtc",
		WebRtc: &webrtc.WebRtcConfig{
			IceServers: []*webrtc.IceServerConfig{
				{Urls: []string{"stun:stun.l.google.com:19302"}},
			},
		},
		AllPeers: true,
	}).MarshalJSON()
	if err != nil {
		return err
	}
	linkSolicitConfBytes, err := (&link_solicit_controller.Config{}).MarshalJSON()
	if err != nil {
		return err
	}
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

		if !slices.Contains(goConf.GoPkgs, "./e2e/wasm/session") {
			goConf.GoPkgs = append(goConf.GoPkgs, "./e2e/wasm/session")
		}
		if !slices.Contains(goConf.GoPkgs, "github.com/s4wave/spacewave/net/transport/webrtc") {
			goConf.GoPkgs = append(goConf.GoPkgs, "github.com/s4wave/spacewave/net/transport/webrtc")
		}
		if !slices.Contains(goConf.GoPkgs, "./e2e/wasm/linksolicit") {
			goConf.GoPkgs = append(goConf.GoPkgs, "./e2e/wasm/linksolicit")
		}
		if goConf.ConfigSet == nil {
			goConf.ConfigSet = make(map[string]*configset_proto.ControllerConfig)
		}
		goConf.ConfigSet["e2e-session-harness"] = &configset_proto.ControllerConfig{
			Id:     ConfigID,
			Config: []byte("{}"),
		}
		goConf.ConfigSet["e2e-session-harness-webrtc"] = &configset_proto.ControllerConfig{
			Id:     webrtc.ConfigID,
			Config: webrtcConfBytes,
		}
		goConf.ConfigSet["e2e-session-harness-link-solicit"] = &configset_proto.ControllerConfig{
			Id:     link_solicit_controller.ConfigID,
			Config: linkSolicitConfBytes,
		}

		data, err := goConf.MarshalJSON()
		if err != nil {
			return err
		}
		builder.Config = data
	}
	return nil
}
