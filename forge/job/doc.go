// Package forge_job defines the Job, the World object that groups the Tasks of
// one coordinated run.
//
// A Job moves PENDING, RUNNING, COMPLETE. Returning a RUNNING Job to PENDING
// cancels its Passes and replaces them with new ones. It links to its Tasks with
// PredJobToTask and completes when every Task is terminal. CompletionResult
// derives its Result from theirs. Its timestamp is the reference time for its
// Tasks' operations, and its Placement, when set, freezes the Worker and Device
// peer chosen when it was created.
//
// CreateJobWithTasks creates a pending Job with one Task per named Target, and
// WaitJobComplete waits for completion by watching the World. Settle ends a
// retained Job's lifetime and releases its private Git checkouts once every Task
// is terminal. The caller drains the Executions first.
package forge_job
