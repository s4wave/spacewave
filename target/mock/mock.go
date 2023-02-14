package forge_target_mock

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
)

// TargetYAML is an example (mock) target config.
const TargetYAML = `
inputs: []
outputs:
  - name: store
    outputType: OutputType_EXEC
    execOutput: "store"
  - name: testValue
    outputType: OutputType_EXEC
    execOutput: "testValue"
# if exec is empty: no execution pass is performed.
exec:
  # indicates to use controllerbus exec method
  controller:
    id: forge/lib/kvtx
    config:
      ops:
      - opType: OpType_CHECK_NOT_EXISTS
        key: "does-not-exist"
      - opType: OpType_SET
        ops:
        - key: "test-1"
          valueString: "Hello World"
        - key: "test-2"
          valueString: "Testing 123"
      - key: "test-2"
        opType: OpType_GET
        output: "testValue"
      - key: "test-2"
        opType: OpType_CHECK
        valueString: "Testing 123"
      - opType: OpType_DELETE
        key: "test-1"
`

// ParseMockTarget parses the mock target yaml.
func ParseMockTarget() (*target_json.Target, error) {
	return target_json.UnmarshalYAML([]byte(TargetYAML))
}

// ResolveMockTarget resolves the mock target on a bus.
func ResolveMockTarget(ctx context.Context, b bus.Bus) (*forge_target.Target, error) {
	return target_json.ResolveYAML(ctx, b, []byte(TargetYAML))
}
