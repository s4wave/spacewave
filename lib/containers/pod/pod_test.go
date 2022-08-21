//go:build podman_test
// +build podman_test

package forge_lib_containers_pod

import (
	"context"
	"os"
	"path"
	"strconv"
	"testing"

	containers_client "github.com/aperturerobotics/containers/podman/client"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	"github.com/aperturerobotics/timestamp"
)

// podCommandNotFoundYAML tests a "command not found" situation.
const podCommandNotFoundYAML = `
exec:
  controller:
    id: forge/lib/containers/pod/1
    config:
      engineId: podman/client
      name: test-pod
      pod:
        spec: |
          containers:
          - image: docker.io/library/alpine:edge
            name: hello
            command:
            - thisdoesnotexist
`

// podSuccessYAML tests a successful pod.
const podSuccessYAML = `
exec:
  controller:
    id: forge/lib/containers/pod/1
    config:
      engineId: podman/client
      meta: |
        generateName: gen-name-pod
      pod:
        spec: |
          restartPolicy: OnFailure
          containers:
          - image: docker.io/library/alpine:edge
            name: hello
            args:
            - echo
            - "Hello world"
            tty: true
`

// TestPodmanPod tests the containers pod controller.
func TestPodmanPod(t *testing.T) {
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}

	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(containers_client.NewFactory(tb.Bus))

	podmanPath := "/run/podman/podman.sock"
	if euid := os.Geteuid(); euid > 0 {
		podmanPath = path.Join("/run/user", strconv.Itoa(euid), "podman/podman.sock")
	}

	tb.Logger.Infof("using podman path: %s", podmanPath)
	podmanURL := "unix://" + podmanPath

	ctx := tb.Context
	containersID := "podman/client"
	_, clientRef, err := containers_client.StartControllerWithConfig(ctx, tb.Bus, &containers_client.Config{
		EngineId: containersID,
		Url:      podmanURL,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer clientRef.Release()

	buildTgt := func(testYAML string) *forge_target.Target {
		tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(testYAML))
		if err != nil {
			t.Fatal(err.Error())
		}
		return tgt
	}

	// ordinarily resolved by Task controller, set it manually
	valueSet := &forge_target.ValueSet{}
	// handle := forge_target.ExecControllerHandleWithAccess(ws.AccessWorldState)
	ts := timestamp.Now()

	const (
		podCommandNotFoundID = "pod-command-not-found"
		podSuccessID         = "pod-success"
	)
	taskMap := map[string]*forge_target.Target{
		podCommandNotFoundID: buildTgt(podCommandNotFoundYAML),
		podSuccessID:         buildTgt(podSuccessYAML),
	}
	jobKey := "job/1"
	clusterKey := "cluster/1"
	finalState, err := tb.RunWorkerWithTasks(taskMap, valueSet, 1, &ts, jobKey, clusterKey)
	// finalState, err := tb.RunExecutionWithTarget(tgt, valueSet, &ts)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = finalState

	/*
		outputs := forge_value.ValueSlice(finalState.GetValueSet().GetOutputs())
		valMap, err := outputs.BuildValueMap(true, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		_ = valMap
	*/
}
