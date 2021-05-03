package forge_target_mock

// TargetYAML is a example yaml config.
const TargetYAML = `
inputs: []
outputs:
  - name: test
    outputType: OutputType_EXEC
    execOutput: "test"
# if exec is empty: no execution pass is performed.
exec:
  # indicates to use controllerbus exec method
  controller:
    # revision: 0 -> defaults to 1
    config:
      exampleField: "Hello world"
    id: controllerbus/example/boilerplate/1
`
