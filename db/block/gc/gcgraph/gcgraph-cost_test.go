//go:build js

package gcgraph

import (
	"context"
	"os"
	"slices"
	"strconv"
	"syscall/js"
	"testing"
	"time"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/opfs"
)

func TestGCGraphRemovalCostByBatchSize(t *testing.T) {
	if os.Getenv("SPACEWAVE_GCGRAPH_OPFS_COST") == "" {
		t.Skip("set SPACEWAVE_GCGRAPH_OPFS_COST=1 to run the timed harness")
	}

	trialValues := make(map[int][]opfsRemovalCost, len(opfsRemovalCostCheckpoints))
	for trial := 1; trial <= opfsRemovalCostTrialCount; trial++ {
		for _, checkpoint := range opfsRemovalCostCheckpoints {
			value := runOPFSRemovalCostTrial(t, trial, checkpoint)
			trialValues[checkpoint] = append(trialValues[checkpoint], value)
			t.Logf(
				"trial=%d removals=%d existing=%d missing=%d file_ops=%d prepare_file_ops=%d mutation_file_ops=%d prepare=%s mutation=%s owner_held=%s",
				trial,
				checkpoint,
				(checkpoint+1)/2,
				checkpoint/2,
				value.fileOperations,
				value.preparationFileOperations,
				value.mutationFileOperations,
				value.preparation,
				value.mutation,
				value.ownerHeld,
			)
		}
	}

	for _, checkpoint := range opfsRemovalCostCheckpoints {
		fileOperations := make([]int, 0, opfsRemovalCostTrialCount)
		preparationFileOperations := make([]int, 0, opfsRemovalCostTrialCount)
		mutationFileOperations := make([]int, 0, opfsRemovalCostTrialCount)
		preparation := make([]time.Duration, 0, opfsRemovalCostTrialCount)
		mutation := make([]time.Duration, 0, opfsRemovalCostTrialCount)
		ownerHeld := make([]time.Duration, 0, opfsRemovalCostTrialCount)
		for _, value := range trialValues[checkpoint] {
			fileOperations = append(fileOperations, value.fileOperations)
			preparationFileOperations = append(
				preparationFileOperations,
				value.preparationFileOperations,
			)
			mutationFileOperations = append(
				mutationFileOperations,
				value.mutationFileOperations,
			)
			preparation = append(preparation, value.preparation)
			mutation = append(mutation, value.mutation)
			ownerHeld = append(ownerHeld, value.ownerHeld)
		}
		slices.Sort(fileOperations)
		slices.Sort(preparationFileOperations)
		slices.Sort(mutationFileOperations)
		slices.Sort(preparation)
		slices.Sort(mutation)
		slices.Sort(ownerHeld)
		medianIndex := len(fileOperations) / 2
		t.Logf(
			"median removals=%d trials=%d file_ops=%d prepare_file_ops=%d mutation_file_ops=%d prepare=%s mutation=%s owner_held=%s",
			checkpoint,
			len(fileOperations),
			fileOperations[medianIndex],
			preparationFileOperations[medianIndex],
			mutationFileOperations[medianIndex],
			preparation[medianIndex],
			mutation[medianIndex],
			ownerHeld[medianIndex],
		)
	}
}

const opfsRemovalCostTrialCount = 7

var opfsRemovalCostCheckpoints = []int{1, 8, 16, 32, 64, 96, 128}

type opfsRemovalCost struct {
	fileOperations            int
	preparationFileOperations int
	mutationFileOperations    int
	preparation               time.Duration
	mutation                  time.Duration
	ownerHeld                 time.Duration
}

