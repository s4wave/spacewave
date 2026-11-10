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
// checksum headers match its body; x-amz-checksum-sha256 may be absent but
// is signed when present. fail maps a path to the status to answer,
// badSum the paths to answer 200 with a wrong echoed sha256, echo the paths
// to answer 200 with the right one. stored holds
// the paths put with 200; an If-None-Match: * PUT to one gets 412. headers
// holds the last request headers of each path.
type fakeS3 struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	reqs    []string
	fail    map[string]int
	badSum  map[string]bool
	echo    map[string]bool
	stored  map[string]bool
	headers map[string]http.Header
}

// newFakeS3 starts a TLS server.
func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{t: t, fail: map[string]int{}, badSum: map[string]bool{}, echo: map[string]bool{}, stored: map[string]bool{}, headers: map[string]http.Header{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path)
	f.headers[r.URL.Path] = r.Header.Clone()
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
	if got, want := r.Header.Get("X-Amz-Checksum-Sha256"), base64.StdEncoding.EncodeToString(sum[:]); got != "" && got != want {
		f.t.Errorf("%s: x-amz-checksum-sha256 %q, want %q", r.URL.Path, got, want)
	}
	if got, want := r.Header.Get("Content-Md5"), base64.StdEncoding.EncodeToString(m[:]); got != want {
		f.t.Errorf("%s: content-md5 %q, want %q", r.URL.Path, got, want)
	}
	if got, want := r.Header.Get("X-Amz-Content-Sha256"), fmt.Sprintf("%x", sum); got != want {
		f.t.Errorf("%s: x-amz-content-sha256 %q, want %q", r.URL.Path, got, want)
	}
	auth := r.Header.Get("Authorization")
	hasSum := r.Header.Get("X-Amz-Checksum-Sha256") != ""
	if !strings.Contains(auth, "content-md5;") || strings.Contains(auth, "x-amz-checksum-sha256;") != hasSum || strings.Contains(auth, testSecret) {
		f.t.Errorf("%s: authorization %q does not sign the checksums", r.URL.Path, auth)
	}

	// Answer the configured failure, a 412 for an existing key, or 200.
	if code, ok := f.fail[r.URL.Path]; ok {
		if code/100 == 3 {
			w.Header().Set("Location", "/bucket-x/elsewhere")
		}
		w.WriteHeader(code)
		return
	}
	if r.Header.Get("If-None-Match") == "*" && f.stored[r.URL.Path] {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	f.stored[r.URL.Path] = true
	if f.badSum[r.URL.Path] {
		w.Header().Set("X-Amz-Checksum-Sha256", base64.StdEncoding.EncodeToString(make([]byte, sha256.Size)))
	}
	if f.echo[r.URL.Path] {
		w.Header().Set("X-Amz-Checksum-Sha256", base64.StdEncoding.EncodeToString(sum[:]))
	}
	w.WriteHeader(http.StatusOK)
}

// reset forgets the requests so far.
func (f *fakeS3) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = nil
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
		ResourceID: defaultResourceID, ChecksumHeader: true, Log: &bytes.Buffer{}, Warn: &bytes.Buffer{}, HTTP: f.srv.Client(),
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
			done, err := readManifest(o.Manifest, io.Discard)
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
	f.reset()
	if _, err := run(o); err == nil {
		t.Fatal("rerun without --resume succeeded")
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}

	// With --resume, the first id is skipped, the second id's pack, put with
	// 200, is not put again, and only its entry and the third id are put.
	o.Resume = true
	rep, err := run(o)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.requests(), append(puts(o.Prefix, ids[1])[1:], puts(o.Prefix, ids[2])...); !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	if rep.Skipped != 1 || rep.SkippedObjects != 3 || rep.Objects != 3 {
		t.Fatalf("report %+v", rep)
	}

	// A third run skips every id.
	f.reset()
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

// rewriteEntry decodes the entry of id, applies edit, and writes it back.
func rewriteEntry(t *testing.T, dir, id string, edit func(*packfile.PackfileEntry)) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, id+entrySuffix))
	if err != nil {
		t.Fatal(err)
	}
	e := &packfile.PackfileEntry{}
	if err := e.UnmarshalVT(raw); err != nil {
		t.Fatal(err)
	}
	edit(e)
	writeEntry(t, dir, e)
}

func TestEntryBloomAndBlockCountRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*packfile.PackfileEntry)
		want string
	}{
		{"block count", func(e *packfile.PackfileEntry) { e.BlockCount++ }, "blocks, the pack holds"},
		{"bloom byte", func(e *packfile.PackfileEntry) { e.BloomFilter[len(e.BloomFilter)-1] ^= 1 }, "bloom filter that is not the pack's"},
		{"bloom empty", func(e *packfile.PackfileEntry) { e.BloomFilter = nil }, "bloom filter that is not the pack's"},
		{"bloom format", func(e *packfile.PackfileEntry) { e.BloomFormatVersion = 0 }, "bloom format version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The second entry keeps its id and size but not the rest.
			f := newFakeS3(t)
			o, ids := setup(t, f, 2)
			rewriteEntry(t, o.Dir, ids[1], tc.edit)

			// The plan is refused before the first id's PUT.
			if _, err := run(o); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run: %v, want %q", err, tc.want)
			}
			if r := f.requests(); len(r) != 0 {
				t.Fatalf("requests sent: %v", r)
			}
		})
	}
}

