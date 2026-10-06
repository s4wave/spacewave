package webrtc

import "time"

const (
	// offerRetryMin is how long an unanswered offer waits for its first
	// retransmit.
	offerRetryMin = 2 * time.Second
	// offerRetryMax caps the wait between retransmits, which doubles each time.
	// A peer that stays offline costs one offer for each wait.
	offerRetryMax = time.Minute
)

// offerRetry paces the retransmits of an offer that no answer has reached. The
// signaling relay delivers at most once and the answerer asks for an offer only
// once, so the offerer owns the retry until the answer arrives. The zero value
// is idle.
type offerRetry struct {
	// timer fires when the next retransmit is due. It is nil while idle or
	// after it fired and before the next arming.
	timer *time.Timer
	// delay is the wait of the armed timer or, after it fired, of the next one.
	// It is zero while idle.
	delay time.Duration
}

// due returns the channel that fires when a retransmit is due. It is nil, and
// blocks forever, while no timer is armed.
func (r *offerRetry) due() <-chan time.Time {
	if r.timer == nil {
		return nil
	}
	return r.timer.C
}

// sync arms the timer while the offer awaits its answer, and goes idle
// otherwise. Going idle restarts the backoff.
func (r *offerRetry) sync(awaitingAnswer bool) {
	// Go idle once the offer needs no retransmit.
	if !awaitingAnswer {
		r.stop()
		return
	}

	// Arm the timer unless one is already running.
	if r.timer != nil {
		return
	}
	if r.delay == 0 {
		r.delay = offerRetryMin
	}
	r.timer = time.NewTimer(r.delay)
}

// fired records that the timer expired and lengthens the next wait.
func (r *offerRetry) fired() {
	r.timer = nil
	r.delay = min(2*r.delay, offerRetryMax)
}

// stop disarms the timer and restarts the backoff.
func (r *offerRetry) stop() {
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer, r.delay = nil, 0
}
