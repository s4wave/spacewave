//go:build !js

package bldr_buildbudget

import "testing"

func TestNewDefaultBudgetFromEnv(t *testing.T) {
	t.Setenv(MemoryBudgetEnv, "8")
	budget, err := newDefaultBudget()
	if err != nil {
		t.Fatal(err)
	}
	if got := budget.Capacity(); got != 8 {
		t.Fatalf("budget capacity = %d GiB, want 8 GiB", got)
	}
}

func TestNewDefaultBudgetRejectsInvalidEnv(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-a-number"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(MemoryBudgetEnv, value)
			if _, err := newDefaultBudget(); err == nil {
				t.Fatalf("newDefaultBudget accepted %q", value)
			}
		})
	}
}
