package cdn_writer

import (
	"io"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport/reqsig"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/peer"
)

// MaxRequestBody bounds a request body: a push request with its Bloom
// filter, or a checkpoint.
const MaxRequestBody = 16 << 20

// Refusal codes beside the session API's own.
const (
	// CheckpointRefusedCode refuses a checkpoint that does not follow the
	// recorded one or lacks its owners' signatures.
	CheckpointRefusedCode = "checkpoint_refused"
	// PackNotAdmittedCode refuses a commit of a pack no live upload admitted.
	PackNotAdmittedCode = "pack_not_admitted"
	// PackMismatchCode refuses a commit whose stored pack differs from the
	// pushed one.
	PackMismatchCode = "pack_mismatch"
)

// routes registers the session API routes the publisher speaks.
func (w *Writer) routes() {
	// Route each call under the base path.
	base := w.cfg.BasePath
	w.mux.HandleFunc("GET "+base+"/api/sobject/{space}/state", w.serveState)
	w.mux.HandleFunc("POST "+base+"/api/sobject/{space}/checkpoint", w.serveCheckpoint)
	w.mux.HandleFunc("GET "+base+"/api/bstore/{space}/sync/pull", w.servePull)
	w.mux.HandleFunc("POST "+base+"/api/bstore/{space}/sync/push", w.servePush)
	w.mux.HandleFunc("POST "+base+"/api/bstore/{space}/sync/push/{pack}/commit", w.serveCommit)
}

// ServeHTTP serves the session API. Mount it without stripping a path
// prefix: the signature covers the path as the writer sent it.
func (w *Writer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.mux.ServeHTTP(rw, r)
}

// serveState returns the Space's state as its checkpoint and config: a CDN
// Space holds no operations above its checkpoint.
func (w *Writer) serveState(rw http.ResponseWriter, r *http.Request) {
	// Authorize, then read the in-memory pointer.
	spaceID, _, ok := w.authorize(rw, r, nil)
	if !ok {
		return
	}
	root := w.Root(spaceID)

	// Describe the checkpoint and its key epoch.
	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConfigChainHash:  root.GetConfigChainHash(),
			ConfigChainSeqno: root.GetConfigChainSeqno(),
		},
		Checkpoint: root.GetCheckpoint(),
	}
	if root.GetCheckpoint() != nil {
		inner, err := root.GetCheckpoint().UnmarshalInner()
		if err != nil {
			writeResult(rw, nil, err)
			return
		}
		if epoch := inner.GetKeyEpoch(); epoch != 0 {
			state.KeyEpochs = []*sobject.SOKeyEpoch{{Epoch: epoch}}
		}
	}
	writeMessage(rw, &api.SOStateMessage{Content: &api.SOStateMessage_Snapshot{Snapshot: state}}, nil)
}

// servePull lists the Space's packs committed after the since cursor, in
// one page.
func (w *Writer) servePull(rw http.ResponseWriter, r *http.Request) {
	// Authorize and read the cursor.
	spaceID, _, ok := w.authorize(rw, r, nil)
	if !ok {
		return
	}
	var since uint64
	if v := r.URL.Query().Get("since"); v != "" {
		var err error
		if since, err = strconv.ParseUint(v, 10, 64); err != nil {
			writeError(rw, http.StatusBadRequest, "bad_request", "since is not a number")
			return
		}
	}

	// List the entries after it from the in-memory pointer.
	resp := &packfile.PullResponse{}
	for _, pack := range w.Root(spaceID).GetPacks() {
		resp.LatestSequence = max(resp.LatestSequence, pack.GetSequence())
		if pack.GetSequence() > since {
			resp.Entries = append(resp.Entries, pack)
		}
	}
	writeMessage(rw, resp, nil)
}

// servePush grants an upload of a pack.
func (w *Writer) servePush(rw http.ResponseWriter, r *http.Request) {
	// Authorize and decode the push, then admit its pack.
	req := &packfile.PushRequest{}
	spaceID, _, ok := w.authorize(rw, r, req)
	if !ok {
		return
	}
	resp, err := w.admitPack(spaceID, req, w.now())
	writeMessage(rw, resp, err)
}

