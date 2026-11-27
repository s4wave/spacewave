// Package gdrivetest provides an offline Drive v3 server with atomic uploads,
// request recording, pagination, range reads, and injected failures.
package gdrivetest

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
)

// Server is an isolated fake Drive with no route to Google's service.
type Server struct {
	// HTTP is the test HTTP listener.
	HTTP *httptest.Server
	// mtx guards all state, including request recording.
	mtx sync.Mutex
	// callsBcast guards calls and provides lossless request notifications.
	callsBcast broadcast.Broadcast
	// calls records every request, including forbidden methods.
	calls []Call
	// files holds complete files and folders by ID.
	files map[string]*file
	// uploads holds incomplete files by session ID.
	uploads map[string]*file
	// failures are injected before handling calls in order.
	failures []Failure
	// next supplies opaque file and session identities.
	next int
	// directory holds completed fake objects on disk, empty for an in-memory fixture.
	directory string
	// pageSize bounds listings so even small fixtures exercise paging.
	pageSize int
}

// NewServer starts an empty fake whose root folder ID is root.
func NewServer() *Server {
	return newServer("")
}

// NewDiskServer keeps complete objects on disk so a multi-gigabyte fixture
// retains at most the current resumable upload in memory.
func NewDiskServer(directory string) (*Server, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	return newServer(directory), nil
}

// newServer starts the fake with its object-storage policy fixed before serving.
func newServer(directory string) *Server {
	s := &Server{files: make(map[string]*file), uploads: make(map[string]*file), pageSize: 2, directory: directory}
	s.HTTP = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	return s
}

// Close stops the listener after tests release the clients.
func (s *Server) Close() { s.HTTP.Close() }

// Calls returns a copy of the recorded actions.
func (s *Server) Calls() []Call {
	var calls []Call
	s.callsBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		calls = append([]Call(nil), s.calls...)
	})
	return calls
}

// WaitCalls waits for requests after a recorded offset, without polling.
func (s *Server) WaitCalls(ctx context.Context, after int) ([]Call, error) {
	for {
		var calls []Call
		var changed <-chan struct{}
		s.callsBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
			if len(s.calls) > after {
				calls = append(calls, s.calls[after:]...)
			}
		})
		if len(calls) != 0 {
			return calls, nil
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-changed:
		}
	}
}

// Inject queues error responses, each consumed by the next HTTP call.
func (s *Server) Inject(failures ...Failure) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.failures = append(s.failures, failures...)
}

// writeFailure emits the nested error shape Drive uses for retry decisions.
func writeFailure(w http.ResponseWriter, f Failure) {
	// Build the external JSON error without reflecting a Go struct.
	var a fastjson.Arena
	v, detail, reasons, reason := a.NewObject(), a.NewObject(), a.NewArray(), a.NewObject()
	reason.Set("reason", a.NewString(f.Reason))
	reasons.SetArrayItem(0, reason)
	detail.Set("errors", reasons)
	detail.Set("message", a.NewString(f.Message))
	v.Set("error", detail)

	// Send the nested provider error with the requested HTTP status.
	w.WriteHeader(f.Status)
	_, _ = w.Write(v.MarshalTo(nil)) // A disconnected test client needs no reply.
}

