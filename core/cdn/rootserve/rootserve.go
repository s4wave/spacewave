// Package cdn_rootserve serves CDN Space root pointers and the cache rule of
// their packs. A pointer changes on every commit, so it is served with
// no-cache and a strong ETag, and a reader polling it gets 304 until it moves.
// A pack's ID names its content, so it is cached for a year.
package cdn_rootserve

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/net/httpclient"
)

// PointerName is the object name of a Space's root pointer under its ID.
const PointerName = "root.packedmsg"

// PointerCacheControl makes every reader revalidate the pointer, so a commit
// reaches a reader on its next poll.
const PointerCacheControl = "no-cache"

// PackCacheControl caches a pack for a year: its ID names its bytes.
const PackCacheControl = "public, max-age=31536000, immutable"

// originTimeout bounds one conditional fetch of a foreign pointer.
const originTimeout = 10 * time.Second

// revalidateAfter is how long a held foreign pointer is served before the
// origin is asked again, so readers polling one Space cost the origin at most
// one request a second between them.
const revalidateAfter = time.Second

// maxSpaceIDLen bounds the Space ID a request may name.
const maxSpaceIDLen = 64

// ErrPointerNotFound reports a Space that has no pointer here or at the
// origin.
var ErrPointerNotFound = errors.New("cdn root pointer not found")

// Pointer is the encoded root pointer of one Space as readers get it.
type Pointer struct {
	// ETag is the strong entity tag of Body.
	ETag string
	// Body is the packedmsg encoding of the pointer.
	Body []byte
}

// EncodePointer encodes ptr as readers fetch it.
func EncodePointer(ptr *cdn.CdnRootPointer) (Pointer, error) {
	// Encode the pointer of a named Space within the reader's size limit.
	if ptr.GetSpaceId() == "" {
		return Pointer{}, errors.New("cdn root pointer has no space id")
	}
	data, err := ptr.MarshalVT()
	if err != nil {
		return Pointer{}, err
	}
	body := packedmsg.AppendPackedMessage(nil, data)
	if int64(len(body)) > cdn_bstore.MaxRootPackedmsgBytes {
		return Pointer{}, errors.Errorf("cdn root pointer exceeds %d bytes", cdn_bstore.MaxRootPackedmsgBytes)
	}
	return Pointer{ETag: pointerETag(body), Body: body}, nil
}

// pointerETag returns the strong entity tag of an encoded pointer.
func pointerETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// Handler serves root pointers at /<space>/root.packedmsg from two sources:
// the Spaces this server writes, and copies of foreign Spaces fetched from
// an origin. Mount it with http.StripPrefix under the pointer base URL that
// readers pass as rootPointerBaseUrl.
type Handler struct {
	// origin is the base URL of foreign pointers, or empty to serve only
	// owned Spaces.
	origin string
	// client fetches foreign pointers.
	client *http.Client

	// mtx guards owned and held.
	mtx sync.Mutex
	// owned is the pointer of each Space this server writes.
	owned map[string]Pointer
	// held is the last good copy of each foreign pointer.
	held map[string]heldPointer
	// now reads the clock.
	now func() time.Time
}

// heldPointer is a foreign pointer and when the origin last vouched for it.
type heldPointer struct {
	Pointer
	// checked is when the origin last served or confirmed it.
	checked time.Time
}

// NewHandler returns a handler serving owned Spaces, and foreign Spaces
// from origin when it is set. A nil client uses one with a 10 second timeout.
func NewHandler(origin string, client *http.Client) *Handler {
	if client == nil {
		client = &http.Client{Timeout: originTimeout}
	}
	return &Handler{
		origin: strings.TrimRight(origin, "/"),
		client: client,
		owned:  make(map[string]Pointer),
		held:   make(map[string]heldPointer),
		now:    time.Now,
	}
}

// SetOwned serves ptr for its Space from now on. Call it after the commit
// that produced ptr is durable, so no reader sees a pointer that a restart
// would lose.
func (h *Handler) SetOwned(ptr *cdn.CdnRootPointer) error {
	// Encode the pointer once, then serve the encoding to every reader.
	enc, err := EncodePointer(ptr)
	if err != nil {
		return err
	}
	h.mtx.Lock()
	h.owned[ptr.GetSpaceId()] = enc
	h.mtx.Unlock()
	return nil
}

// DropOwned stops serving spaceID as an owned Space.
func (h *Handler) DropOwned(spaceID string) {
	h.mtx.Lock()
	delete(h.owned, spaceID)
	h.mtx.Unlock()
}

