// Package forge_task defines the Task, the state machine that runs one Target
// against its current inputs.
//
// A Task links to its Target with PredTaskToTarget and to each Pass it creates
// with PredTaskToPass; PredTaskToSubtask and PredTaskToCached link child Tasks
// and a previous Task whose result it inherits. Its states:
//
//   - PENDING: waiting for the Target and inputs to resolve. Once they do, the
//     Task creates a Pass, keyed by its pass_nonce, and moves to RUNNING.
//   - RUNNING: a Pass is executing. A Target or input change cancels the Pass
//     and the Task returns to PENDING.
//   - CHECKING: the Pass completed and the Task checks its result.
//   - COMPLETE: done for the current Target and inputs, in success or failure.
//     A Target or watched input change returns it to PENDING; an explicitly
//     canceled Task stays stopped until retried.
//   - RETRY: waits for an input change before trying a new Pass.
//
// RetireTask ends worker demand for a one-shot Task and its attempts while
// keeping their bodies, results and history, marking each with
// PredObjectRetired so the intent survives a restart. The controller
// subpackage runs the state machine; tx holds its World operations.
package forge_task
