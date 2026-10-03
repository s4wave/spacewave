package block_store_http_server

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_http "github.com/s4wave/spacewave/db/block/store/http"
	"github.com/s4wave/spacewave/net/hash"
)

// HTTPBlockServer is HTTP server serving a BlockStore.
type HTTPBlockServer struct {
	// store is the store to read from
	store block.StoreOps
	// write enables write ops
	write bool
	// pathPrefix is the path prefix to use for requests.
	pathPrefix string
	// forceHashType forces using the given hash type.
	// returns an error if any request forces a different hash type.
	// if 0 uses the default specified by the store
	forceHashType hash.HashType
}

// NewHTTPBlockServer builds a new block store server on top of a block store.
//
// if write=false, supports read operations only.
// pathPrefix is the URL path prefix for block store ops.
// forceHashType can be 0 to use the default hash type.
// if forceHashType is set, returns an error if the client attempts to force a different hash type.
func NewHTTPBlock(store block.StoreOps, write bool, pathPrefix string, forceHashType hash.HashType) *HTTPBlockServer {
	return &HTTPBlockServer{
		store:         store,
		write:         write,
		pathPrefix:    pathPrefix,
		forceHashType: forceHashType,
	}
}

// ServeHTTP serves the HTTP server at pathPrefix.
func (h *HTTPBlockServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Match the request URL against the block-store path prefix.
	reqURL := req.URL
	if reqURL == nil {
		return
	}
	reqPath := path.Clean(reqURL.Path)
	if !strings.HasPrefix(reqPath, h.pathPrefix) {
		return
	}

	// Extract the block-store operation path and require a nonempty operation.
	opPath := strings.TrimPrefix(reqPath, h.pathPrefix)
	if len(opPath) != 0 && opPath[0] == '/' {
		opPath = opPath[1:]
	}
	if len(opPath) == 0 {
		rw.WriteHeader(404)
		return
	}

	// Prepare block-reference parsing and HTTP method validation for dispatch.
	pathPts := strings.Split(opPath, "/")
	parseRef := func(refStr string) *block.BlockRef {
		// Parse and validate the requested block reference.
		ref := &block.BlockRef{}
		err := ref.ParseFromB58(refStr)
		if err == nil {
			// expect a non-nil ref
			err = ref.Validate(false)
		}

		// Return a bad-request response for an invalid block reference.
		if err != nil {
			rw.WriteHeader(400)
			_, _ = rw.Write([]byte("invalid block ref: "))
			_, _ = rw.Write([]byte(err.Error())) //nolint:gosec
			_, _ = rw.Write([]byte("\n"))
			return nil
		}
		return ref
	}
	checkMethod := func(expected string) bool {
		if req.Method != expected {
			rw.WriteHeader(405)
			_, _ = rw.Write([]byte("method not allowed: " + req.Method)) //nolint:gosec
			return false
		}
		return true
	}

	// Dispatch the block-store operation to its HTTP handler.
	switch pathPts[0] {
	case block_store_http.GetPath:
		if !checkMethod("GET") {
			return
		}
		if len(pathPts) != 2 {
			rw.WriteHeader(404)
			_, _ = rw.Write([]byte("not found"))
			return
		}
		ref := parseRef(pathPts[1])
		if ref == nil {
			return
		}
		withRefs := req.URL.Query().Has(block_store_http.RefsQuery)
		h.ServeGetBlock(req.Context(), rw, ref, withRefs)
		return
	case block_store_http.ExistsPath:
		if !checkMethod("GET") {
			return
		}
		if len(pathPts) != 2 {
			rw.WriteHeader(404)
			return
		}
		ref := parseRef(pathPts[1])
		if ref == nil {
			return
		}
		h.ServeGetBlockExists(req.Context(), rw, ref)
		return
	case block_store_http.RmPath:
		if !checkMethod("DELETE") {
			return
		}
		if len(pathPts) != 2 {
			rw.WriteHeader(404)
			return
		}
		ref := parseRef(pathPts[1])
		if ref == nil {
			return
		}
		h.ServeRmBlock(req.Context(), rw, ref)
		return
	case block_store_http.PutPath:
		h.ServePutBlock(req.Context(), rw, req.Body)
		return
	default:
		rw.WriteHeader(404)
		return
	}
}

