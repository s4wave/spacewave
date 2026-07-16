//go:build !js

package bldr_buildbudget

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestBudgetAdmissionCoversGoScriptAndViteStages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget, err := NewBudget(4)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()

		goScriptPermit, err := budget.Acquire(ctx, GoScriptCompileWeight)
		if err != nil {
			t.Fatalf("acquire GoScript budget: %v", err)
		}
		defer goScriptPermit.Release()

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

		select {
		case <-viteAcquired:
			t.Fatal("Vite stage acquired budget while GoScript stage held it")
		default:
		}

		goScriptPermit.Release()
		vitePermit := <-viteAcquired
		if vitePermit == nil {
			t.Fatal("Vite stage failed to acquire shared budget")
		}
		vitePermit.Release()
	})
}

func TestBudgetContextCancellationDoesNotLeakCapacity(t *testing.T) {
	budget, err := NewBudget(4)
	if err != nil {
		t.Fatal(err)
	}

	first, err := budget.Acquire(context.Background(), GoScriptCompileWeight)
	if err != nil {
		t.Fatalf("acquire first permit: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := budget.Acquire(ctx, ViteBuildWeight); err == nil {
		t.Fatal("canceled acquisition succeeded")
	}

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
