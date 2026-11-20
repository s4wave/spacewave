// Package forge_target defines the Target, the immutable description of the
// work a Task runs, and the ExecController contract that performs it.
//
// A Target names its inputs, its outputs and an Exec block that selects a
// controllerbus controller. Tasks, Passes and Executions refer to it by block
// reference, so every replica of a Pass runs the same configuration. An Input
// is an inline Value, an alias of another input, a World, an object in a World,
// or a named output of a completed Task (InputType_TASK_OUTPUT); WatchChanges
// marks an input whose change restarts the Task. An Output is either mounted
// from an exec output or given inline.
//
// An ExecController is a controllerbus controller that also implements
// InitForgeExecController. The Execution resolves the inputs into an InputMap,
// calls InitForgeExecController with it and an ExecControllerHandle, then runs
// Execute. The handle is the controller's only access to the run: storage
// through AccessStorage, named outputs through SetOutputs, log lines, its peer,
// its reference timestamp and the claim epoch it was granted.
package forge_target
