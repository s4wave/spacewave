# Library

> Core library of targets for Forge.

## Introduction

This is an assortment of controllers for Forge with associated examples.

Note that custom controllers can easily be added by third-party code.

## Controllers

The following are implemented in this tree:

 - [kvtx](./kvtx): key-value transaction store operations
 - [containers](./containers): run pods and containers
 - [git](./git): git repo operations
 - [world](./world): utilities to operate on Hydra Worlds

## Example: Run Kubernetes Pod

The following is an example of running a Kubernetes Pod:

```yaml
inputs: []
outputs: []
exec:
  controller:
    id: forge/lib/containers/pod
    config:
      spec: |
        restartPolicy: OnFailure
        containers:
        - image: docker.io/library/alpine:edge
          name: hello
          command:
          - echo
          - Hello world
          tty: true
```

## Example: Git Clone

The following is an example of cloning a Git repo to a Hydra world:

```yaml
outputs:
  - name: repo
    outputType: OutputType_EXEC
    execOutput: "repo"
exec:
  controller:
    config:
      objectKey: "my-repo"
      cloneOpts:
        url: "https://github.com/pkg/errors"
      worktreeOpts:
        objectKey: "my-worktree"
        workdirRef:
          objectKey: "my-workdir"
        createWorkdir: true
    id: forge/lib/git/clone
```

## Example: KVTX

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
    id: forge/lib/kvtx
```

In this example we use the kvtx controller to perform a series of operations.

