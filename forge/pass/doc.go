// Package forge_pass defines the Pass, one attempt to run a Task's Target
// against one frozen input snapshot.
//
// A Pass copies the Target reference and inputs when it is created, so a change
// never reaches running Executions: the Task cancels the Pass and creates a new
// one, linked to its predecessor with PredPassToPrevious. It links to its
// Executions with PredPassToExecution and keeps an ExecState snapshot of each,
// so it can judge all replicas from one block. Its states:
//
//   - PENDING: not started. Starting it creates the Executions, placed on the
//     Pass's Placement peer when it has one.
//   - RUNNING: Executions run, and their snapshots update as they change.
//   - CHECKING: every Execution is terminal. The Pass computes its outputs, and
//     the replicas must agree: ComputeOutputsWithStates fails the Pass when any
//     replica's outputs differ from another's.
//   - COMPLETE: terminal, in success or failure.
//   - CANCELING: the Pass waits for its Executions to drain, then completes
//     with the cancellation as its Result.
//
// The Pass needs as many ExecStates as its replica count before it can
// complete. WaitPassComplete waits for COMPLETE by watching the World.
package forge_pass
