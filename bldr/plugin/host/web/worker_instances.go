package plugin_host_web

import (
	"context"
	"maps"

	web_worker "github.com/s4wave/spacewave/bldr/web/worker"
	"github.com/sirupsen/logrus"
)

// workerListingDoc is the part of a WebDocument that lists workers.
type workerListingDoc interface {
	// GetWebDocumentUuid returns the web document identifier.
	GetWebDocumentUuid() string
	// GetWebWorkers returns the current snapshot of active WebWorkers.
	GetWebWorkers(ctx context.Context) (map[string]web_worker.WebWorker, error)
}

// removeWebWorkerInstances removes web workers matching the plugin worker ID
// for which shouldRemove returns true, and returns the workers that matched.
// The caller decides which generations are stale: tracking startup keeps the
// ready worker of the current execution, while execution cleanup removes only
// its own generation.
func removeWebWorkerInstances(
	ctx context.Context,
	doc workerListingDoc,
	le *logrus.Entry,
	webRuntimeID string,
	pluginWebWorkerID string,
	shouldRemove func(generation string) bool,
) (map[string]web_worker.WebWorker, error) {
	docWebWorkers, err := doc.GetWebWorkers(ctx)
	if err != nil {
		return nil, err
	}

	docWebWorkers = maps.Clone(docWebWorkers)
	for id, worker := range docWebWorkers {
		if worker.GetId() != pluginWebWorkerID || !shouldRemove(worker.GetGeneration()) {
			delete(docWebWorkers, id)
			continue
		}

		le.
			WithFields(logrus.Fields{
				"web-document": doc.GetWebDocumentUuid(),
				"web-runtime":  webRuntimeID,
				"web-worker":   pluginWebWorkerID,
				"generation":   worker.GetGeneration(),
			}).
			Debug("removing old instance of web worker")
		_, err := worker.Remove(ctx)
		if err != nil {
			le.WithError(err).Warn("unable to remove old web worker instance")
		}
	}

	return docWebWorkers, nil
}

// removeStaleWebWorkerInstances removes workers left by previous executions of
// the plugin. Workers carrying ownGeneration belong to this execution and are
// never removed; every other generation is a stale predecessor under the
// scheduler's single-execution-per-plugin contract, including workers left
// behind when a previous execution crashed without cleanup.
func removeStaleWebWorkerInstances(
	ctx context.Context,
	doc workerListingDoc,
	le *logrus.Entry,
	webRuntimeID string,
	pluginWebWorkerID string,
	ownGeneration string,
) (map[string]web_worker.WebWorker, error) {
	return removeWebWorkerInstances(
		ctx, doc, le, webRuntimeID, pluginWebWorkerID,
		func(generation string) bool { return generation != ownGeneration },
	)
}

// removeOwnWebWorkerInstances removes only the workers this execution created.
// It runs when the execution exits so no instance of its generation survives.
func removeOwnWebWorkerInstances(
	ctx context.Context,
	doc workerListingDoc,
	le *logrus.Entry,
	webRuntimeID string,
	pluginWebWorkerID string,
	ownGeneration string,
) (map[string]web_worker.WebWorker, error) {
	return removeWebWorkerInstances(
		ctx, doc, le, webRuntimeID, pluginWebWorkerID,
		func(generation string) bool { return generation == ownGeneration },
	)
}
