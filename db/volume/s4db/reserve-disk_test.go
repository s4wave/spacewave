//go:build !js && !wasip1

package volume_s4db_test

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// reserveDiskEnv names a directory on a small file system the disk reserve
// run-through fills. The run-through is skipped without it.
const reserveDiskEnv = "SPACEWAVE_RESERVE_DISK_DIR"

// TestReserveDisk fills a real file system through a volume controller whose
// scheduled collection is an hour away. Each write replaces the bucket's head,
// so the earlier heads are garbage. It checks that writes stop with
// volume.ErrReserve before the disk is full, that reads continue, that the
// controller collects at once and the volume returns to writable, and that
// writes then resume.
func TestReserveDisk(t *testing.T) {
	// Register the factory on a test bus.
	dir := os.Getenv(reserveDiskEnv)
	if dir == "" {
		t.Skip("set " + reserveDiskEnv + " to a directory on a small file system")
	}
	ctx := t.Context()
	b, sr, err := core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sr.AddFactory(volume_s4db.NewFactory(b))

	// Load the volume on the named file system with a 128 MiB reserve.
	conf := &volume_s4db.Config{
		Path:         filepath.Join(dir, "volume.s4wave"),
		ReserveBytes: 128 << 20,
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
	}
	ctrl, _, ref, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx, b, resolver.NewLoadControllerWithConfig(conf), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	// Reach the volume's bucket roots.
	gv, err := ctrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vol, ok := gv.(*volume_s4db.Volume)
	if !ok {
		t.Fatalf("volume %T is not an s4db volume", gv)
	}

	// Write 1 MiB heads until the reserve refuses one.
	var head *block.BlockRef
	var n int
	for ; ; n++ {
		next, err := writeHead(t, vol, n)
		if errors.Is(err, volume.ErrReserve) {
			break
		}
		if err != nil {
			t.Fatalf("write %d before the reserve: %v", n, err)
		}
		head = next
	}
	refused, _ := vol.GetHealthSnapshotWithWait()
	t.Logf("refused write %d: free %d MiB, file %d MiB, reserve %d MiB, state %v",
		n, refused.GetFreeDiskBytes()>>20, refused.GetFileBytes()>>20,
		refused.GetReserveBytes()>>20, refused.GetWriteState())

	// Reads continue while writes are refused.
	if data, found, err := vol.GetBlock(ctx, head); err != nil || !found || len(data) != 1<<20 {
		t.Fatalf("read of the head below the reserve: found %v, %v", found, err)
	}

	// The controller collects the replaced heads and the volume returns to
	// writable.
	start := time.Now()
	deadline := time.After(2 * time.Minute)
	for {
		h, waitCh := vol.GetHealthSnapshotWithWait()
		if h.GetWriteState() == volume.VolumeWriteState_VOLUME_WRITE_STATE_WRITABLE {
			t.Logf("writable after %v: free %d MiB, file %d MiB",
				time.Since(start).Round(time.Millisecond), h.GetFreeDiskBytes()>>20, h.GetFileBytes()>>20)
			break
		}
		select {
		case <-waitCh:
		case <-deadline:
			t.Fatalf("still %v after two minutes: free %d MiB", h.GetWriteState(), h.GetFreeDiskBytes()>>20)
		}
	}

	// Writes resume.
	for i := range 16 {
		if _, err := writeHead(t, vol, n+1+i); err != nil {
			t.Fatalf("write %d after collection: %v", i, err)
		}
	}
}

// writeHead writes a 1 MiB block numbered n as the fill bucket's head and
// drops the write's staging edge, so the head it replaced is garbage.
func writeHead(t *testing.T, vol *volume_s4db.Volume, n int) (*block.BlockRef, error) {
	// Build a block distinct by its number.
	t.Helper()
	ctx := t.Context()
	data := make([]byte, 1<<20)
	binary.BigEndian.PutUint64(data, uint64(n))
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write it as the head and release its staging edge.
	entries := []*block.PutBatchEntry{{Ref: ref, Data: data}}
	if err := vol.SetBucketRoots(ctx, "fill", entries, []block.NamedRoot{{Name: "head", Ref: ref}}); err != nil {
		return nil, err
	}
	return ref, vol.ReleaseBucketRoots(ctx, "fill", []*block.BlockRef{ref})
}
