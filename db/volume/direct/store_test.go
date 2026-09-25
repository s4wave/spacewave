package direct

import (
	"context"
	"testing"

	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/volume/crashtest"
	"github.com/s4wave/spacewave/db/volume/records"
)

// TestKV runs the kvtx conformance tests.
func TestKV(t *testing.T) {
	s, err := Open(t.Context(), records.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := kvtx_kvtest.TestAll(t.Context(), s); err != nil {
		t.Fatal(err)
	}
}

// TestCrashRecovery runs the crash workload on the memory record store.
func TestCrashRecovery(t *testing.T) {
	crashtest.Run(t, crashtest.AllBlocks, records.NewMemory, func(ctx context.Context, rs *records.Memory) (crashtest.Target, error) {
		return Open(ctx, rs)
	})
}