func runOPFSRemovalCostTrial(t *testing.T, trial, edgeCount int) opfsRemovalCost {
	t.Helper()
	ctx := context.Background()
	name := "test-gcgraph-removal-cost-" + strconv.Itoa(trial) + "-" + strconv.Itoa(edgeCount)

	baseDriver := opfs.DefaultDriver
	countingDriver := &countingOPFSDriver{Driver: baseDriver}
	opfs.DefaultDriver = countingDriver
	defer func() { opfs.DefaultDriver = baseDriver }()
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := opfs.DeleteEntry(root, name, true); err != nil && !opfs.IsNotFound(err) {
		t.Fatal(err)
	}

	g, cleanup := newTestGraph(t, name)
	defer cleanup()

	removes := make([]block_gc.RefEdge, edgeCount)
	for i := range removes {
		removes[i] = block_gc.RefEdge{
			Subject: name + "/subject/" + strconv.Itoa(i),
			Object:  name + "/object/" + strconv.Itoa(i),
		}
		if i%2 == 0 {
			if err := g.AddRef(ctx, removes[i].Subject, removes[i].Object); err != nil {
				t.Fatal(err)
			}
		}
	}
	countingDriver.fileOperations = 0

	release, err := g.acquireOwnershipLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	lockStart := time.Now()

	start := time.Now()
	adds, existingRemoves, err := g.prepareRefBatch(ctx, nil, removes)
	preparation := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	preparationFileOperations := countingDriver.fileOperations
	if len(existingRemoves) != (edgeCount+1)/2 {
		t.Fatalf("prepared %d existing removals, want %d", len(existingRemoves), (edgeCount+1)/2)
	}
	if len(adds) != len(existingRemoves) {
		t.Fatalf("prepared %d orphan additions, want %d", len(adds), len(existingRemoves))
	}

	start = time.Now()
	err = g.applyPreparedRefBatch(adds, existingRemoves)
	mutation := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	fileOperations := countingDriver.fileOperations
	release()
	released = true
	ownerHeld := time.Since(lockStart)

	return opfsRemovalCost{
		fileOperations:            fileOperations,
		preparationFileOperations: preparationFileOperations,
		mutationFileOperations:    fileOperations - preparationFileOperations,
		preparation:               preparation,
		mutation:                  mutation,
		ownerHeld:                 ownerHeld,
	}
}

// countingOPFSDriver counts calls at the OPFS driver boundary while preserving
// the selected driver's file and locking behavior.
type countingOPFSDriver struct {
	opfs.Driver
	fileOperations int
}

func (d *countingOPFSDriver) GetDirectory(parent js.Value, name string, create bool) (js.Value, error) {
	d.fileOperations++
	return d.Driver.GetDirectory(parent, name, create)
}

func (d *countingOPFSDriver) OpenAsyncFile(dir js.Value, name string) (*opfs.AsyncFile, error) {
	d.fileOperations++
	return d.Driver.OpenAsyncFile(dir, name)
}

func (d *countingOPFSDriver) CreateAsyncFile(dir js.Value, name string) (*opfs.AsyncFile, error) {
	d.fileOperations++
	return d.Driver.CreateAsyncFile(dir, name)
}

func (d *countingOPFSDriver) WriteFile(dir js.Value, name string, data []byte) error {
	d.fileOperations++
	return d.Driver.WriteFile(dir, name, data)
}

func (d *countingOPFSDriver) ReadFile(dir js.Value, name string) ([]byte, error) {
	d.fileOperations++
	return d.Driver.ReadFile(dir, name)
}

func (d *countingOPFSDriver) DeleteEntry(dir js.Value, name string, recursive bool) error {
	d.fileOperations++
	return d.Driver.DeleteEntry(dir, name, recursive)
}

func (d *countingOPFSDriver) ListDirectory(dir js.Value) ([]string, error) {
	d.fileOperations++
	return d.Driver.ListDirectory(dir)
}

func (d *countingOPFSDriver) FileExists(dir js.Value, name string) (bool, error) {
	d.fileOperations++
	return d.Driver.FileExists(dir, name)
}

func (d *countingOPFSDriver) OpenSyncFile(dir js.Value, name string) (*opfs.SyncFile, error) {
	d.fileOperations++
	return d.Driver.OpenSyncFile(dir, name)
}

func (d *countingOPFSDriver) CreateSyncFile(dir js.Value, name string) (*opfs.SyncFile, error) {
	d.fileOperations++
	return d.Driver.CreateSyncFile(dir, name)
}

func (d *countingOPFSDriver) CreateSyncFileContext(
	ctx context.Context,
	dir js.Value,
	name string,
) (*opfs.SyncFile, error) {
	d.fileOperations++
	return d.Driver.CreateSyncFileContext(ctx, dir, name)
}
