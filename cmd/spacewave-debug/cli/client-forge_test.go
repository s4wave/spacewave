//go:build !js

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

func TestParsePluginExecTarget(t *testing.T) {
	target := []byte(`
outputs:
- name: output
  outputType: OutputType_EXEC
  execOutput: output
exec:
  controller:
    id: "space-exec/plugin"
    config:
      pluginId: "glados-core"
      controllerId: "glados/container-runtime/v86/browser"
      controllerConfig: "AQID"
`)
	conf, err := parsePluginExecTarget(target)
	if err != nil {
		t.Fatal(err.Error())
	}
	if conf.GetPluginId() != "glados-core" {
		t.Fatalf("plugin id: %s", conf.GetPluginId())
	}
	if conf.GetControllerId() != "glados/container-runtime/v86/browser" {
		t.Fatalf("controller id: %s", conf.GetControllerId())
	}
	if !bytes.Equal(conf.GetControllerConfig(), []byte{1, 2, 3}) {
		t.Fatalf("controller config: %v", conf.GetControllerConfig())
	}
}

func TestParsePluginExecTargetRejectsInputs(t *testing.T) {
	target := []byte(`
inputs:
- name: workspace
  inputType: InputType_ALIAS
  alias: workspace
exec:
  controller:
    id: "space-exec/plugin"
    config:
      pluginId: "glados-core"
      controllerId: "glados/container-runtime/v86/browser"
      controllerConfig: "AQID"
`)
	_, err := parsePluginExecTarget(target)
	if err == nil || !strings.Contains(err.Error(), "does not support target inputs") {
		t.Fatalf("err: %v", err)
	}
}

func TestPrintPluginExecResponse(t *testing.T) {
	var out bytes.Buffer
	err := printPluginExecResponse(context.Background(), &out, &space_exec.PluginExecResponse{
		Logs: []*space_exec.PluginExecLog{{
			Level:   "info",
			Message: "started",
		}},
		Outputs: []*forge_value.Value{{Name: "status"}},
		OutputFiles: []*space_exec.PluginExecOutputFile{{
			Path: "result.txt",
			Data: []byte("hello"),
		}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, want := range []string{
		"log[info]: started",
		"output: status",
		"output-file: result.txt bytes=5",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}
