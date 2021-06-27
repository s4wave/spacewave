package forge_kvtx

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	execution_mock "github.com/aperturerobotics/forge/execution/mock"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/sirupsen/logrus"
)

// TestKvtx tests the kvtx execution controller
func TestKvtx(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	testYAML := `
inputs: []
outputs:
  # contains the kvtx store modified
  - name: store
    outputType: OutputType_EXEC
    execOutput: "store"
exec:
  controller:
    # revision: 0 -> defaults to 1
    config:
      ops:
      - opType: OpType_SET
        key: "test-1"
        valueString: "Hello World"
    id: forge/lib/kvtx/1
`

	tgt := &target_json.Target{}
	err := tgt.UnmarshalYAML([]byte(testYAML))
	if err != nil {
		t.Fatal(err.Error())
	}
	err = execution_mock.RunTargetInTestbed(ctx, le, tgt, func(b bus.Bus, sr *static.Resolver) {
		sr.AddFactory(NewFactory(b))
	})
	if err != nil {
		t.Fatal(err.Error())
	}

}
