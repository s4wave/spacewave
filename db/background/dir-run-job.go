package background

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// JobFunc runs one attempt of a background job.
//
// ctx is marked with block.WithBackground, so fetches the job causes yield to
// foreground reads on the same store. ctx ends when the job is released. A nil
// return finishes the job. An error retries the job with backoff.
type JobFunc func(ctx context.Context) error

// RunJob is a directive to run a background job.
//
// Equivalent directives, those with the same key, share one run with the
// function of the first one added, so a key must identify the same work for
// every caller. Releasing the last of them cancels the run. A finished job
// stays done while any registrant holds the job. Registering the key again
// after all registrants released the job runs the job again.
//
// Value: *JobState, replaced on each transition.
type RunJob interface {
	// Directive indicates RunJob is a directive.
	directive.Directive

	// RunJobKey identifies the work. Cannot be empty.
	RunJobKey() string
	// RunJobFunc returns the function that runs one attempt.
	RunJobFunc() JobFunc
}

// runJob implements RunJob.
type runJob struct {
	// key identifies the work.
	key string
	// run runs one attempt.
	run JobFunc
}

// NewRunJob constructs a new RunJob directive.
func NewRunJob(key string, run JobFunc) RunJob {
	return &runJob{key: key, run: run}
}

// AddJob registers a background job on the bus. The job runs until the
// returned reference is released, unless another registrant of the same key
// still holds the job.
func AddJob(ctx context.Context, b bus.Bus, key string, run JobFunc) (directive.Reference, error) {
	_, ref, err := b.AddDirective(ctx, NewRunJob(key, run), nil)
	return ref, err
}

// RunJobKey identifies the work.
func (d *runJob) RunJobKey() string {
	return d.key
}

// RunJobFunc returns the function that runs one attempt.
func (d *runJob) RunJobFunc() JobFunc {
	return d.run
}

// Validate validates the directive.
func (d *runJob) Validate() error {
	if d.key == "" {
		return errors.New("background job key cannot be empty")
	}
	if d.run == nil {
		return errors.New("background job function cannot be nil")
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *runJob) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent checks if the other directive runs the same job.
func (d *runJob) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(RunJob)
	return ok && d.key == od.RunJobKey()
}

// Superceeds checks if the directive overrides another.
func (d *runJob) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *runJob) GetName() string {
	return "RunBackgroundJob"
}

// GetDebugVals returns the directive arguments stringified.
func (d *runJob) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"key": []string{d.key}}
}

// _ is a type assertion
var (
	_ RunJob               = (*runJob)(nil)
	_ directive.Debuggable = (*runJob)(nil)
)
