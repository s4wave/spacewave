//go:build !tinygo

package block_store_gdrive

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // Drive supplies MD5 as a transfer checksum.
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

// ChunkBytes is Drive's 256 KiB multiple and the multipart cutoff.
const ChunkBytes = 8 << 20

// fileFields limits metadata responses to the fields integrity checks consume.
const fileFields = "id,name,mimeType,size,md5Checksum,appProperties"

// Client is an append-only ObjectStore rooted at one Drive folder. Bucket is
// ignored: a Client's root is its isolation boundary. HTTP must refresh its
// credential; callers reuse a Client per credential so they share its pacer.
// No lock is held across a network request. Concurrent directory lookups join
// the same in-flight listing rather than creating duplicate folders.
type Client struct {
	// http authenticates and executes each request.
	http *http.Client
	// api is the Drive v3 metadata endpoint.
	api string
	// upload is the Drive v3 upload endpoint.
	upload string
	// root is the folder ID, never a user-supplied path.
	root string
	// pacer is shared by all requests with this credential.
	pacer *rate.Limiter
	// mtx guards files and loaded.
	mtx sync.Mutex
	// files caches metadata by complete key.
	files map[string]File
	// folders keeps every same-name folder, so a lost create reply cannot hide packs.
	folders map[string][]File
	// loaded records directories whose children have been listed.
	loaded map[string]bool
	// loading joins concurrent directory initialization.
	loading singleflight.Group
}

// NewClient constructs a lazily connected Drive store. An empty endpoint uses
// Google's service; a custom endpoint is for an offline fake server.
func NewClient(client *http.Client, root, endpoint string) (*Client, error) {
	// Require an explicitly authenticated transport and a root folder.
	if client == nil || root == "" {
		return nil, errors.New("Drive HTTP client and root folder are required")
	}
	if endpoint == "" {
		endpoint = "https://www.googleapis.com"
	}
	return &Client{
		http: client, root: root, api: strings.TrimRight(endpoint, "/") + "/drive/v3/files",
		upload:  strings.TrimRight(endpoint, "/") + "/upload/drive/v3/files",
		pacer:   rate.NewLimiter(rate.Every(100*time.Millisecond), 100),
		files:   map[string]File{"": {ID: root, Folder: true}},
		folders: map[string][]File{"": {{ID: root, Folder: true}}}, loaded: make(map[string]bool),
	}, nil
}