// serveHTTP records every call and then executes one atomic fake endpoint.
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// Serialize fake endpoint state, including the optional disk commit.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.callsBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.calls = append(s.calls, Call{Method: r.Method, Path: r.URL.Path, Range: r.Header.Get("Range") + r.Header.Get("Content-Range")})
		broadcast()
	})
	if len(s.failures) != 0 {
		f := s.failures[0]
		s.failures = s.failures[1:]
		writeFailure(w, f)
		return
	}

	// Keep deletion a visible failure even if a new client path tries it.
	if r.Method == http.MethodDelete || r.Method == http.MethodPatch {
		writeFailure(w, Failure{Status: 405, Message: "immutable backup"})
		return
	}
	if r.Method == http.MethodGet {
		s.get(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/session/") {
		s.chunk(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeFailure(w, Failure{Status: 405})
		return
	}

	// Read the two upload protocols or a folder create.
	var meta, data []byte
	var err error
	if r.URL.Query().Get("uploadType") == "multipart" {
		_, params, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr != nil {
			writeFailure(w, Failure{Status: 400})
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		part, partErr := reader.NextPart()
		if partErr != nil {
			writeFailure(w, Failure{Status: 400})
			return
		}

		// Read both related parts before publishing the file.
		meta, err = io.ReadAll(part)
		if err == nil {
			part, err = reader.NextPart()
		}
		if err == nil {
			data, err = io.ReadAll(part)
		}
	} else {
		meta, err = io.ReadAll(r.Body)
	}
	if err != nil {
		writeFailure(w, Failure{Status: 400})
		return
	}

	// Assign identity only after the create metadata is complete.
	v, err := fastjson.ParseBytes(meta)
	if err != nil {
		writeFailure(w, Failure{Status: 400})
		return
	}
	s.next++
	f := &file{id: strconv.Itoa(s.next), parent: string(v.GetStringBytes("parents", "0")), meta: meta, data: data}
	if r.URL.Query().Get("uploadType") == "resumable" {
		s.uploads[f.id] = f
		w.Header().Set("Location", s.HTTP.URL+"/session/"+f.id)
		w.WriteHeader(200)
		return
	}
	if err := s.finishFile(f); err != nil {
		writeFailure(w, Failure{Status: 500})
		return
	}
	s.files[f.id] = f
	_, _ = w.Write(s.fileMetadata(f)) // A disconnected client may leave a complete orphan.
}

// get implements paged parent queries and exact range media reads.
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	// List in creation order so page tokens remain stable across one listing.
	if r.URL.Path == "/drive/v3/files" {
		q := r.URL.Query().Get("q")
		if !strings.HasSuffix(q, "' in parents and trashed = false") {
			writeFailure(w, Failure{Status: 400})
			return
		}
		parent := strings.TrimSuffix(strings.TrimPrefix(q, "'"), "' in parents and trashed = false")
		start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
		var a fastjson.Arena
		v, entries := a.NewObject(), a.NewArray()
		count, next := 0, 0
		for n := max(start, 1); n <= s.next; n++ {
			f := s.files[strconv.Itoa(n)]
			if f == nil || f.parent != parent {
				continue
			}
			if count == s.pageSize {
				next = n
				break
			}
			item, _ := fastjson.ParseBytes(s.fileMetadata(f))
			entries.SetArrayItem(count, item)
			count++
		}
		v.Set("files", entries)
		if next != 0 {
			v.Set("nextPageToken", a.NewString(strconv.Itoa(next)))
		}
		_, _ = w.Write(v.MarshalTo(nil)) // No state depends on delivery of a listing.
		return
	}

	// Never expose a session's partial bytes through files.get.
	f := s.files[strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")]
	if f == nil {
		writeFailure(w, Failure{Status: 404})
		return
	}
	if r.URL.Query().Get("alt") != "media" {
		_, _ = w.Write(s.fileMetadata(f)) // Metadata reads are side-effect free.
		return
	}
	var media io.ReaderAt = bytes.NewReader(f.data)
	var disk *os.File
	if f.path != "" {
		var err error
		disk, err = os.Open(f.path)
		if err != nil {
			writeFailure(w, Failure{Status: 500})
			return
		}
		defer disk.Close()
		media = disk
	}
	if raw := r.Header.Get("Range"); raw != "" {
		bounds := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
		start, err := strconv.Atoi(bounds[0])
		if err != nil || len(bounds) != 2 {
			writeFailure(w, Failure{Status: 416})
			return
		}
		end, err := strconv.Atoi(bounds[1])
		if err != nil || start < 0 || end < start || end >= f.storedSize {
			writeFailure(w, Failure{Status: 416})
			return
		}
		w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(f.storedSize))
		w.WriteHeader(206)
		_, _ = io.Copy(w, io.NewSectionReader(media, int64(start), int64(end-start+1))) // Media reads do not mutate the file.
		return
	}
	_, _ = io.Copy(w, io.NewSectionReader(media, 0, int64(f.storedSize))) // Media reads do not mutate the file.
}

// chunk commits bytes to an upload and publishes its file only at completion.
func (s *Server) chunk(w http.ResponseWriter, r *http.Request) {
	// Reject expired sessions without touching existing complete files.
	id := strings.TrimPrefix(r.URL.Path, "/session/")
	f := s.uploads[id]
	if f == nil {
		writeFailure(w, Failure{Status: 404})
		return
	}
	if f.finished {
		_, _ = w.Write(s.fileMetadata(f)) // Status probes after a lost final reply see the complete file.
		return
	}

	// Decode the declared extent, preserving status-only probes.
	raw := strings.TrimPrefix(r.Header.Get("Content-Range"), "bytes ")
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		writeFailure(w, Failure{Status: 400})
		return
	}
	total, err := strconv.Atoi(parts[1])
	if err != nil {
		writeFailure(w, Failure{Status: 400})
		return
	}
	if parts[0] != "*" {
		bounds := strings.Split(parts[0], "-")
		if len(bounds) != 2 {
			writeFailure(w, Failure{Status: 400})
			return
		}

		// Enforce sequential chunks and the Drive alignment rule.
		start, _ := strconv.Atoi(bounds[0])
		end, _ := strconv.Atoi(bounds[1])
		data, readErr := io.ReadAll(r.Body)
		if readErr != nil || start != len(f.data) || end-start+1 != len(data) || end >= total || (end+1 < total && len(data)%(256<<10) != 0) {
			writeFailure(w, Failure{Status: 400, Message: errors.New("invalid upload range").Error()})
			return
		}
		f.data = append(f.data, data...)
	}

	// Publish only complete bytes; otherwise report the committed prefix.
	if len(f.data) == total {
		if err := s.finishFile(f); err != nil {
			writeFailure(w, Failure{Status: 500})
			return
		}
		s.files[id] = f
		_, _ = w.Write(s.fileMetadata(f)) // A lost reply leaves a complete immutable file.
		return
	}
	if len(f.data) != 0 {
		w.Header().Set("Range", "bytes=0-"+strconv.Itoa(len(f.data)-1))
	}
	w.WriteHeader(308)
}

