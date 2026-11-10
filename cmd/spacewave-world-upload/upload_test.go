package main

import (
	"bytes"
	"crypto/md5" //nolint:gosec // Content-MD5.
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// testSecret is the secret of the test creds; no output may hold it.
const testSecret = "test-secret-do-not-print"

// writePair writes one pack of n blocks and its entry into dir, named by its
// v1 id as the re-pack tool names them, and returns the id.
func writePair(t *testing.T, dir string, seed, n int) string {
	t.Helper()
	var blocks [][]byte
	var hashes []*hash.Hash
	for i := range n {
		data := []byte(fmt.Sprintf("block %d of pack %d", i, seed))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		blocks, hashes = append(blocks, data), append(hashes, ref.GetHash())
	}
	var buf bytes.Buffer
	i := 0
	res, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(blocks) {
			return nil, nil, nil
		}
		i++
		return hashes[i-1], &block.StoredBlock{Data: blocks[i-1], RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.BuildPackID(defaultResourceID, res)
	if err != nil {
		t.Fatal(err)
	}
	writeEntry(t, dir, &packfile.PackfileEntry{
		Id: id, BloomFilter: res.BloomFilter, BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount: res.BlockCount, SizeBytes: res.BytesWritten,
		CreatedAt: timestamppb.New(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)),
	})
	if err := os.WriteFile(filepath.Join(dir, id), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return id
}

// writeEntry writes an entry file for its id.
func writeEntry(t *testing.T, dir string, e *packfile.PackfileEntry) {
	t.Helper()
	raw, err := e.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, e.GetId()+entrySuffix), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeS3 records the requests it receives and checks each is a PUT whose
// checksum headers match its body. fail maps a path to the status to answer.
type fakeS3 struct {
	t    *testing.T
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []string
	fail map[string]int
}

// newFakeS3 starts a TLS server.
func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{t: t, fail: map[string]int{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path)
	if r.Method != http.MethodPut {
		f.t.Errorf("request with method %s to %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Error(err)
		return
	}
	sum := sha256.Sum256(body)
	m := md5.Sum(body) //nolint:gosec // Content-MD5.
	if got, want := r.Header.Get("X-Amz-Checksum-Sha256"), base64.StdEncoding.EncodeToString(sum[:]); got != want {
		f.t.Errorf("%s: x-amz-checksum-sha256 %q, want %q", r.URL.Path, got, want)
	}
	if got, want := r.Header.Get("Content-Md5"), base64.StdEncoding.EncodeToString(m[:]); got != want {
		f.t.Errorf("%s: content-md5 %q, want %q", r.URL.Path, got, want)
	}
	if got, want := r.Header.Get("X-Amz-Content-Sha256"), fmt.Sprintf("%x", sum); got != want {
		f.t.Errorf("%s: x-amz-content-sha256 %q, want %q", r.URL.Path, got, want)
	}
	auth := r.Header.Get("Authorization")
	if !strings.Contains(auth, "content-md5;") || !strings.Contains(auth, "x-amz-checksum-sha256;") || strings.Contains(auth, testSecret) {
		f.t.Errorf("%s: authorization %q does not sign the checksums", r.URL.Path, auth)
	}
	if code, ok := f.fail[r.URL.Path]; ok {
		if code/100 == 3 {
			w.Header().Set("Location", "/bucket-x/elsewhere")
		}
		w.WriteHeader(code)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// requests returns the requests so far.
func (f *fakeS3) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reqs)
}

// setup writes the pairs, the creds, and the allow-list of every id, and
// returns options aimed at f.
func setup(t *testing.T, f *fakeS3, pairs int) (*options, []string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "in")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := range pairs {
		ids = append(ids, writePair(t, dir, i, 3+i))
	}
	slices.Sort(ids)
	credsPath := filepath.Join(root, "creds")
	if err := os.WriteFile(credsPath, []byte("# test\naccess_key_id=AKTEST\nsecret_access_key="+testSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	allow := filepath.Join(root, "allow")
	if err := os.WriteFile(allow, []byte(strings.Join(ids, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &options{
		Dir: dir, Endpoint: f.srv.URL, Region: "us-west-004", Bucket: "bucket-x", Prefix: "w/repack-1/",
		CredsPath: credsPath, Manifest: filepath.Join(root, "manifest.tsv"), AllowList: allow,
		ResourceID: defaultResourceID, Log: &bytes.Buffer{}, HTTP: f.srv.Client(),
	}, ids
}

// puts returns the PUT lines for the ids, pack before entry.
func puts(prefix string, ids ...string) []string {
	var out []string
	for _, id := range ids {
		out = append(out, "PUT /bucket-x/"+prefix+"packs/"+id, "PUT /bucket-x/"+prefix+"entries/"+id)
	}
	return out
}

func TestKeyGuard(t *testing.T) {
	// Refuse bad prefixes.
	for _, p := range []string{"", "w", "/w/", "w//", "../", "w/../", "w/./", "./w/", ".w/", "w x/", "w/\n/", "w\\x/", "w/packs/pfv1_a"} {
		if _, err := newKeyGuard(p); err == nil {
			t.Errorf("prefix %q accepted", p)
		}
	}

	// Accept good keys and refuse bad ids and paths under a good prefix.
	g, err := newKeyGuard("w/repack-1/")
	if err != nil {
		t.Fatal(err)
	}
	id := identity.NewPackID()
	for _, k := range []string{"w/repack-1/packs/" + id, "w/repack-1/entries/" + id} {
		if err := g.check(k); err != nil {
			t.Errorf("key %q refused: %v", k, err)
		}
	}
	for _, k := range []string{
		"w/repack-1/packs/pfv1_0abc", "w/repack-1/packs/pfv1_Ol", "w/repack-1/packs/pfv1_", "w/repack-1/packs/" + id + ".entry",
		"w/repack-1/packs/../packs/" + id, "w/repack-1/../packs/" + id, "w/repack-1/packs/" + id + "/x", "w/repack-1/packs//" + id,
		"w/repack-1/other/" + id, "w/repack-2/packs/" + id, "/w/repack-1/packs/" + id, "w/repack-1/packs/" + id + "\n",
	} {
		if err := g.check(k); err == nil {
			t.Errorf("key %q accepted", k)
		}
	}

	// A plan built from a bad prefix sends nothing.
	f := newFakeS3(t)
	o, _ := setup(t, f, 1)
	o.Prefix = "w/../x/"
	if _, err := run(o); err == nil {
		t.Fatal("run with a .. prefix succeeded")
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}
}

func TestCredsMode0644Refused(t *testing.T) {
	f := newFakeS3(t)
	o, _ := setup(t, f, 1)
	if err := os.Chmod(o.CredsPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCreds(o.CredsPath); err == nil || !strings.Contains(err.Error(), "0644") {
		t.Fatalf("mode 0644 creds: %v", err)
	}
	_, err := run(o)
	if err == nil || strings.Contains(err.Error(), testSecret) {
		t.Fatalf("run with 0644 creds: %v", err)
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}

	// The 0600 file reads, and printing it hides the secret.
	if err := os.Chmod(o.CredsPath, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := readCreds(o.CredsPath)
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprintf("%v %s", c, c); strings.Contains(s, testSecret) || strings.Contains(s, "AKTEST") {
		t.Fatalf("creds print %q", s)
	}
}

func TestPackBeforeEntryOnlyPUT(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 3)
	rep, err := run(o)
	if err != nil {
		t.Fatal(err)
	}

	// Each pack, then its entry, in id order, PUT alone.
	if got, want := f.requests(), puts(o.Prefix, ids...); !slices.Equal(got, want) {
		t.Fatalf("requests\n%v\nwant\n%v", got, want)
	}
	if rep.Objects != 6 || rep.Bytes == 0 {
		t.Fatalf("report %+v", rep)
	}

	// The manifest holds one 200 line per object, with its digests.
	raw, err := os.ReadFile(o.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 6 {
		t.Fatalf("manifest has %d lines", len(lines))
	}
	data, _ := os.ReadFile(filepath.Join(o.Dir, ids[0]))
	sum := sha256.Sum256(data)
	f0 := strings.Split(lines[0], "\t")
	if f0[1] != o.Prefix+"packs/"+ids[0] || f0[2] != fmt.Sprint(len(data)) || f0[3] != fmt.Sprintf("%x", sum) || f0[5] != "200" {
		t.Fatalf("manifest line %q", lines[0])
	}
	if out := o.Log.(*bytes.Buffer).String(); !strings.Contains(out, "totals: 6 objects") || strings.Contains(out, testSecret) {
		t.Fatalf("log %q", out)
	}
}

func TestStopOnFirstNon200(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		entry  bool
	}{
		{"pack 500", http.StatusInternalServerError, false},
		{"pack 403", http.StatusForbidden, false},
		{"pack 307", http.StatusTemporaryRedirect, false},
		{"pack 204", http.StatusNoContent, false},
		{"entry 503", http.StatusServiceUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeS3(t)
			o, ids := setup(t, f, 2)
			key := o.Prefix + "packs/" + ids[0]
			if tc.entry {
				key = o.Prefix + "entries/" + ids[0]
			}
			f.fail["/bucket-x/"+key] = tc.status
			if _, err := run(o); err == nil {
				t.Fatal("run succeeded")
			}

			// One request for the failed object, no retry, no other key,
			// and no entry after a failed pack.
			want := puts(o.Prefix, ids[0])
			if !tc.entry {
				want = want[:1]
			}
			if got := f.requests(); !slices.Equal(got, want) {
				t.Fatalf("requests %v, want %v", got, want)
			}

			// The manifest records the failure, which resume does not skip.
			done, err := readManifest(o.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := done[key]; ok {
				t.Fatalf("failed key %s recorded as done", key)
			}
		})
	}
}

func TestResumeSkipsDone(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 3)

	// The first run fails on the second id's entry.
	f.fail["/bucket-x/"+o.Prefix+"entries/"+ids[1]] = http.StatusInternalServerError
	if _, err := run(o); err == nil {
		t.Fatal("first run succeeded")
	}

	// A rerun without --resume refuses the used manifest and sends nothing.
	delete(f.fail, "/bucket-x/"+o.Prefix+"entries/"+ids[1])
	f.reqs = nil
	if _, err := run(o); err == nil {
		t.Fatal("rerun without --resume succeeded")
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}

	// With --resume, the first id is skipped and the rest are put.
	o.Resume = true
	rep, err := run(o)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.requests(), puts(o.Prefix, ids[1], ids[2]); !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	if rep.Skipped != 1 || rep.Objects != 4 {
		t.Fatalf("report %+v", rep)
	}

	// A third run skips every id.
	f.reqs = nil
	if rep, err = run(o); err != nil || rep.Skipped != 3 || len(f.requests()) != 0 {
		t.Fatalf("third run: %+v %v %v", rep, err, f.requests())
	}
}

func TestSizeMismatchRefused(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 2)

	// Rewrite the second entry one byte larger than its pack.
	raw, err := os.ReadFile(filepath.Join(o.Dir, ids[1]+entrySuffix))
	if err != nil {
		t.Fatal(err)
	}
	e := &packfile.PackfileEntry{}
	if err := e.UnmarshalVT(raw); err != nil {
		t.Fatal(err)
	}
	e.SizeBytes++
	writeEntry(t, o.Dir, e)

	// The plan is refused before the first id's PUT.
	if _, err := run(o); err == nil || !strings.Contains(err.Error(), "bytes, the pack is") {
		t.Fatalf("run: %v", err)
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}
}

func TestAllowListRefusesOtherID(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 2)

	// Allow the first id alone: the plan of both is refused.
	if err := os.WriteFile(o.AllowList, []byte(ids[0]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(o); err == nil || !strings.Contains(err.Error(), "not in --allow-list") {
		t.Fatalf("run: %v", err)
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}

	// A subset within the allow-list is put, in the --ids order.
	idsFile := filepath.Join(t.TempDir(), "ids")
	if err := os.WriteFile(idsFile, []byte(ids[0]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o.IDs = idsFile
	if _, err := run(o); err != nil {
		t.Fatal(err)
	}
	if got, want := f.requests(), puts(o.Prefix, ids[0]); !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
}

func TestIncompletePairAndWrongIDRefused(t *testing.T) {
	// A pack without its entry.
	f := newFakeS3(t)
	o, ids := setup(t, f, 2)
	if err := os.Remove(filepath.Join(o.Dir, ids[1]+entrySuffix)); err != nil {
		t.Fatal(err)
	}
	if _, err := run(o); err == nil || !strings.Contains(err.Error(), "has no entry") {
		t.Fatalf("pack without entry: %v", err)
	}

	// A pack whose bytes give another id.
	o, ids = setup(t, f, 2)
	other := identity.NewPackID()
	if err := os.Rename(filepath.Join(o.Dir, ids[1]), filepath.Join(o.Dir, other)); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(o.Dir, ids[1]+entrySuffix))
	e := &packfile.PackfileEntry{}
	if err := e.UnmarshalVT(raw); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(o.Dir, ids[1]+entrySuffix))
	e.Id = other
	writeEntry(t, o.Dir, e)
	if err := os.WriteFile(o.AllowList, []byte(ids[0]+"\n"+other+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(o); err == nil || !strings.Contains(err.Error(), "its bytes give id") {
		t.Fatalf("wrong id: %v", err)
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}
}

// TestSignV4Vector checks the signer against the PUT example of the AWS
// SigV4 documentation for S3.
func TestSignV4Vector(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://examplebucket.s3.amazonaws.com/test$file.text", strings.NewReader("Welcome to Amazon S3."))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	req.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	p := &putter{region: "us-east-1", creds: &creds{accessKeyID: "AKIAIOSFODNN7EXAMPLE", secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}}
	p.sign(req, "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class, " +
		"Signature=98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization\n%s\nwant\n%s", got, want)
	}
}
