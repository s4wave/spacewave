//go:build js && !bldr_indexeddb

package web_runtime_sqlite

import (
	"context"
	"syscall/js"
	"time"

	web_document "github.com/aperturerobotics/bldr/web/document"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	web_worker "github.com/aperturerobotics/bldr/web/worker"
	sqlite_wasm "github.com/aperturerobotics/hydra/sql/sqlite-wasm"
	sqlite_wasm_rpc "github.com/aperturerobotics/hydra/sql/sqlite-wasm/rpc"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// sqliteWorkerID is the worker identifier used for CreateWebWorker.
const sqliteWorkerID = "sqlite-worker"

const sqliteWorkerStopTimeout = 5 * time.Second

// StartSqliteWorker keeps one sqlite dedicated worker running at a time. It
// waits for a visible WebDocument, spawns the worker there, establishes starpc
// RPC, and calls SetClient on the hydra sql/sqlite-wasm driver. If the hosting
// document is closed, the worker is torn down and respawned in the next
// available document. Hidden documents remain eligible to continue hosting an
// already-running worker so sqlite stays available when all tabs are hidden.
func StartSqliteWorker(ctx context.Context, le *logrus.Entry, rt web_runtime.WebRuntime) error {
	workerURL := js.Global().Get("BLDR_SQLITE_WORKER_URL")
	if workerURL.IsUndefined() || workerURL.IsNull() || workerURL.String() == "" {
		le.Warn("BLDR_SQLITE_WORKER_URL not set, skipping sqlite worker")
		return nil
	}
	path := workerURL.String()
	le.WithField("url", path).Info("starting sqlite dedicated worker")

	for {
		doc, err := waitAvailableWebDocument(ctx, rt)
		if err != nil {
			return err
		}
		docLog := le.WithField("doc", doc.GetWebDocumentUuid())
		docLog.Debug("using WebDocument for sqlite worker")

		worker, err := doc.CreateWebWorker(ctx, &web_document.CreateWebWorkerRequest{
			Id:         sqliteWorkerID,
			Path:       path,
			WorkerMode: web_document.WebWorkerMode_WORKER_MODE_DEDICATED,
		})
		if err != nil {
			return err
		}
		if worker == nil {
			docLog.Debug("sqlite worker was not created, waiting for WebDocument state change")
			if err := waitForDocumentClose(ctx, doc); err != nil {
				return err
			}
			continue
		}

		docLog.Info("sqlite worker created")
		client := srpc.NewClient(rt.GetWebWorkerOpenStream(sqliteWorkerID))
		sqlite_wasm.SetClient(sqlite_wasm_rpc.NewSRPCSqliteBridgeClient(client))
		docLog.Info("sqlite worker RPC client set")

		waitErr := waitForDocumentClose(ctx, doc)

		sqlite_wasm.SetClient(nil)
		docLog.Info("sqlite worker RPC client cleared")
		if stopErr := removeWorker(worker, docLog); stopErr != nil {
			docLog.WithError(stopErr).Warn("failed to remove sqlite worker")
		}

		if waitErr != nil {
			return waitErr
		}
		docLog.Info("sqlite worker host document closed, respawning")
	}
}

func waitAvailableWebDocument(ctx context.Context, rt web_runtime.WebRuntime) (web_document.WebDocument, error) {
	for {
		docs, err := rt.GetWebDocuments(ctx)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			status := doc.GetWebDocumentStatusCtr().GetValue()
			if status == nil || status.GetClosed() || status.GetHidden() {
				continue
			}
			return doc, nil
		}

		if len(docs) == 0 {
			if _, err := rt.WaitFirstWebDocument(ctx); err != nil {
				return nil, err
			}
			continue
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func waitForDocumentClose(ctx context.Context, doc web_document.WebDocument) error {
	_, err := doc.GetWebDocumentStatusCtr().WaitValueWithValidator(ctx, func(status *web_document.WebDocumentStatus) (bool, error) {
		return status == nil || status.GetClosed(), nil
	}, nil)
	return err
}

func removeWorker(worker web_worker.WebWorker, le *logrus.Entry) error {
	if worker == nil {
		return nil
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), sqliteWorkerStopTimeout)
	defer stopCancel()
	removed, err := worker.Remove(stopCtx)
	if err != nil && err != context.Canceled {
		return err
	}
	le.WithField("removed", removed).Debug("sqlite worker removed")
	return nil
}
