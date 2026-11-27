// Package forge_value defines the Value, the named and typed reference that
// carries data through Forge, and the Result of a finished object.
//
// A Value holds a block reference, a cross-bucket reference, or a
// WorldObjectSnapshot that pins an object's key, root reference, revision, type
// and parent. A ValueSlice keeps Values unique and sorted by name. A Result
// records success, a failure message or cancellation for a Job, Task, Pass or
// Execution. Values carry the data and Results the outcome.
package forge_value
