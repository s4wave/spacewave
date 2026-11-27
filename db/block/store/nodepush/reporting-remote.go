package block_store_nodepush

import (
	"context"

	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// ReportingRemote is the node's upload target: a packahead.Remote that records
// a report for each pack it uploads.
//
// Invariant: a pack counts as uploaded only once its report is durable. A crash
// between the upload and the report leaves HasPackfile false, so the drain
// uploads the same bytes again and reports it, rather than skipping a pack the
// local store would never hear of.
type ReportingRemote struct {
	remote packahead.Remote
	server *Server
}

// NewReportingRemote wraps remote, reporting through server.
func NewReportingRemote(remote packahead.Remote, server *Server) *ReportingRemote {
	return &ReportingRemote{remote: remote, server: server}
}

// HasPackfile reports whether the pack has a durable report, which is written
// only after the remote took the pack. So the node never lists the remote.
func (r *ReportingRemote) HasPackfile(_ context.Context, id string) (bool, error) {
	return r.server.reported(id), nil
}

// PutPackFile uploads a pack, then reports it.
func (r *ReportingRemote) PutPackFile(ctx context.Context, id string, data []byte) (*packfile.PackfileEntry, error) {
	// Upload the pack.
	entry, err := r.remote.PutPackFile(ctx, id, data)
	if err != nil {
		return nil, err
	}

	// Retain read copies and record the uploaded pack for the local store.
	if err := r.server.reportUpload(id, data, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

// _ is a type assertion
var _ packahead.Remote = (*ReportingRemote)(nil)
