package sobject_world_engine

import (
	"context"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// slowCommitThreshold is the duration of a World write, from its writer lock
// wait to the end of its Commit, after which Commit logs its stage times. It
// is the duration after which a World call is reported as slow. Tests set it.
var slowCommitThreshold = 10 * time.Second

// commitStage names a stage of a World write.
type commitStage int

const (
	// stageLockWait waits for the writer lock.
	stageLockWait commitStage = iota
	// stageForkReplay replays the operation set and forks the candidate.
	stageForkReplay
	// stageCallback is the time between opening the write and calling Commit,
	// during which the caller holds the writer unless the write is optimistic.
	stageCallback
	// stageWorldStateCommit closes the operation buffer.
	stageWorldStateCommit
	// stageBlockCommit commits the candidate World blocks.
	stageBlockCommit
	// stageCommitLockWait is the wait of an optimistic write for the writer
	// lock at Commit.
	stageCommitLockWait
	// stageValidate replays the operation set for an optimistic write and
	// checks its reads against the changes since its fork.
	stageValidate
	// stagePublicationAdmission prepares the operation and waits until the
	// SharedObject admits it. A SharedObject that does not publish writes its
	// state here, so its receipt wait stays at zero.
	stagePublicationAdmission
	// stageAdvance replays the operation set and installs the World. The
	// writer lock is released at its end.
	stageAdvance
	// stageReceiptWait waits outside the writer lock for the publication to
	// be durable.
	stageReceiptWait
	// commitStageCount is the number of stages.
	commitStageCount
)

// commitStageNames are the log names of the stages.
var commitStageNames = [commitStageCount]string{
	stageLockWait:             "lock-wait",
	stageForkReplay:           "fork-replay",
	stageCallback:             "callback",
	stageWorldStateCommit:     "world-state-commit",
	stageBlockCommit:          "block-commit",
	stageCommitLockWait:       "commit-lock-wait",
	stageValidate:             "validate",
	stagePublicationAdmission: "publication-admission",
	stageAdvance:              "advance",
	stageReceiptWait:          "receipt-wait",
}

// commitClock times the stages of one World write with a clock read at each
// stage boundary. A nil commitClock records nothing.
type commitClock struct {
	// started is when the write began waiting for the writer lock.
	started time.Time
	// mark is the end of the last closed stage.
	mark time.Time
	// open is the earliest stage not yet closed.
	open commitStage
	// stages are the durations of the closed stages.
	stages [commitStageCount]time.Duration
	// observer receives the stage times when the write commits, or is nil.
	observer CommitObserver
	// optimistic is set when the write released the writer lock after its
	// fork, so its callback and candidate commit are not held.
	optimistic bool
}

// newCommitClock starts the clock at the writer lock wait. The write reports
// its stage times to the CommitObserver of ctx, if it has one.
func newCommitClock(ctx context.Context) *commitClock {
	now := time.Now()
	return &commitClock{started: now, mark: now, observer: commitObserverFromContext(ctx)}
}

// end closes stage at the current time. Earlier open stages that never ran
// close with no duration, and closing a closed stage does nothing.
func (c *commitClock) end(stage commitStage) {
	// Ignore a stage that is already closed.
	if c == nil || stage < c.open {
		return
	}

	// Attribute the time since the last boundary to the stage.
	now := time.Now()
	c.stages[stage] = now.Sub(c.mark)
	c.mark = now
	c.open = stage + 1
}

// held reports whether the write holds the writer lock during stage.
func (c *commitClock) held(stage commitStage) bool {
	switch stage {
	case stageLockWait, stageCommitLockWait, stageReceiptWait:
		return false
	case stageCallback, stageWorldStateCommit, stageBlockCommit:
		return !c.optimistic
	default:
		return true
	}
}

// finish reports the stage times of the write to its observer as Commit ends and
// logs them when the write took longer than slowCommitThreshold. The stages
// field lists them in stage order, the hold field is the writer lock hold: the
// stages that ran under the lock, the optimistic field says whether the
// callback ran without it, and the reads field is the scope of what the write
// read.
func (c *commitClock) finish(le *logrus.Entry, reads world.ReadScope) {
	// Skip the work when nothing reads the times.
	if c == nil {
		return
	}
	elapsed := time.Since(c.started)
	slow := elapsed > slowCommitThreshold
	if !slow && c.observer == nil {
		return
	}

	// Sum the hold over the stages that ran under the lock.
	var hold time.Duration
	for stage := range commitStageCount {
		if c.held(stage) {
			hold += c.stages[stage]
		}
	}

	// Report every stage to the observer.
	if c.observer != nil {
		times := CommitTimes{Elapsed: elapsed, Hold: hold, Optimistic: c.optimistic, Stages: make([]CommitStageTime, commitStageCount)}
		for stage, name := range commitStageNames {
			times.Stages[stage] = CommitStageTime{Name: name, Duration: c.stages[stage]}
		}
		c.observer(times)
	}

	// Name the duration of every stage in order for a slow write.
	if !slow {
		return
	}
	var stages strings.Builder
	for stage, name := range commitStageNames {
		if stage != 0 {
			stages.WriteByte(' ')
		}
		stages.WriteString(name + "=" + c.stages[stage].String())
	}
	le.WithFields(logrus.Fields{
		"elapsed":    elapsed,
		"hold":       hold,
		"stages":     stages.String(),
		"optimistic": c.optimistic,
		"reads":      reads.String(),
	}).Warn("slow World commit")
}
