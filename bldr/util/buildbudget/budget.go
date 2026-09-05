// Package bldr_buildbudget bounds concurrent memory-heavy Bldr build stages.
package bldr_buildbudget

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
)

const (
	// MemoryBudgetEnv configures the shared build budget in GiB.
	MemoryBudgetEnv = "BLDR_BUILD_MEMORY_BUDGET_GIB"

	// GoScriptCompileWeight is the estimated memory cost of one GoScript compile.
	GoScriptCompileWeight int64 = 4

	// GoAnalysisWeight is the estimated memory cost of one plugin package analysis.
	GoAnalysisWeight int64 = 2

	// ViteBuildWeight is the estimated memory cost of one Vite build.
	ViteBuildWeight int64 = 2

	// defaultBudgetFractionDenominator divides available host memory to
	// derive the default budget capacity.
	defaultBudgetFractionDenominator int64 = 4
)

// Budget bounds memory-heavy build stages across all Bldr build paths.
type Budget struct {
	// semaphore admits build stages up to the configured capacity.
	semaphore *semaphore.Weighted
	// capacity is the total budget in GiB.
	capacity int64
}

var (
	// defaultBudgetOnce constructs the default budget on first use.
	defaultBudgetOnce sync.Once
	// defaultBudget is the shared process-wide build budget.
	defaultBudget *Budget
	// defaultBudgetErr is the construction error from defaultBudgetOnce.
	defaultBudgetErr error
)

// NewBudget constructs a shared build budget with a GiB capacity.
func NewBudget(capacity int64) (*Budget, error) {
	if capacity < GoScriptCompileWeight {
		return nil, errors.Errorf(
			"build budget must be at least %d GiB, got %d GiB",
			GoScriptCompileWeight,
			capacity,
		)
	}
	return &Budget{
		semaphore: semaphore.NewWeighted(capacity),
		capacity:  capacity,
	}, nil
}

// Default returns the process-wide build budget configured from the environment.
func Default() (*Budget, error) {
	defaultBudgetOnce.Do(func() {
		defaultBudget, defaultBudgetErr = newDefaultBudget()
	})
	return defaultBudget, defaultBudgetErr
}

// Capacity returns the configured budget in GiB.
func (b *Budget) Capacity() int64 {
	return b.capacity
}

// Acquire waits for weight GiB of shared build capacity or context cancellation.
func (b *Budget) Acquire(ctx context.Context, weight int64) (*Permit, error) {
	if weight <= 0 {
		return nil, errors.Errorf("build budget weight must be positive, got %d", weight)
	}
	if weight > b.capacity {
		return nil, errors.Errorf(
			"build budget weight %d GiB exceeds capacity %d GiB",
			weight,
			b.capacity,
		)
	}
	if err := b.semaphore.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	return &Permit{budget: b, weight: weight, released: new(sync.Once)}, nil
}

// newDefaultBudget constructs the process-wide budget from the environment
// override or available host memory.
func newDefaultBudget() (*Budget, error) {
	// Prefer the explicit environment override when set.
	raw := strings.TrimSpace(os.Getenv(MemoryBudgetEnv))
	if raw != "" {
		capacity, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parse %s", MemoryBudgetEnv)
		}
		return NewBudget(capacity)
	}

	// Otherwise derive the budget from available host memory.
	availableBytes, err := availableHostMemoryBytes()
	if err != nil {
		return nil, errors.Wrap(err, "resolve available host memory")
	}
	availableGiB := int64(availableBytes / (1 << 30))
	capacity := max(availableGiB/defaultBudgetFractionDenominator, GoScriptCompileWeight)
	if capacity <= 0 {
		return nil, errors.New("available host memory produced an invalid build budget")
	}
	return NewBudget(capacity)
}
