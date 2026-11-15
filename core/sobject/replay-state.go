package sobject

import "sync"

// ReplayObserver is a Body that follows where its replay stands. The driver
// calls ObserveReplay on each move of the state it shows in the health: after
// a step that reached the end of its order or stopped at a *BodyWait, and when
// a state arrives that holds another peer's operation the body has not
// applied. Calls are serialized, but the second kind comes while a step may
// hold the body. A body that refuses writes outside caught_up derives that
// refusal from this state alone, so a write and the health agree.
type ReplayObserver interface {
	// ObserveReplay receives the replay state. The caller keeps state
	// unchanged afterwards.
	ObserveReplay(state *SOReplayState)
}

// replayOpening is the replay state before a step reached the end of the
// order.
var replayOpening = &SOReplayState{State: &SOReplayState_Opening{Opening: &SOReplayOpening{}}}

// replayCaughtUp is the replay state at the end of the order the last step
// read.
var replayCaughtUp = &SOReplayState{State: &SOReplayState_CaughtUp{CaughtUp: &SOReplayCaughtUp{}}}

// replayCatchingUp is the replay state while another peer's operation is held
// and not yet applied.
var replayCatchingUp = &SOReplayState{State: &SOReplayState_CatchingUp{CatchingUp: &SOReplayCatchingUp{}}}

// replayMachine is where replay stands, derived from what the last step
// reached and from the states that arrived since. Each state the driver
// watches gets the next generation as it arrives. held is the latest
// generation whose state holds another peer's operation above the highest
// position of that peer any earlier state held, and applied is the latest
// generation a step brought the body to the end of. While the last step
// caught up and held is above applied, the machine is catching up.
//
// Its lock is apart from the driver's hold of the body, so a state that
// arrives during a long step moves it at once. It tells the body and the
// health under that lock, so both see its moves in order.
type replayMachine struct {
	// mtx guards the fields below and orders the calls to observe and send.
	mtx sync.Mutex
	// stepped is what the last step reached: opening, waiting or caught up.
	stepped *SOReplayState
	// held is the generation of the latest state that held a new operation
	// of another peer.
	held uint64
	// applied is the generation of the latest state a step caught up with.
	applied uint64
	// state is the derived state last told to observe.
	state *SOReplayState
	// shown is the report last sent, or nil before the first.
	shown *BodyReport
	// observe receives each move of state, or is nil.
	observe func(*SOReplayState)
	// send receives each changed report, or is nil.
	send func(*BodyReport)
}

// newReplayMachine returns the machine before its first step.
func newReplayMachine(observe func(*SOReplayState), send func(*BodyReport)) *replayMachine {
	return &replayMachine{stepped: replayOpening, state: replayOpening, observe: observe, send: send}
}

// arrive records that the state of generation gen holds a new operation of
// another peer, moving a caught up machine to catching up.
func (m *replayMachine) arrive(gen uint64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.held = max(m.held, gen)
	m.move()
}

// step records what a step to the state of generation gen reached: caught up,
// waiting, or opening for a step that has not reached the end yet.
func (m *replayMachine) step(state *SOReplayState, gen uint64) {
	// Record what the step reached, and the generation it caught up with.
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.stepped = state
	if state.GetCaughtUp() != nil {
		m.applied = max(m.applied, gen)
	}
	m.move()
}

// show sends report with the current state when it differs from the last.
func (m *replayMachine) show(report *BodyReport) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.showLocked(report)
}

// shownAny reports whether a report was sent.
func (m *replayMachine) shownAny() bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.shown != nil
}

// move derives the state and tells the body and the health when it moved.
// The caller holds mtx.
func (m *replayMachine) move() {
	// Derive the state: a caught up step behind a newer arrival is catching up.
	state := m.stepped
	if state.GetCaughtUp() != nil && m.held > m.applied {
		state = replayCatchingUp
	}
	if state.EqualVT(m.state) {
		return
	}

	// Tell the body, then the health, of the move.
	m.state = state
	if m.observe != nil {
		m.observe(state)
	}
	if m.shown != nil {
		next := *m.shown
		m.showLocked(&next)
	}
}

// showLocked sends report with the current state when it changed. The caller
// holds mtx.
func (m *replayMachine) showLocked(report *BodyReport) {
	report.Replay = m.state
	if m.send == nil || report.equal(m.shown) {
		return
	}
	m.shown = report
	m.send(report)
}

// foreignPositions tracks the highest position, by author nonce, of each other
// peer's operations the states held so far.
type foreignPositions struct {
	// self is the local peer, whose operations are excluded.
	self string
	// nonces is the highest nonce of each other author held.
	nonces map[string]uint64
}

// newForeignPositions returns the positions before any state, for self.
func newForeignPositions(self string) *foreignPositions {
	return &foreignPositions{self: self, nonces: make(map[string]uint64)}
}

// advance records the positions ev holds and reports whether it holds an
// operation of another peer above the highest position of that peer held
// before: one with data in the order, or any covered by a checkpoint, whose
// acknowledgments cannot be told apart. An acknowledgment in the order
// applies nothing, and an operation the order does not place yet cannot be
// applied, so neither counts.
func (p *foreignPositions) advance(ev *OrderEvent) bool {
	arrived := false
	for _, pos := range ev.Checkpoint.GetAuthors() {
		if id := pos.GetPeerId(); id != p.self && pos.GetNonce() > p.nonces[id] {
			p.nonces[id] = pos.GetNonce()
			arrived = true
		}
	}
	for _, h := range ev.Order {
		inner := ev.Set.Get(h)
		id := inner.GetPeerId()
		if id == p.self || inner.GetNonce() <= p.nonces[id] {
			continue
		}
		p.nonces[id] = inner.GetNonce()
		arrived = arrived || !inner.IsAcknowledgment()
	}
	return arrived
}