// serveCommit adds an uploaded pack to the Space.
func (w *Writer) serveCommit(rw http.ResponseWriter, r *http.Request) {
	spaceID, _, ok := w.authorize(rw, r, nil)
	if !ok {
		return
	}
	err := w.commitPack(r.Context(), spaceID, r.PathValue("pack"), w.now())
	writeMessage(rw, &packfile.PushResponse{PackId: r.PathValue("pack")}, err)
}

// serveCheckpoint moves the Space to the posted checkpoint.
func (w *Writer) serveCheckpoint(rw http.ResponseWriter, r *http.Request) {
	// Authorize and decode the checkpoint, then commit it.
	req := &api.PostCheckpointRequest{}
	spaceID, signer, ok := w.authorize(rw, r, req)
	if !ok {
		return
	}
	if req.GetCheckpoint() == nil {
		writeError(rw, http.StatusBadRequest, "bad_request", "a checkpoint is required")
		return
	}
	err := w.postCheckpoint(r.Context(), spaceID, signer, req.GetCheckpoint())
	writeMessage(rw, &api.SubmitCheckpointResponse{}, err)
}

// authorize reads the request body into msg when set, and returns the Space
// and the signer when one of the Space's writers signed the request.
// Otherwise it writes the refusal.
func (w *Writer) authorize(rw http.ResponseWriter, r *http.Request, msg interface{ UnmarshalVT([]byte) error }) (string, peer.ID, bool) {
	// Read the body within the limit.
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBody+1))
	if err != nil {
		writeError(rw, http.StatusBadRequest, "bad_request", "read body")
		return "", "", false
	}
	if len(body) > MaxRequestBody {
		writeError(rw, http.StatusRequestEntityTooLarge, "bad_request", "body too large")
		return "", "", false
	}

	// Require a signature by one of the Space's writers.
	spaceID := r.PathValue("space")
	if w.Root(spaceID) == nil {
		writeError(rw, http.StatusNotFound, "not_found", "this writer takes no publishes to this Space")
		return "", "", false
	}
	now := w.now()
	signer, err := reqsig.Verify(r, body, w.cfg.SigningEnv, now)
	if err != nil {
		writeError(rw, http.StatusUnauthorized, "invalid_signature", err.Error())
		return "", "", false
	}
	if !w.mayWrite(spaceID, signer, now) {
		writeError(rw, http.StatusForbidden, "forbidden", "this signer does not write this Space")
		return "", "", false
	}

	// Decode the request.
	if msg != nil {
		if err := msg.UnmarshalVT(body); err != nil {
			writeError(rw, http.StatusBadRequest, "bad_request", "decode request")
			return "", "", false
		}
	}
	return spaceID, signer, true
}

// writeMessage writes resp, or the refusal err names.
func writeMessage(rw http.ResponseWriter, resp interface{ MarshalVT() ([]byte, error) }, err error) {
	var data []byte
	if err == nil {
		data, err = resp.MarshalVT()
	}
	writeResult(rw, data, err)
}

// writeResult writes data, or the refusal err names.
func writeResult(rw http.ResponseWriter, data []byte, err error) {
	switch {
	case err == nil:
		rw.Header().Set("Content-Type", "application/octet-stream")
		_, _ = rw.Write(data) //nolint:gosec // G705: a binary protobuf body served as application/octet-stream.
	case errors.Is(err, ErrPackReplaced):
		writeError(rw, http.StatusConflict, clouderror.PackReplacementConflictCode, err.Error())
	case errors.Is(err, ErrCheckpointStale), errors.Is(err, ErrCheckpointSigner):
		writeError(rw, http.StatusConflict, CheckpointRefusedCode, err.Error())
	case errors.Is(err, ErrPackNotAdmitted):
		writeError(rw, http.StatusConflict, PackNotAdmittedCode, err.Error())
	case errors.Is(err, ErrPackMismatch):
		writeError(rw, http.StatusConflict, PackMismatchCode, err.Error())
	case errors.Is(err, ErrSpaceUnknown):
		writeError(rw, http.StatusServiceUnavailable, "space_unavailable", err.Error())
	default:
		writeError(rw, http.StatusInternalServerError, "internal", err.Error())
	}
}

// writeError writes a refusal as the session client reads it.
func writeError(rw http.ResponseWriter, status int, code, message string) {
	data, _ := (&api.ErrorResponse{Code: code, Message: message}).MarshalJSON()
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_, _ = rw.Write(data)
}
