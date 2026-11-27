//go:build !js

package bldr_plugin_compiler_go

import (
	"testing"

	"github.com/s4wave/spacewave/testbed/reporoot"
	"github.com/sirupsen/logrus"
)

// TestDiscoveryWorkerFactory pins that the Docker factory, which a Worker
// registers with its own admission, is not discovered as a bus-level factory.
func TestDiscoveryWorkerFactory(t *testing.T) {
	// Analyze the real package, which must load but declare no NewFactory.
	const dockerPkg = "github.com/s4wave/spacewave/forge/lib/docker"
	an, err := AnalyzePackages(
		t.Context(),
		logrus.NewEntry(logrus.New()),
		reporoot.Must(t),
		[]string{"./forge/lib/docker"},
		nil,
		"linux",
		"amd64",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	// The analysis must hold the package without a discovered factory.
	pkg := an.packages[dockerPkg]
	if pkg == nil {
		t.Fatalf("package %s was not analyzed", dockerPkg)
	}
	if pkg.Factory != nil {
		t.Fatalf("package %s declares a bus-level factory", dockerPkg)
	}
}