// ExpireUploads drops incomplete sessions without deleting any complete file.
func (s *Server) ExpireUploads() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	clear(s.uploads)
}

// SplitFolder simulates concurrent writers whose same-name folders contain
// disjoint complete files. It preserves each file's declared logical key.
// This fixture mutation makes no HTTP call and is never used by production.
func (s *Server) SplitFolder(name string) bool {
	// Select an existing folder in creation order and duplicate its metadata.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	var original *file
	for index := 1; index <= s.next; index++ {
		candidate := s.files[strconv.Itoa(index)]
		if candidate == nil {
			continue
		}
		value, _ := fastjson.ParseBytes(candidate.meta)
		if string(value.GetStringBytes("mimeType")) == "application/vnd.google-apps.folder" && string(value.GetStringBytes("name")) == name {
			original = candidate
			break
		}
	}

	// Add the duplicate only after finding a complete existing directory.
	if original == nil {
		return false
	}
	s.next++
	duplicate := *original
	duplicate.id = strconv.Itoa(s.next)
	s.files[duplicate.id] = &duplicate

	// Alternate child files between the otherwise identical directories.
	count := 0
	for index := 1; index < s.next; index++ {
		child := s.files[strconv.Itoa(index)]
		if child != nil && child.parent == original.id {
			if count%2 == 0 {
				child.parent = duplicate.id
			}
			count++
		}
	}
	return true
}
