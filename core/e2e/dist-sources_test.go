//go:build !js

package s4wave_core_e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestCheckoutWebDistSourcesMaterializesVendorGoModules(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "../.."))
	tmpRoot := filepath.Join(repoRoot, ".tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workDir, err := os.MkdirTemp(tmpRoot, "core-e2e-dist-sources-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(workDir); err != nil {
			t.Errorf("cleanup dist source test dir: %v", err)
		}
	})
	distDir := filepath.Join(workDir, "src")

	if err := CheckoutWebDistSources(ctx, le, repoRoot, distDir); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"vendor/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.ts",
		"vendor/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.ts",
		"vendor/github.com/aperturerobotics/util/csync/rwmutex.ts",
	} {
		if _, err := os.Stat(filepath.Join(distDir, path)); err != nil {
			t.Fatalf("expected dist vendor file %s: %v", path, err)
		}
	}
}