func TestBadSumRecordedNotDone(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 2)
	key := o.Prefix + "packs/" + ids[0]
	f.badSum["/bucket-x/"+key] = true

	// The 200 with a wrong echoed sha256 fails the run at once.
	if _, err := run(o); err == nil || !strings.Contains(err.Error(), "service reports sha256") {
		t.Fatalf("run: %v", err)
	}
	if got, want := f.requests(), puts(o.Prefix, ids[0])[:1]; !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}

	// The manifest records it as 200-badsum, which resume does not count.
	raw, err := os.ReadFile(o.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\t")
	if len(fields) != 6 || fields[1] != key || fields[5] != statusBadSum {
		t.Fatalf("manifest %q", raw)
	}
	done, err := readManifest(o.Manifest, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := done[key]; ok {
		t.Fatalf("200-badsum key %s recorded as done", key)
	}
}

func TestEchoCounts(t *testing.T) {
	// Two pairs; the service echoes our sha256 for three objects and none
	// for the last.
	f := newFakeS3(t)
	o, ids := setup(t, f, 2)
	f.echo["/bucket-x/"+o.Prefix+"packs/"+ids[0]] = true
	f.echo["/bucket-x/"+o.Prefix+"entries/"+ids[0]] = true
	f.echo["/bucket-x/"+o.Prefix+"packs/"+ids[1]] = true
	rep, err := run(o)
	if err != nil {
		t.Fatal(err)
	}

	// The report and the one totals line both count them.
	if rep.EchoMatched != 3 || rep.NoEcho != 1 {
		t.Fatalf("echo matched %d, no echo %d, want 3 and 1", rep.EchoMatched, rep.NoEcho)
	}
	out := o.Log.(*bytes.Buffer).String()
	if n := strings.Count(out, "echo matched"); n != 1 || !strings.Contains(out, "; echo matched 3, no echo 1\n") {
		t.Fatalf("log %q lacks one echo count line", out)
	}
}

func TestChecksumHeaderFlag(t *testing.T) {
	// signedHeaders returns the SignedHeaders list of h's Authorization.
	signedHeaders := func(h http.Header) []string {
		_, after, _ := strings.Cut(h.Get("Authorization"), "SignedHeaders=")
		signed, _, _ := strings.Cut(after, ",")
		return strings.Split(signed, ";")
	}

	// With --checksum-header, the default, every PUT carries it signed.
	f := newFakeS3(t)
	o, ids := setup(t, f, 1)
	if _, err := run(o); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/bucket-x/" + o.Prefix + "packs/" + ids[0], "/bucket-x/" + o.Prefix + "entries/" + ids[0]} {
		h := f.headers[path]
		if h.Get("X-Amz-Checksum-Sha256") == "" {
			t.Fatalf("%s: x-amz-checksum-sha256 absent", path)
		}
		if !slices.Contains(signedHeaders(h), "x-amz-checksum-sha256") {
			t.Fatalf("%s: signed headers %v lack x-amz-checksum-sha256", path, signedHeaders(h))
		}
	}

	// With --checksum-header=false it is absent and unsigned, the payload
	// hash and Content-MD5 stay, and the run passes.
	f2 := newFakeS3(t)
	o2, ids2 := setup(t, f2, 1)
	o2.ChecksumHeader = false
	if _, err := run(o2); err != nil {
		t.Fatal(err)
	}
	if got, want := f2.requests(), puts(o2.Prefix, ids2[0]); !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	for _, path := range []string{"/bucket-x/" + o2.Prefix + "packs/" + ids2[0], "/bucket-x/" + o2.Prefix + "entries/" + ids2[0]} {
		h := f2.headers[path]
		if got := h.Get("X-Amz-Checksum-Sha256"); got != "" {
			t.Fatalf("%s: x-amz-checksum-sha256 sent with the flag off: %q", path, got)
		}
		signed := signedHeaders(h)
		if slices.Contains(signed, "x-amz-checksum-sha256") {
			t.Fatalf("%s: signed headers %v hold x-amz-checksum-sha256", path, signed)
		}
		if h.Get("Content-Md5") == "" || h.Get("X-Amz-Content-Sha256") == "" || !slices.Contains(signed, "content-md5") || !slices.Contains(signed, "x-amz-content-sha256") {
			t.Fatalf("%s: Content-MD5 or payload hash missing or unsigned: %v", path, signed)
		}
	}
}