// request retries transient failures with a replayable body. It never reports
// response bodies, which may contain authorization details.
func (c *Client) request(ctx context.Context, method, endpoint string, body []byte, headers http.Header) (*http.Response, error) {
	// Bound retries; the owning backup routine decides when to resume later.
	for attempt := range 8 {
		if err := c.pacer.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if headers != nil {
			req.Header = headers.Clone()
		}
		resp, err := c.http.Do(req)
		if err == nil && resp.StatusCode < 400 {
			return resp, nil
		}

		// Classify quota and authorization outcomes before considering a retry.
		retry := err != nil
		if errors.Is(err, ErrSignIn) {
			return nil, ErrSignIn
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, context.Cause(ctx)
			}
			err = errors.New("Drive transport failed")
		}
		var retryAfter time.Duration
		if resp != nil {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close() // The response is discarded after classification.
			if readErr != nil {
				return nil, readErr
			}
			v, _ := fastjson.ParseBytes(data)
			reason := string(v.GetStringBytes("error", "errors", "0", "reason"))
			message := string(v.GetStringBytes("error", "message"))
			switch {
			case resp.StatusCode == http.StatusNotFound:
				return nil, block_store_s3.ErrNotFound
			case resp.StatusCode == http.StatusUnauthorized:
				return nil, ErrSignIn
			case reason == "storageQuotaExceeded" || reason == "quotaExceeded" || reason == "teamDriveFileLimitExceeded":
				return nil, ErrQuotaFull
			case method != http.MethodGet &&
				(strings.Contains(endpoint, "/upload/drive/") || headers.Get("Content-Range") != "") &&
				strings.Contains(message, "User rate limit exceeded"):
				return nil, ErrUploadLimit
			}
			retry = resp.StatusCode == 429 || resp.StatusCode >= 500 || reason == "rateLimitExceeded" || reason == "userRateLimitExceeded"
			if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil {
				retryAfter = time.Duration(seconds) * time.Second
			}
			err = errors.Errorf("Drive request failed with HTTP %d", resp.StatusCode)
		}
		if !retry || attempt == 7 {
			return nil, err
		}

		// A chunk's reply may be lost after commitment. Its caller waits, then
		// queries the committed offset before sending any more media bytes.
		delay := max(time.Duration(1<<attempt)*100*time.Millisecond, retryAfter)
		if method == http.MethodPut && !strings.HasPrefix(headers.Get("Content-Range"), "bytes */") {
			return nil, &uploadRetry{cause: err, after: delay}
		}
		if err := waitRetry(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("Drive retry exhausted")
}

// list visits one directory a page at a time without collecting the listing.
func (c *Client) list(ctx context.Context, parent string, visit func(File) error) error {
	// Escape Drive's quoted query literal independently from URL encoding.
	escaped := strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(parent)
	q := url.Values{"q": {"'" + escaped + "' in parents and trashed = false"}, "pageSize": {"1000"}, "fields": {"nextPageToken,files(" + fileFields + ")"}}
	for {
		resp, err := c.request(ctx, http.MethodGet, c.api+"?"+q.Encode(), nil, nil)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close() // Fully consumed metadata body.
		if err != nil {
			return err
		}
		// Decode one bounded metadata page and visit its entries.
		v, err := fastjson.ParseBytes(data)
		if err != nil {
			return errors.Wrap(err, "Drive list metadata")
		}
		for _, item := range v.GetArray("files") {
			f, err := ParseFile(item)
			if err != nil {
				return err
			}
			if err := visit(f); err != nil {
				return err
			}
		}
		page := string(v.GetStringBytes("nextPageToken"))
		if page == "" {
			return nil
		}
		q.Set("pageToken", page)
	}
}

// directory resolves a path and lists its direct children once per client.
func (c *Client) directory(ctx context.Context, dir string, create bool) (File, error) {
	// Join an existing initialization without holding a mutex during I/O.
	result := c.loading.DoChan(dir, func() (any, error) {
		// Reuse a completed listing or resolve the parent before creating a segment.
		c.mtx.Lock()
		f, found := c.files[dir]
		loaded := c.loaded[dir]
		c.mtx.Unlock()
		if loaded {
			return f, nil
		}
		if !found {
			parentPath, name := path.Split(dir)
			parent, err := c.directory(ctx, strings.TrimSuffix(parentPath, "/"), create)
			if err != nil {
				return nil, err
			}
			c.mtx.Lock()
			f, found = c.files[dir]
			c.mtx.Unlock()
			if !found {
				if !create {
					return nil, block_store_s3.ErrNotFound
				}
				f, err = c.createFolder(ctx, parent.ID, name)
				if err != nil {
					return nil, err
				}

				// A retried create may have committed an earlier same-name folder.
				c.mtx.Lock()
				parents := slices.Clone(c.folders[strings.TrimSuffix(parentPath, "/")])
				c.mtx.Unlock()
				folders, err := c.findFolders(ctx, parents, name, f)
				if err != nil {
					return nil, err
				}
				c.mtx.Lock()
				c.folders[dir] = folders
				c.mtx.Unlock()
			}
		}
		if !f.Folder {
			return nil, errors.New("Drive path segment is not a folder")
		}

		// Publish the cache only after the full listing succeeds.
		children := make(map[string]File)
		childFolders := make(map[string][]File)
		c.mtx.Lock()
		folders := slices.Clone(c.folders[dir])
		c.mtx.Unlock()

		// Union duplicate folders, accepting one verified file for each logical key.
		for _, folder := range folders {
			if folder.ID < f.ID {
				f = folder
			}
			if err := c.list(ctx, folder.ID, func(child File) error {
				// Keep every folder and choose the lowest verified file identity.
				key := path.Join(dir, child.Name)
				old, ok := children[key]
				if child.Folder {
					childFolders[key] = append(childFolders[key], child)
				}
				if child.Folder || child.Verified() && child.Key == key {
					if !ok || child.ID < old.ID {
						children[key] = child
					}
				}
				return nil
			}); err != nil {
				return nil, err
			}
		}

		// Publish the complete direct-child cache in one short critical section.
		c.mtx.Lock()
		for key, nodes := range childFolders {
			c.folders[key] = nodes
		}
		for key, child := range children {
			c.files[key] = child
		}
		c.files[dir], c.loaded[dir] = f, true
		c.mtx.Unlock()
		return f, nil
	})
	select {
	case <-ctx.Done():
		return File{}, context.Cause(ctx)
	case r := <-result:
		if r.Err != nil {
			return File{}, r.Err
		}
		return r.Val.(File), nil
	}
}

// findFolders recovers every duplicate after an ambiguous create. Reads treat
// them as one directory; an upload may choose any one without stranding bytes.
func (c *Client) findFolders(ctx context.Context, parents []File, name string, created File) ([]File, error) {
	// Keep the successful response even if a listing has not caught up yet.
	folders := []File{created}
	seen := map[string]struct{}{created.ID: {}}
	for _, parent := range parents {
		if err := c.list(ctx, parent.ID, func(folder File) error {
			_, found := seen[folder.ID]
			if folder.Folder && folder.Name == name && !found {
				folders = append(folders, folder)
				seen[folder.ID] = struct{}{}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return folders, nil
}

// createFolder creates one segment; no request ever updates or deletes it.
func (c *Client) createFolder(ctx context.Context, parent, name string) (File, error) {
	// Decode only a successful create response.
	resp, err := c.request(ctx, http.MethodPost, c.api+"?fields="+url.QueryEscape(fileFields),
		metadata(name, parent, "", "", "", 0, true), http.Header{"Content-Type": {"application/json"}})
	if err != nil {
		return File{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return File{}, err
	}
	v, err := fastjson.ParseBytes(data)
	if err != nil {
		return File{}, err
	}
	return ParseFile(v)
}

// CheckRootFolder verifies that this credential can actually see the selected
// folder. A parent query alone can return an empty list for an inaccessible ID.
func (c *Client) CheckRootFolder(ctx context.Context) error {
	// Read metadata only; connectivity checks never create or delete a probe.
	resp, err := c.request(ctx, http.MethodGet, c.api+"/"+url.PathEscape(c.root)+"?fields=id,mimeType", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	// Decode just enough metadata to prove the selected object is a folder.
	value, err := fastjson.ParseBytes(data)
	if err != nil {
		return err
	}
	if string(value.GetStringBytes("id")) == "" || string(value.GetStringBytes("mimeType")) != folderType {
		return errors.New("Drive backup destination is not an accessible folder")
	}
	return nil
}

// PutObject adds a complete immutable file, never replacing a different value.
func (c *Client) PutObject(ctx context.Context, _ string, key string, data []byte, contentType string) error {
	// An omitted media type has the same binary meaning as a PackStore upload.
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Require a canonical key inside this backup's folder.
	if key == "" || path.Clean(key) != key || strings.HasPrefix(key, "/") || key == ".." || strings.HasPrefix(key, "../") {
		return errors.New("Drive object key must be a relative canonical path")
	}

	// Resolve the parent before comparing an existing immutable object.
	dir, name := path.Split(key)
	parent, err := c.directory(ctx, strings.TrimSuffix(dir, "/"), true)
	if err != nil {
		return err
	}

	// Compare content digests with any existing immutable object.
	md5sum, sha256sum := md5.Sum(data), sha256.Sum256(data) //nolint:gosec // SHA-256 identifies the immutable content.
	md5str, sha256str := hex.EncodeToString(md5sum[:]), hex.EncodeToString(sha256sum[:])
	c.mtx.Lock()
	old, exists := c.files[key]
	c.mtx.Unlock()
	if exists {
		if old.Verified() && old.MD5 == md5str && old.SHA256 == sha256str && old.Size == int64(len(data)) {
			return nil
		}
		return ErrImmutable
	}

	// Choose the upload protocol from the fixed 8 MiB cutoff.
	meta := metadata(name, parent.ID, key, md5str, sha256str, len(data), false)
	var f File
	if len(data) <= ChunkBytes {
		f, err = c.multipart(ctx, meta, data, contentType)
	} else {
		f, err = c.resumable(ctx, meta, data, contentType)
	}
	if err != nil {
		return err
	}
	if f.ID == "" || f.Size != int64(len(data)) || f.MD5 != md5str {
		return errors.New("Drive uploaded object checksum or size mismatch")
	}

	// Cache the verified response; proofs may now record this durable object.
	f.Key, f.SHA256, f.ExpectedMD5, f.ExpectedSize = key, sha256str, md5str, strconv.Itoa(len(data))
	c.mtx.Lock()
	c.files[key] = f
	c.mtx.Unlock()
	return nil
}

// multipart uploads a small object in one atomic create.
func (c *Client) multipart(ctx context.Context, meta, data []byte, contentType string) (File, error) {
	// Build the related metadata and binary parts.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range []struct {
		typ  string
		data []byte
	}{{"application/json", meta}, {contentType, data}} {
		h := textproto.MIMEHeader{"Content-Type": {part.typ}}
		out, err := writer.CreatePart(h)
		if err != nil {
			return File{}, err
		}
		if _, err := out.Write(part.data); err != nil {
			return File{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return File{}, err
	}
	return c.uploadFile(ctx, http.MethodPost,
		c.upload+"?uploadType=multipart&fields="+url.QueryEscape(fileFields), body.Bytes(),
		http.Header{"Content-Type": {"multipart/related; boundary=" + writer.Boundary()}})
}

// uploadFile decodes a completed upload response.
func (c *Client) uploadFile(ctx context.Context, method, endpoint string, body []byte, headers http.Header) (File, error) {
	// Require complete metadata, never a partial response.
	resp, err := c.request(ctx, method, endpoint, body, headers)
	if err != nil {
		return File{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return File{}, err
	}
	v, err := fastjson.ParseBytes(data)
	if err != nil {
		return File{}, err
	}
	return ParseFile(v)
}

// resumable sends aligned chunks. After an ambiguous response it queries the
// committed offset instead of repeating a chunk that may already be durable.
// A forgotten session starts a new immutable create; a process restart may
// leave an orphan, but a partial file never appears in a listing.
func (c *Client) resumable(ctx context.Context, meta, data []byte, contentType string) (File, error) {
	// Retry only forgotten sessions here; quota and authorization settle at once.
	for range 3 {
		resp, err := c.request(ctx, http.MethodPost,
			c.upload+"?uploadType=resumable&fields="+url.QueryEscape(fileFields), meta,
			http.Header{"Content-Type": {"application/json"},
				"X-Upload-Content-Type":   {contentType},
				"X-Upload-Content-Length": {strconv.Itoa(len(data))}})
		if err != nil {
			return File{}, err
		}
		session := resp.Header.Get("Location")
		_ = resp.Body.Close() // A session response carries no media.
		if session == "" {
			return File{}, errors.New("Drive upload session missing location")
		}

		// Advance only from the server's confirmed range, not sent byte counts.
		offset, stalled := 0, 0
		for offset < len(data) && stalled < 8 {
			end := min(offset+ChunkBytes, len(data))
			h := http.Header{"Content-Type": {contentType}, "Content-Range": {
				"bytes " + strconv.Itoa(offset) + "-" + strconv.Itoa(end-1) + "/" + strconv.Itoa(len(data))}}
			resp, err = c.request(ctx, http.MethodPut, session, data[offset:end], h)
			if errors.Is(err, block_store_s3.ErrNotFound) {
				break
			}
			if errors.Is(err, ErrUploadLimit) || errors.Is(err, ErrQuotaFull) || errors.Is(err, ErrSignIn) || ctx.Err() != nil {
				return File{}, err
			}
			if err != nil {
				var retry *uploadRetry
				if !errors.As(err, &retry) {
					return File{}, err
				}
				if err := waitRetry(ctx, max(retry.after, time.Duration(1<<stalled)*100*time.Millisecond)); err != nil {
					return File{}, err
				}
				resp, err = c.request(ctx, http.MethodPut, session, nil,
					http.Header{"Content-Range": {"bytes */" + strconv.Itoa(len(data))}})
				if errors.Is(err, block_store_s3.ErrNotFound) {
					break
				}
				if err != nil {
					return File{}, err
				}
			}

			// The final response authenticates the complete file's checksum.
			if resp.StatusCode != 308 {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				_ = resp.Body.Close() // Complete metadata has been consumed.
				if readErr != nil {
					return File{}, readErr
				}
				v, parseErr := fastjson.ParseBytes(body)
				if parseErr != nil {
					return File{}, parseErr
				}
				return ParseFile(v)
			}

			// An empty committed range means the first chunk never arrived.
			raw := resp.Header.Get("Range")
			_ = resp.Body.Close() // Incomplete responses carry no object body.
			next := 0
			if raw != "" {
				n, parseErr := strconv.Atoi(strings.TrimPrefix(raw, "bytes=0-"))
				if parseErr != nil || n < 0 || n+1 > end {
					return File{}, errors.New("Drive upload returned invalid committed range")
				}
				next = n + 1
			}
			if next < offset {
				return File{}, errors.New("Drive upload committed range moved backwards")
			}
			if next == offset {
				stalled++
			} else {
				stalled = 0
			}
			offset = next
		}
		if stalled == 8 {
			return File{}, errors.New("Drive upload made no progress")
		}
	}
	return File{}, errors.New("Drive upload session repeatedly expired")
}

// find resolves an existing file without creating a directory.
func (c *Client) find(ctx context.Context, key string) (File, error) {
	// Load this directory before reading its verified name cache.
	dir, _ := path.Split(key)
	if _, err := c.directory(ctx, strings.TrimSuffix(dir, "/"), false); err != nil {
		return File{}, err
	}
	c.mtx.Lock()
	f, ok := c.files[key]
	c.mtx.Unlock()
	if !ok || !f.Verified() {
		return File{}, block_store_s3.ErrNotFound
	}
	return f, nil
}

// GetObject opens the complete immutable object.
func (c *Client) GetObject(ctx context.Context, _ string, key string) (io.ReadCloser, error) {
	// Resolve the opaque file ID before requesting its media.
	f, err := c.find(ctx, key)
	if err != nil {
		return nil, err
	}
	resp, err := c.request(ctx, http.MethodGet, c.api+"/"+url.PathEscape(f.ID)+"?alt=media", nil, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// GetObjectRange reads only the requested extent of an immutable object.
func (c *Client) GetObjectRange(ctx context.Context, _ string, key string, off int64, length int) ([]byte, error) {
	// Clamp the request at the known complete file size.
	if off < 0 || length < 0 {
		return nil, errors.New("negative Drive object range")
	}

	// Resolve the complete file before clamping the requested extent.
	f, err := c.find(ctx, key)
	if err != nil {
		return nil, err
	}
	if off >= f.Size || length == 0 {
		return nil, nil
	}

	// Request the exact range after validating its bounds.
	end := off + min(int64(length), f.Size-off) - 1
	resp, err := c.request(ctx, http.MethodGet, c.api+"/"+url.PathEscape(f.ID)+"?alt=media", nil,
		http.Header{"Range": {"bytes=" + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(end, 10)}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, errors.New("Drive ignored the requested object range")
	}
	return io.ReadAll(io.LimitReader(resp.Body, end-off+1))
}

// ListObjects streams the objects whose keys have prefix, one directory page
// at a time. A missing prefix is an empty listing.
func (c *Client) ListObjects(ctx context.Context, _ string, prefix string, visit func(string, int64) error) error {
	// Resolve the deepest complete directory segment of the prefix.
	dir, _ := path.Split(prefix)
	_, err := c.directory(ctx, strings.TrimSuffix(dir, "/"), false)
	if errors.Is(err, block_store_s3.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return c.walk(ctx, strings.TrimSuffix(dir, "/"), prefix, visit)
}

// walk streams a directory subtree without retaining the full tree.
func (c *Client) walk(ctx context.Context, dir, prefix string, visit func(string, int64) error) error {
	// Resolve all same-name directories before choosing one verified file per key.
	if _, err := c.directory(ctx, dir, false); err != nil {
		return err
	}
	c.mtx.Lock()
	folders := slices.Clone(c.folders[dir])
	c.mtx.Unlock()
	for _, folder := range folders {
		if err := c.list(ctx, folder.ID, func(f File) error {
			// Visit only the canonical entry across all duplicate folder listings.
			key := path.Join(dir, f.Name)
			c.mtx.Lock()
			canonical := c.files[key]
			c.mtx.Unlock()
			if canonical.ID != f.ID {
				return nil
			}
			if f.Folder {
				if strings.HasPrefix(prefix, key+"/") || strings.HasPrefix(key+"/", prefix) {
					return c.walk(ctx, key, prefix, visit)
				}
				return nil
			}
			if f.Verified() && f.Key == key && strings.HasPrefix(key, prefix) {
				return visit(key, f.Size)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteObject always refuses and makes no HTTP request.
func (c *Client) DeleteObject(context.Context, string, string) error {
	return ErrDeleteForbidden
}

// _ is a type assertion.
var _ block_store_s3.ObjectStore = (*Client)(nil)
