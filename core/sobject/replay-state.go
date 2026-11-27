package sobject

// ReplayObserver is a Body that follows where its replay stands. The driver
// calls ObserveReplay after each step that reached the end of its order or
// stopped at a *BodyWait, holding the body as it does for Apply, with the
// state it then shows in the health. A body that refuses writes outside
// caught_up derives that refusal from this state alone, so a write and the
// health agree.
type ReplayObserver interface {
	// ObserveReplay receives the replay state. The caller keeps state
	// unchanged afterwards.
	ObserveReplay(state *SOReplayState)
}

// RootWaiter is a Body whose replayed state may keep roots this device's store
// lacks. The driver shows them in each report, and steps again when they
// change, so the health shows a root once the body finds it missing and drops
// it once a source served it.
type RootWaiter interface {
	// RootWaits returns the roots the state keeps that this device holds no
	// copy of, sorted by name, and a channel closed when they change. The
	// caller keeps the roots unchanged.
	RootWaits() ([]*SORootWait, <-chan struct{})
}

// replayOpening is the replay state before a step reached the end of the
// order.
var replayOpening = &SOReplayState{State: &SOReplayState_Opening{Opening: &SOReplayOpening{}}}

// replayCaughtUp is the replay state at the end of the order the last step
// read.
var replayCaughtUp = &SOReplayState{State: &SOReplayState_CaughtUp{CaughtUp: &SOReplayCaughtUp{}}}