func TestIfNoneMatchSignedAnd412Stops(t *testing.T) {
	// With --if-none-match every PUT carries If-None-Match: * under the
	// signature.
	f := newFakeS3(t)
	o, ids := setup(t, f, 1)
	o.IfNoneMatch = true
	if _, err := run(o); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/bucket-x/" + o.Prefix + "packs/" + ids[0], "/bucket-x/" + o.Prefix + "entries/" + ids[0]} {
		h := f.headers[path]
		if h.Get("If-None-Match") != "*" {
			t.Fatalf("%s: If-None-Match %q", path, h.Get("If-None-Match"))
		}
		auth := h.Get("Authorization")
		_, after, _ := strings.Cut(auth, "SignedHeaders=")
		signed, _, _ := strings.Cut(after, ",")
		if !slices.Contains(strings.Split(signed, ";"), "if-none-match") {
			t.Fatalf("%s: signed headers %q lack if-none-match", path, signed)
		}
	}

	// Without it, the default, the header is absent.
	f2 := newFakeS3(t)
	o2, ids2 := setup(t, f2, 1)
	if _, err := run(o2); err != nil {
		t.Fatal(err)
	}
	if h := f2.headers["/bucket-x/"+o2.Prefix+"packs/"+ids2[0]]; h.Get("If-None-Match") != "" {
		t.Fatalf("If-None-Match sent with the flag off: %q", h.Get("If-None-Match"))
	}

	// A pack the bucket holds gets 412, which stops the run at once and
	// names the key.
	f3 := newFakeS3(t)
	o3, ids3 := setup(t, f3, 2)
	key := o3.Prefix + "packs/" + ids3[0]
	f3.stored["/bucket-x/"+key] = true
	o3.IfNoneMatch = true
	_, err := run(o3)
	if err == nil || !strings.Contains(err.Error(), key+" exists in the bucket; not overwritten") {
		t.Fatalf("run: %v", err)
	}
	if got, want := f3.requests(), puts(o3.Prefix, ids3[0])[:1]; !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	done, err := readManifest(o3.Manifest, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := done[key]; ok {
		t.Fatalf("412 key %s recorded as done", key)
	}
}

func TestResumeAfterPackPutsOnlyEntry(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 1)
	o.IfNoneMatch = true

	// The first run puts the pack and fails on the entry.
	entryPath := "/bucket-x/" + o.Prefix + "entries/" + ids[0]
	f.fail[entryPath] = http.StatusInternalServerError
	if _, err := run(o); err == nil {
		t.Fatal("first run succeeded")
	}

	// The resume puts only the entry; a pack PUT would get 412.
	delete(f.fail, entryPath)
	f.reset()
	o.Resume = true
	rep, err := run(o)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.requests(), puts(o.Prefix, ids[0])[1:]; !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
	if rep.Objects != 1 || rep.SkippedObjects != 1 || rep.Skipped != 0 {
		t.Fatalf("report %+v", rep)
	}
}

func TestManifestTornLineDropped(t *testing.T) {
	f := newFakeS3(t)
	o, ids := setup(t, f, 2)

	// The first run puts the first id, then a crash tears a line.
	idsFile := filepath.Join(t.TempDir(), "ids")
	if err := os.WriteFile(idsFile, []byte(ids[0]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o.IDs = idsFile
	if _, err := run(o); err != nil {
		t.Fatal(err)
	}
	mf, err := os.OpenFile(o.Manifest, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	torn := "2026-11-10T00:00:00Z\t" + o.Prefix + "packs/" + ids[1] + "\t12"
	if _, err := mf.WriteString(torn); err != nil {
		t.Fatal(err)
	}
	_ = mf.Close()

	// The resume warns, drops the torn line, and puts the second id.
	f.reset()
	o.IDs, o.Resume = "", true
	if _, err := run(o); err != nil {
		t.Fatal(err)
	}
	if w := o.Warn.(*bytes.Buffer).String(); !strings.Contains(w, "torn last line") {
		t.Fatalf("warning %q", w)
	}
	if got, want := f.requests(), puts(o.Prefix, ids[1]); !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}

	// The torn bytes were cut, so every line is whole.
	raw, err := os.ReadFile(o.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), torn+"\n") || strings.Count(string(raw), "\n") != 4 {
		t.Fatalf("manifest %q", raw)
	}
	done, err := readManifest(o.Manifest, io.Discard)
	if err != nil || len(done) != 4 {
		t.Fatalf("manifest done %v %v", done, err)
	}
}

func TestManifestLockedRefused(t *testing.T) {
	f := newFakeS3(t)
	o, _ := setup(t, f, 1)

	// Another holder takes the lock on the manifest.
	mf, err := os.OpenFile(o.Manifest, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer mf.Close()
	if err := lockFile(mf); err != nil {
		t.Fatal(err)
	}

	// A run, a resume and a dry run each refuse to start.
	for _, mode := range []string{"run", "resume", "dry run"} {
		o.Resume, o.DryRun = mode == "resume", mode == "dry run"
		if _, err := run(o); err == nil || !strings.Contains(err.Error(), "locked by another run") {
			t.Fatalf("%s: %v", mode, err)
		}
	}
	if r := f.requests(); len(r) != 0 {
		t.Fatalf("requests sent: %v", r)
	}

	// Once the holder is gone, the run goes ahead.
	_ = mf.Close()
	o.Resume, o.DryRun = false, false
	if _, err := run(o); err != nil {
		t.Fatal(err)
	}
}
