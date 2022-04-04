# Run Worker

This example loads a YAML target a runs the full Forge stack:

 - Cluster: associates the Job with Worker.
 - Job: watches the list of Task.
 - Worker: tracks objects linked to the Worker & starts controllers.
 - Task: manages inputs, outputs, target, and passes.
   - re-start the task when the inputs or target change
 - Pass: an instance of executing the Task with a set of inputs & a target.
   - outputs are taken from Executions as per the Target config
 - Execution: an instance of executing the Pass (a replica) on a Worker.

## Demo: Git

```sh
./run-worker ../targets/03-git.yaml
```

The 03-git example will clone a Git repository to a world object.

Creates objects with type:

 - Repo: the git repository
 - Worktree: info on checkout of Repo to Workdir at path.
 - Workdir: unixfs working directory

## Demo: Podman

```sh
# use system-wide podman
./run-worker --podman-url "unix:///run/podman/podman.sock" ../targets/04-containers-pod.yaml
# use user-specific podman
./run-worker --podman-url "unix:///run/user/$(id -u)/podman/podman.sock" ../targets/04-containers-pod.yaml
```

The 04-containers-pod example will run a example job in a Pod with Podman.

Set the URL to the podman.sock and make sure it is accessible.
