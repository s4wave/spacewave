//go:build !js

package bldr_buildbudget

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestBudgetAdmissionCoversGoScriptAndViteStages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Prepare a shared build budget inside the deterministic concurrency test.
		budget, err := NewBudget(4)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()

		// Hold the GoScript compile permit until its stage finishes.
		goScriptPermit, err := budget.Acquire(ctx, GoScriptCompileWeight)
		if err != nil {
			t.Fatalf("acquire GoScript budget: %v", err)
		}
		defer goScriptPermit.Release()

		// Start the Vite stage and wait until its budget acquisition blocks.
		viteAcquired := make(chan *Permit, 1)
		go func() {
			permit, acquireErr := budget.Acquire(ctx, ViteBuildWeight)
			if acquireErr == nil {
				viteAcquired <- permit
				return
			}
			viteAcquired <- nil
		}()
		synctest.Wait()

		// Verify Vite cannot acquire while GoScript holds the budget.
		select {
		case <-viteAcquired:
			t.Fatal("Vite stage acquired budget while GoScript stage held it")
		default:
		}

		// Finish GoScript and verify Vite acquires the released capacity.
		goScriptPermit.Release()
		vitePermit := <-viteAcquired
		if vitePermit == nil {
			t.Fatal("Vite stage failed to acquire shared budget")
		}
		vitePermit.Release()
	})
}

func TestBudgetContextCancellationDoesNotLeakCapacity(t *testing.T) {
	// Prepare a shared build budget for cancellation coverage.
	budget, err := NewBudget(4)
	if err != nil {
		t.Fatal(err)
	}

	// Acquire the budget before introducing a canceled waiter.
	first, err := budget.Acquire(context.Background(), GoScriptCompileWeight)
	if err != nil {
		t.Fatalf("acquire first permit: %v", err)
	}

	// Verify a canceled Vite acquisition fails.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := budget.Acquire(ctx, ViteBuildWeight); err == nil {
		t.Fatal("canceled acquisition succeeded")
	}

	// Release the first stage and verify canceled acquisition did not consume capacity.
	first.Release()
	second, err := budget.Acquire(context.Background(), GoScriptCompileWeight)
	if err != nil {
		t.Fatalf("acquire after canceled waiter: %v", err)
	}
	second.Release()
}

func TestBudgetRejectsOversizedStage(t *testing.T) {
	budget, err := NewBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := budget.Acquire(context.Background(), GoScriptCompileWeight+1); err == nil {
		t.Fatal("oversized stage was admitted")
	}
}
