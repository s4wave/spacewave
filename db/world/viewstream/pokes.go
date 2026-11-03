package viewstream

import "sync"

// Pokes lets the watchers of a feed that acts for no one actor rerun it at
// once after their own write, as a bound view's pacer does for its actor.
type Pokes struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// poked reports a poke since the latest run started.
	poked bool
	// wake closes at the next poke.
	wake chan struct{}
}

// NewPokes constructs Pokes with none pending.
func NewPokes() *Pokes {
	return &Pokes{wake: make(chan struct{})}
}

// Poke asks for the next change to pass at once.
func (p *Pokes) Poke() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if !p.poked {
		p.poked = true
		close(p.wake)
	}
}

// Pending reports a poke since the latest run started, or else returns a
// channel that closes at the next one.
func (p *Pokes) Pending() (bool, <-chan struct{}) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return p.poked, p.wake
}

// Clear forgets the Pokes a starting run answers.
func (p *Pokes) Clear() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if p.poked {
		p.poked = false
		p.wake = make(chan struct{})
	}
}
