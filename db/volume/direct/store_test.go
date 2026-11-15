package direct

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
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

// TestGetBlockRejectsWrongBytes checks that a record holding bytes under a ref
// they do not hash to reads as ErrBlockCorrupt with no data, through both reads.
func TestGetBlockRejectsWrongBytes(t *testing.T) {
	s, err := Open(t.Context(), records.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Store other bytes under the ref, below the put's own hash check.
	ref, err := block.BuildBlockRef([]byte("block data"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.add(t.Context(), []*block.PutBatchEntry{{Ref: ref, Data: []byte("other data")}}); err != nil {
		t.Fatal(err)
	}

	// Both reads report the mismatch and return no bytes.
	data, found, err := s.GetBlock(t.Context(), ref)
	if !errors.Is(err, block.ErrBlockCorrupt) || found || data != nil {
		t.Fatalf("GetBlock: found %v data %q err %v", found, data, err)
	}
	stored, err := s.GetStoredBlock(t.Context(), ref)
	if !errors.Is(err, block.ErrBlockCorrupt) || stored != nil {
		t.Fatalf("GetStoredBlock: %v err %v", stored, err)
	}
}
