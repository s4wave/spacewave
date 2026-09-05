//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/pkg/errors"
	bldr_buildbudget "github.com/s4wave/spacewave/bldr/util/buildbudget"
	"github.com/sirupsen/logrus"
)

func TestAnalyzePackagesWaitsForBuildBudget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.test/budget\n\ngo 1.26.2\n")
	budget, err := bldr_buildbudget.Default()
	if err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		permit, err := budget.Acquire(t.Context(), budget.Capacity())
		if err != nil {
			t.Fatal(err)
		}
		defer permit.Release()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := AnalyzePackages(ctx, logrus.NewEntry(logrus.New()), dir, []string{"."}, nil, "js", "wasm", false)
			done <- err
		}()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("analysis ran while the build budget was occupied: %v", err)
		default:
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel waiting analysis: %v", err)
		}
	})
}
