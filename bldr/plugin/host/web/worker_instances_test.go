package plugin_host_web

import (
	"context"
	"testing"

	web_worker "github.com/s4wave/spacewave/bldr/web/worker"
	"github.com/sirupsen/logrus"
)

// sweepTestWorker is a web_worker.WebWorker fake that records removals.
type sweepTestWorker struct {
	id      string
	gen     string
	ready   bool
	removed bool
}

func (w *sweepTestWorker) GetId() string         { return w.id }
func (w *sweepTestWorker) GetDocumentId() string { return "doc-1" }
func (w *sweepTestWorker) GetGeneration() string { return w.gen }
func (w *sweepTestWorker) GetShared() bool       { return false }
func (w *sweepTestWorker) Remove(_ context.Context) (bool, error) {
	w.removed = true
	return true, nil
}

// sweepTestDoc is a minimal web_document.WebDocument fake holding workers.
type sweepTestDoc struct {
	uuid    string
	workers map[string]web_worker.WebWorker
}

func (d *sweepTestDoc) GetWebDocumentUuid() string { return d.uuid }
func (d *sweepTestDoc) GetWebWorkers(context.Context) (map[string]web_worker.WebWorker, error) {
	return d.workers, nil
}

var _ workerListingDoc = (*sweepTestDoc)(nil)

func newSweepTestDoc(workers ...*sweepTestWorker) *sweepTestDoc {
	d := &sweepTestDoc{uuid: "doc-1", workers: make(map[string]web_worker.WebWorker)}
	for _, w := range workers {
		d.workers[w.id] = w
	}
	return d
}

const (
	sweepTestPluginWorkerID = "plugin/spacewave-sql/space/local/acct/so1"
	sweepTestOwnGeneration  = "exec-new"
	sweepTestOldGeneration  = "exec-old"
)

// TestTrackingStartupKeepsReadyOwnGenerationWorker fails before the
// generation fence: tracking startup removed the ready worker the current
// execution had just created, closing it under the app mid body mount.
func TestTrackingStartupKeepsReadyOwnGenerationWorker(t *testing.T) {
	own := &sweepTestWorker{
		id:    sweepTestPluginWorkerID,
		gen:   sweepTestOwnGeneration,
		ready: true,
	}
	doc := newSweepTestDoc(own, &sweepTestWorker{
		id:  "plugin/other/inst",
		gen: sweepTestOwnGeneration,
	})

	got, err := removeStaleWebWorkerInstances(
		context.Background(), doc,
		logrus.WithField("test", t.Name()),
		"runtime-1", sweepTestPluginWorkerID, sweepTestOwnGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if own.removed {
		t.Fatal("tracking startup removed the ready worker of its own generation")
	}
	if _, ok := doc.workers[sweepTestPluginWorkerID]; !ok {
		t.Fatal("ready own-generation worker no longer registered")
	}
	if len(got) != 0 {
		t.Fatalf("expected no removed instances, got %d", len(got))
	}
}

// TestTrackingStartupReclaimsStalePredecessor keeps crash reclamation: a
// ready worker from a previous execution is stale under the scheduler's
// single-execution-per-plugin contract and must be removed at startup.
func TestTrackingStartupReclaimsStalePredecessor(t *testing.T) {
	stale := &sweepTestWorker{
		id:    sweepTestPluginWorkerID,
		gen:   sweepTestOldGeneration,
		ready: true,
	}
	doc := newSweepTestDoc(stale)

	got, err := removeStaleWebWorkerInstances(
		context.Background(), doc,
		logrus.WithField("test", t.Name()),
		"runtime-1", sweepTestPluginWorkerID, sweepTestOwnGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.removed {
		t.Fatal("stale predecessor worker from a previous execution was not reclaimed")
	}
	if len(got) != 1 {
		t.Fatalf("expected the removed stale instance to be reported, got %d", len(got))
	}
}

// TestExecutionCleanupRemovesOnlyOwnGeneration pins the exit path: cleanup
// removes this execution's worker and leaves other generations alone.
func TestExecutionCleanupRemovesOnlyOwnGeneration(t *testing.T) {
	own := &sweepTestWorker{id: sweepTestPluginWorkerID, gen: sweepTestOwnGeneration}
	if _, err := removeOwnWebWorkerInstances(
		context.Background(), newSweepTestDoc(own),
		logrus.WithField("test", t.Name()),
		"runtime-1", sweepTestPluginWorkerID, sweepTestOwnGeneration,
	); err != nil {
		t.Fatal(err)
	}
	if !own.removed {
		t.Fatal("execution cleanup did not remove its own generation")
	}

	foreign := &sweepTestWorker{
		id:    sweepTestPluginWorkerID,
		gen:   sweepTestOldGeneration,
		ready: true,
	}
	got, err := removeOwnWebWorkerInstances(
		context.Background(), newSweepTestDoc(foreign),
		logrus.WithField("test", t.Name()),
		"runtime-1", sweepTestPluginWorkerID, sweepTestOwnGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no removed instances for foreign generation, got %d", len(got))
	}
	if foreign.removed {
		t.Fatal("execution cleanup removed a foreign generation")
	}
}
