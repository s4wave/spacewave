// Package forge is the root of Forge, which runs Jobs of Tasks on Workers with
// all of its state stored as World objects.
//
// A Job (forge/job) groups Tasks. A Task (forge/task) binds one Target
// (forge/target) to its resolved inputs and runs it as a sequence of Passes. A
// Pass (forge/pass) freezes one Target and input snapshot and runs the Target
// as one or more replica Executions (forge/execution), each on one assigned
// peer. A Cluster (forge/cluster) schedules Jobs onto the Workers
// (forge/worker) linked to it, and forge/runtime admits Worker capacity on a
// Device. Values (forge/value) carry data between them. Every object is a
// block in the World, every relationship is a graph quad with a forge/
// predicate, and every state change is a typed World operation, so any peer
// that can read the World can watch a Job advance.
//
// Every object carries a creation timestamp that Executions use as their
// reference time, so the same Target over the same inputs produces the same
// outputs on every peer.
package forge
