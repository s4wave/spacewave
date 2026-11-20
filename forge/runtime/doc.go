// Package forge_runtime admits Forge Worker capacity on a Device and tracks the
// reservations that Executions hold against it.
//
// A Device advertises a forge-worker capability whose availability
// LookupDeviceAvailability reads and WatchDeviceAvailability watches. Before a
// Worker runs an Execution, admission reserves capacity on the Device. A
// reservation is:
//
//   - Reserved: capacity is debited and no runtime has claimed it.
//   - Active: one claimed runtime generation holds it.
//   - Uncertain: the outcome is unknown, as after a Device disconnect.
//   - PendingStop: the runtime is fenced and its stop is not yet confirmed.
//   - Released: cleanup is recorded and no capacity is held.
//
// Capacity stays debited until the same fenced runtime reconnects, its stop is
// confirmed, or the lease expires (DefaultLeaseDuration for a reservation,
// DefaultOwnerLeaseDuration for the owner's claim), so a lost Device neither
// leaks capacity forever nor lets it be spent twice.
package forge_runtime
