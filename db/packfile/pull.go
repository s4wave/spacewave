package packfile

import (
	"context"

	"github.com/pkg/errors"
)

// CatalogPuller reads one page of a block store catalog after a cursor.
type CatalogPuller interface {
	// SyncPull returns the catalog page after since. Since zero starts from the
	// beginning.
	SyncPull(ctx context.Context, resourceID string, since uint64) (*PullResponse, error)
}

// NextCursor returns the cursor that continues a pull after this page: the
// greatest sequence on the page, or zero when the page is empty.
func (r *PullResponse) NextCursor() uint64 {
	var next uint64
	for _, entry := range r.GetEntries() {
		next = max(next, entry.GetSequence())
	}
	for _, event := range r.GetReplacementEvents() {
		next = max(next, event.GetSequence())
	}
	return next
}

// PullCatalog reads every page of a block store catalog from the beginning and
// returns the pages merged into one response. A restarted page discards the
// pages read before it.
func PullCatalog(ctx context.Context, puller CatalogPuller, resourceID string) (*PullResponse, error) {
	catalog := &PullResponse{}
	var since uint64
	for {
		page, err := puller.SyncPull(ctx, resourceID, since)
		if err != nil {
			return nil, err
		}
		if page.GetRestart() {
			catalog = &PullResponse{}
		}
		catalog.Entries = append(catalog.Entries, page.GetEntries()...)
		catalog.ReplacementEvents = append(catalog.ReplacementEvents, page.GetReplacementEvents()...)
		catalog.LatestSequence = page.GetLatestSequence()
		if !page.GetMore() {
			return catalog, nil
		}
		next := page.NextCursor()
		if next <= since && !page.GetRestart() {
			return nil, errors.Errorf("catalog page after %d did not advance", since)
		}
		since = next
	}
}
