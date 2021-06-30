# Library

> Core library of targets for Forge.

## Introduction

This is an assortment of controllers for Forge with associated examples.

Note that custom controllers can easily be added by third-party code.

## Example

The following is an example of a Target to create a kv tree with some values.

```yaml
# the inputs / outputs listed in the Target are not used.
inputs:
  # value to set to test-3 key
  - name: testValue
    inputType: InputType_WORLD_OBJECT
    objectKey: "testValue"
outputs:
  # contains the kvtx store modified
  - name: store
    outputType: OutputType_EXEC
    execOutput: "store"
  - name: test-1-value
    outputType: OutputType_EXEC
    execOutput: "test-1-value"
  - name: test-2-existed
    outputType: OutputType_EXEC
    execOutput: "test-2-existed"
exec:
  controller:
    # revision: 0 -> defaults to 1
    config:
      ops:
      - opType: OpType_SET
        ops:
        - key: "test-1"
          valueString: "Hello World"
        - key: "test-2"
          valueString: "Testing 123"
      - key: "test-1"
        opType: OpType_GET
        output: "test-1-value"
      - key: "test-2"
        opType: OpType_GET_EXISTS
        output: "test-2-existed"
      - key: "test-2"
        opType: OpType_CHECK
        valueString: "Testing 123"
      - key: "test-4"
        opType: OpType_SET_BLOB
        valueInput: "testValue"
        output: "test-4-value"
      - opType: OpType_CHECK_EXISTS
        key: "test-2"
      - opType: OpType_DELETE
        key: "test-2"
      - opType: OpType_CHECK_NOT_EXISTS
        key: "test-2"
    id: forge/lib/kvtx/1
```

In this example we use the kvtx controller to perform a series of operations.

## Controllers

The following are implemented in this tree:

 - kvtx: key-value transaction store operations