// ServeHTTP answers GET and HEAD of /<space>/root.packedmsg.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Accept only reads of a well-formed pointer path.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	spaceID, ok := parsePointerPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Find the pointer; a foreign pointer whose origin failed keeps its last
	// copy.
	ptr, err := h.Pointer(r.Context(), spaceID)
	if errors.Is(err, ErrPointerNotFound) {
		http.NotFound(w, r)
		return
	}
	if ptr.Body == nil {
		http.Error(w, "cdn root pointer origin unavailable", http.StatusBadGateway)
		return
	}

	// Serve it for revalidation; ServeContent answers If-None-Match with 304.
	hdr := w.Header()
	hdr.Set("Content-Type", "application/octet-stream")
	hdr.Set("Cache-Control", PointerCacheControl)
	hdr.Set("ETag", ptr.ETag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(ptr.Body))
}

// Pointer returns the pointer of spaceID: the owned one, or else the
// origin's. A held copy the origin confirmed within the last second is served
// as is; an older one is revalidated conditionally. When the origin fails it
// returns the held copy, if any, with the error.
func (h *Handler) Pointer(ctx context.Context, spaceID string) (Pointer, error) {
	// Serve an owned Space without asking the origin.
	h.mtx.Lock()
	ptr, owned := h.owned[spaceID]
	held := h.held[spaceID]
	h.mtx.Unlock()
	if owned {
		return ptr, nil
	}
	if h.origin == "" {
		return Pointer{}, ErrPointerNotFound
	}
	if held.Body != nil && h.now().Sub(held.checked) < revalidateAfter {
		return held.Pointer, nil
	}

	// Revalidate the held copy at the origin and keep what it answers.
	fetched, err := h.fetch(ctx, spaceID, held.Pointer)
	h.mtx.Lock()
	defer h.mtx.Unlock()
	switch {
	case errors.Is(err, ErrPointerNotFound):
		delete(h.held, spaceID)
	case err == nil:
		h.held[spaceID] = heldPointer{Pointer: fetched, checked: h.now()}
		return fetched, nil
	}
	return h.held[spaceID].Pointer, err
}

// fetch asks the origin for spaceID's pointer, conditional on held.
func (h *Handler) fetch(ctx context.Context, spaceID string, held Pointer) (Pointer, error) {
	// Ask for the pointer unless it still matches the held copy.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.origin+"/"+spaceID+"/"+PointerName, nil) //nolint:gosec // the origin is configuration and spaceID passed validSpaceID.
	if err != nil {
		return Pointer{}, err
	}
	if held.ETag != "" {
		req.Header.Set("If-None-Match", held.ETag)
	}
	resp, err := h.client.Do(req) //nolint:gosec // the request URL is the configured origin.
	if err != nil {
		return Pointer{}, errors.Wrap(err, "fetch cdn root pointer")
	}
	defer httpclient.DrainAndCloseResponseBody(resp)

	// Keep the held copy on 304, drop it on 404, and refuse other failures.
	switch resp.StatusCode {
	case http.StatusNotModified:
		if held.Body == nil {
			return Pointer{}, errors.New("cdn root pointer origin answered 304 with no held copy")
		}
		return held, nil
	case http.StatusNotFound:
		return Pointer{}, ErrPointerNotFound
	case http.StatusOK:
	default:
		return Pointer{}, errors.Errorf("cdn root pointer origin status %d", resp.StatusCode)
	}

	// Read the new pointer within the reader's size limit, tagging it by its
	// bytes so every server gives one body one tag.
	body, err := io.ReadAll(io.LimitReader(resp.Body, cdn_bstore.MaxRootPackedmsgBytes+1))
	if err != nil {
		return Pointer{}, errors.Wrap(err, "read cdn root pointer")
	}
	if int64(len(body)) > cdn_bstore.MaxRootPackedmsgBytes {
		return Pointer{}, errors.Errorf("cdn root pointer exceeds %d bytes", cdn_bstore.MaxRootPackedmsgBytes)
	}
	if _, ok := packedmsg.DecodePackedMessage(string(body)); !ok {
		return Pointer{}, errors.New("cdn root pointer from origin failed packedmsg decode")
	}
	return Pointer{ETag: pointerETag(body), Body: body}, nil
}

// parsePointerPath returns the Space ID of /<space>/root.packedmsg.
func parsePointerPath(path string) (string, bool) {
	spaceID, ok := strings.CutSuffix(strings.TrimPrefix(path, "/"), "/"+PointerName)
	if !ok || !validSpaceID(spaceID) {
		return "", false
	}
	return spaceID, true
}

// validSpaceID reports whether id is a Space ID a pointer path may name:
// lowercase letters, digits, '-' and '_', at most 64 of them.
func validSpaceID(id string) bool {
	if id == "" || len(id) > maxSpaceIDLen {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// ImmutablePacks wraps a handler serving a Space's objects so every pack
// under /packs/ is cached for a year.
func ImmutablePacks(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/packs/") {
			w.Header().Set("Cache-Control", PackCacheControl)
		}
		next.ServeHTTP(w, r)
	})
}