// ServeGetBlock serves a get block request, with the block's refs when
// withRefs is set. ref must have been validated already.
func (h *HTTPBlockServer) ServeGetBlock(ctx context.Context, rw http.ResponseWriter, ref *block.BlockRef, withRefs bool) {
	stored, err := block.ReadStoredBlock(ctx, h.store, ref, withRefs)
	resp := &block_store_http.GetResponse{}
	switch {
	case err != nil:
		resp.Err = err.Error()
	case stored != nil && len(stored.Data) != 0:
		resp.Data = stored.Data
		resp.Refs = stored.Refs
		resp.RefsKnown = stored.RefsKnown
	default:
		resp.NotFound = true
	}

	h.writeResponse(rw, resp, resp.GetNotFound())
}

// ServeGetBlockExists serves a get block exists request.
// ref must have been validated already.
func (h *HTTPBlockServer) ServeGetBlockExists(ctx context.Context, rw http.ResponseWriter, ref *block.BlockRef) {
	exists, err := h.store.GetBlockExists(ctx, ref)
	resp := &block_store_http.ExistsResponse{}
	if err != nil {
		resp.Err = err.Error()
	} else if exists {
		resp.Exists = true
	} else {
		resp.NotFound = true
	}

	h.writeResponse(rw, resp, resp.GetNotFound())
}

// ServeRmBlock serves a rm block request.
// ref must have been validated already.
func (h *HTTPBlockServer) ServeRmBlock(ctx context.Context, rw http.ResponseWriter, ref *block.BlockRef) {
	// Reject block removal when the server is read-only.
	if !h.write {
		rw.WriteHeader(401)
		_, _ = rw.Write([]byte(block_store.ErrReadOnly.Error() + "\n"))
		return
	}

	// Remove the block and encode the store result.
	err := h.store.RmBlock(ctx, ref)
	resp := &block_store_http.RmResponse{}
	if err != nil {
		resp.Err = err.Error()
	} else {
		resp.Removed = true
	}

	// Send the block-removal response.
	h.writeResponse(rw, resp, false)
}

// ServePutBlock serves a put block request.
// ref must have been validated already.
func (h *HTTPBlockServer) ServePutBlock(ctx context.Context, rw http.ResponseWriter, reqBody io.ReadCloser) {
	// Reject block writes when the server is read-only.
	if !h.write {
		_ = reqBody.Close()
		rw.WriteHeader(401)
		_, _ = rw.Write([]byte(block_store.ErrReadOnly.Error() + "\n"))
		return
	}

	// Read the encoded block-write request body.
	bodyData, err := io.ReadAll(reqBody)
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte(err.Error() + "\n"))
		return
	}

	// Decode and validate the block-write request.
	req := &block_store_http.PutRequest{}
	err = req.UnmarshalVT(bodyData)
	if err == nil {
		err = req.Validate()
	}
	if err != nil {
		rw.WriteHeader(400)
		_, _ = rw.Write([]byte(err.Error() + "\n"))
		return
	}

	// Select the block hash and enforce the server hash requirement.
	putOpts := req.GetPutOpts()
	if putOpts == nil {
		putOpts = &block.PutOpts{}
	}
	reqHashType := putOpts.SelectHashType(h.forceHashType)
	if h.forceHashType != 0 && reqHashType != h.forceHashType {
		rw.WriteHeader(400)
		_, _ = rw.Write([]byte("cannot write block using "))
		_, _ = rw.Write([]byte(reqHashType.String())) //nolint:gosec
		_, _ = rw.Write([]byte(": service requires "))
		_, _ = rw.Write([]byte(h.forceHashType.String())) //nolint:gosec
		_, _ = rw.Write([]byte("\n"))
		return
	}

	// Write the block with the selected hash and encode the store result.
	putOpts.HashType = reqHashType
	putRef, existed, err := h.store.PutBlock(ctx, req.GetData(), putOpts)
	resp := &block_store_http.PutResponse{}
	if err != nil {
		resp.Err = err.Error()
	} else {
		resp.Exists = existed
		resp.Ref = putRef
	}

	// Send the block-write response.
	h.writeResponse(rw, resp, false)
}

// writeResponse writes a response message.
func (h *HTTPBlockServer) writeResponse(rw http.ResponseWriter, msg block.Block, notFound bool) {
	// Encode the block-store response or report an encoding failure.
	respData, err := msg.MarshalBlock()
	if err != nil {
		rw.WriteHeader(500)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}

	// Send the encoded block-store response with its presence status.
	rw.Header().Set("content-type", "application/vnd.google.protobuf")
	if !notFound {
		rw.WriteHeader(200)
	} else {
		rw.WriteHeader(404)
	}
	_, _ = rw.Write(respData)
}

// _ is a type assertion
var _ http.Handler = (*HTTPBlockServer)(nil)
