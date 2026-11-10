package block_gc

import (
	"context"
	"fmt"
	"os"
	rtrace "runtime/trace"
)

// landUsePrior reports the measurement knob that skips the listed check of a
// child the replaced block's list names.
func landUsePrior() bool { return os.Getenv("LAND_PRIOR") != "" }

// landSkipListed reports the measurement knob that skips the listed check for
// every child of an adopted Volume: the upper bound, not a design.
func landSkipListed() bool { return os.Getenv("LAND_SKIP_LISTED") != "" }

// landStats counts one Land's listed checks.
type landStats struct {
	priors, priorFound, skipped, asked, unlisted int
}

// trace logs the counts to an execution trace.
func (s *landStats) trace(ctx context.Context, l *lander) {
	if !rtrace.IsEnabled() {
		return
	}
	rtrace.Log(ctx, "s4c-prior", fmt.Sprintf("adopted=%t parents=%d priors=%d found=%d skipped=%d asked=%d unlisted=%d",
		l.adopted, len(l.order), s.priors, s.priorFound, s.skipped, s.asked, s.unlisted))
}
