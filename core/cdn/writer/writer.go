package cdn_writer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_rootserve "github.com/s4wave/spacewave/core/cdn/rootserve"
	"github.com/s4wave/spacewave/core/sobject"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// UploadExpiry is how long a presigned upload admits its pack.
const UploadExpiry = 15 * time.Minute

// minAdmittedSweep is the admission count below which a push never sweeps
// expired admissions.
const minAdmittedSweep = 1024

// packHeader is stored with every pack. A pack's ID names its content, so
// its bytes never change and an edge caches it for a year.
var packHeader = http.Header{
	"Content-Type":  {"application/octet-stream"},
	"Cache-Control": {cdn_rootserve.PackCacheControl},
}

// Config configures a Writer.
type Config struct {
	// Store holds each owned Space's pointer.
	Store Store
	// Bucket writes and reads the bucket holding the Spaces' packs.
	Bucket *block_store_s3.Client
	// BucketName names that bucket.
	BucketName string
	// Prefix is prepended to every object key, such as a staging prefix.
	Prefix string
	// Roots serves the owned Spaces' pointers to readers. Optional.
	Roots *cdn_rootserve.Handler
	// SigningEnv is the environment prefix writers sign requests with. Use
	// one no Spacewave Cloud environment uses, so a request signed for one is
	// not taken by the other.
	SigningEnv string
	// BasePath is the path the routes sit under, as the session client's
	// endpoint names it, or empty for the origin root.
	BasePath string
	// Logger logs imports and commits. Optional.
	Logger *logrus.Entry
}

// Writer writes the CDN Spaces it owns. A Space's pointer lives in the
// Store and in memory; its packs live in the bucket under the paths readers
// use.
type Writer struct {
	// cfg is the configuration.
	cfg Config
	// le logs imports and commits.
	le *logrus.Entry
	// mux routes the session API.
	mux *http.ServeMux
	// now reads the clock.
	now func() time.Time

	// mtx guards spaces, admitted and sweepAt.
	mtx sync.Mutex
	// spaces holds each owned Space.
	spaces map[string]*ownedSpace
	// admitted holds each pack granted an upload, keyed by its bucket key,
	// until it commits or its upload expires.
	admitted map[string]admittedPack
	// sweepAt is the admission count at which a push sweeps expired ones.
	sweepAt int
}

// ownedSpace is one Space the writer owns.
type ownedSpace struct {
	// commitMtx serializes commits to the Space from the Store write through
	// the served pointer, so a slower commit never serves an older pointer.
	commitMtx sync.Mutex

	// The fields below are guarded by Writer.mtx.

	// root is the current pointer; it is replaced, never changed.
	root *cdn.CdnRootPointer
	// listed holds the ID of each pack root lists.
	listed map[string]struct{}
	// writers are the Space's fixed writers, whose checkpoints the recorded
	// owners sign.
	writers []peer.ID
	// admitted are writers admitted at runtime and when each admission ends.
	// Each signs the checkpoints it posts.
	admitted map[peer.ID]time.Time
}

// admittedPack is a pack granted an upload and not yet committed.
type admittedPack struct {
	// spaceID is the Space the pack joins.
	spaceID string
	// entry is the pack's catalog entry.
	entry *packfile.PackfileEntry
	// sum is the SHA-256 of the pack's bytes.
	sum []byte
	// expires is when the upload ends.
	expires time.Time
}

// New returns a writer with cfg that owns no Space yet.
func New(cfg Config) (*Writer, error) {
	// Require the store, the bucket and a signing environment.
	if cfg.Store == nil || cfg.Bucket == nil || cfg.BucketName == "" {
		return nil, errors.New("a CDN writer needs a store and a bucket")
	}
	if cfg.SigningEnv == "" {
		return nil, errors.New("a CDN writer needs a signing environment")
	}
	le := cfg.Logger
	if le == nil {
		le = logrus.NewEntry(logrus.StandardLogger())
	}

	// Route the session API.
	w := &Writer{
		cfg:      cfg,
		le:       le,
		mux:      http.NewServeMux(),
		now:      time.Now,
		spaces:   make(map[string]*ownedSpace),
		admitted: make(map[string]admittedPack),
		sweepAt:  minAdmittedSweep,
	}
	w.routes()
	return w, nil
}

// Own makes the writer the writer of spaceID with the given fixed writers.
// The first time, it imports the pointer the bucket holds into the Store.
func (w *Writer) Own(ctx context.Context, spaceID string, writers ...peer.ID) error {
	// Read the recorded pointer, importing the bucket's when there is none.
	root, err := w.cfg.Store.Root(ctx, spaceID)
	if err != nil {
		return errors.Wrapf(err, "read CDN Space %s", spaceID)
	}
	if root == nil {
		imported, err := w.readBucketRoot(ctx, spaceID)
		if err != nil {
			return errors.Wrapf(err, "import CDN Space %s", spaceID)
		}
		if root, err = w.cfg.Store.Import(ctx, imported); err != nil {
			return errors.Wrapf(err, "import CDN Space %s", spaceID)
		}
		w.le.WithField("space", spaceID).WithField("packs", len(root.GetPacks())).Info("imported a CDN Space from the bucket")
	}
	return w.serve(spaceID, root, writers)
}

// Create makes the writer the writer of the Space genesis starts, such as
// one NewGenesis returns, with the given fixed writers. The first time, it
// records genesis in the Store; later it serves the recorded pointer.
func (w *Writer) Create(ctx context.Context, genesis *cdn.CdnRootPointer, writers ...peer.ID) error {
	// Record the genesis unless the Store holds the Space.
	spaceID := genesis.GetSpaceId()
	if spaceID == "" {
		return errors.New("a created CDN Space needs a Space ID")
	}
	root, err := w.cfg.Store.Import(ctx, genesis)
	if err != nil {
		return errors.Wrapf(err, "create CDN Space %s", spaceID)
	}
	return w.serve(spaceID, root, writers)
}

// Admit lets writer publish to an owned Space until expires. A writer
// admitted this way signs the checkpoints it posts with its own key.
func (w *Writer) Admit(spaceID string, writer peer.ID, expires time.Time) error {
	// Record the admission on the owned Space.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	sp := w.spaces[spaceID]
	if sp == nil {
		return ErrSpaceUnknown
	}
	sp.admitted[writer] = expires
	return nil
}

// Root returns the current pointer of an owned Space, which the caller must
// not change, or nil.
func (w *Writer) Root(spaceID string) *cdn.CdnRootPointer {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if sp := w.spaces[spaceID]; sp != nil {
		return sp.root
	}
	return nil
}

// serve records root as the current pointer of an owned Space and serves it.
func (w *Writer) serve(spaceID string, root *cdn.CdnRootPointer, writers []peer.ID) error {
	// Hand the pointer to readers first: it is durable in the Store.
	if root.GetSpaceId() != spaceID {
		return errors.Errorf("pointer names Space %q, not %q", root.GetSpaceId(), spaceID)
	}
	if w.cfg.Roots != nil {
		if err := w.cfg.Roots.SetOwned(root); err != nil {
			return err
		}
	}

	// Index its packs and take its writers.
	listed := make(map[string]struct{}, len(root.GetPacks()))
	for _, pack := range root.GetPacks() {
		listed[pack.GetId()] = struct{}{}
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()
	sp := w.spaces[spaceID]
	if sp == nil {
		sp = &ownedSpace{admitted: make(map[peer.ID]time.Time)}
		w.spaces[spaceID] = sp
	}
	sp.root, sp.listed, sp.writers = root, listed, slices.Clone(writers)
	return nil
}

// space returns an owned Space.
func (w *Writer) space(spaceID string) (*ownedSpace, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if sp := w.spaces[spaceID]; sp != nil {
		return sp, nil
	}
	return nil, ErrSpaceUnknown
}

// readBucketRoot reads the pointer the bucket holds for spaceID.
func (w *Writer) readBucketRoot(ctx context.Context, spaceID string) (*cdn.CdnRootPointer, error) {
	// Read the encoded pointer within the reader's size limit.
	body, err := w.cfg.Bucket.GetObject(ctx, w.cfg.BucketName, w.cfg.Prefix+spaceID+"/"+cdn_rootserve.PointerName)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, cdn_bstore.MaxRootPackedmsgBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > cdn_bstore.MaxRootPackedmsgBytes {
		return nil, errors.New("root pointer exceeds the size limit")
	}

	// Decode it and check it names the Space.
	raw, ok := packedmsg.DecodePackedMessage(string(data))
	if !ok {
		return nil, errors.New("decode root pointer: checksum mismatch")
	}
	root := &cdn.CdnRootPointer{}
	if err := root.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "decode root pointer")
	}
	if root.GetSpaceId() != spaceID {
		return nil, errors.Errorf("root pointer names Space %q", root.GetSpaceId())
	}
	return root, nil
}

// PackKey is the bucket key of a pack of spaceID, under the paths readers
// fetch packs from.
func (w *Writer) PackKey(spaceID, packID string) string {
	shard := packID
	if len(shard) > 2 {
		shard = shard[:2]
	}
	return w.cfg.Prefix + spaceID + "/packs/" + shard + "/" + packID + ".kvf"
}

// admitPack grants an upload of the pack req describes to spaceID, or
// reports that the Space already lists it. The pack joins the Space when
// commitPack finds its bytes in the bucket.
func (w *Writer) admitPack(spaceID string, req *packfile.PushRequest, now time.Time) (*packfile.PushResponse, error) {
	// Check the request describes a whole pack that adds to the Space.
	if len(req.GetReplacesPackIds()) != 0 {
		return nil, ErrPackReplaced
	}
	packID := req.GetPackId()
	if !validPackID(packID) || req.GetSizeBytes() == 0 || req.GetBlockCount() == 0 || len(req.GetSha256()) != sha256.Size || len(req.GetBloomFilter()) == 0 || req.GetBloomFormatVersion() == 0 {
		return nil, errors.New("a pushed pack needs an ID, a size, a block count, a SHA-256 and a Bloom filter")
	}

	// Answer a listed pack without an upload.
	resp := &packfile.PushResponse{PackId: packID, SizeBytes: req.GetSizeBytes()}
	w.mtx.Lock()
	sp := w.spaces[spaceID]
	var listed bool
	if sp != nil {
		_, listed = sp.listed[packID]
	}
	w.mtx.Unlock()
	if sp == nil {
		return nil, ErrSpaceUnknown
	}
	if listed {
		resp.AlreadyExists = true
		return resp, nil
	}

	// Presign the upload and remember the pack until it commits, sweeping
	// expired admissions only once the map has doubled since the last sweep.
	key := w.PackKey(spaceID, packID)
	url, header := w.cfg.Bucket.PresignPutHeader(w.cfg.BucketName, key, int64(req.GetSizeBytes()), req.GetSha256(), packHeader, UploadExpiry, now) //nolint:gosec // a pack's size fits an int64.
	expires := now.Add(UploadExpiry)
	w.mtx.Lock()
	if len(w.admitted) >= w.sweepAt {
		for k, pack := range w.admitted {
			if !now.Before(pack.expires) {
				delete(w.admitted, k)
			}
		}
		w.sweepAt = max(minAdmittedSweep, 2*len(w.admitted))
	}
	w.admitted[key] = admittedPack{
		spaceID: spaceID,
		entry: &packfile.PackfileEntry{
			Id:                 packID,
			BloomFilter:        slices.Clone(req.GetBloomFilter()),
			BlockCount:         req.GetBlockCount(),
			SizeBytes:          req.GetSizeBytes(),
			BloomFormatVersion: req.GetBloomFormatVersion(),
		},
		sum:     slices.Clone(req.GetSha256()),
		expires: expires,
	}
	w.mtx.Unlock()

	// Return the URL with the headers it was signed for.
	resp.Upload = &packfile.PushUpload{Url: url, Headers: make(map[string]string, len(header)), ExpiresAt: timestamppb.New(expires)}
	for name := range header {
		resp.Upload.Headers[name] = header.Get(name)
	}
	return resp, nil
}

// commitPack adds an admitted pack to spaceID once the bucket holds its
// exact bytes. It reads the pack's own object and nothing else in the
// bucket.
func (w *Writer) commitPack(ctx context.Context, spaceID, packID string, now time.Time) error {
	// Find the pack's live admission. A pack the Space lists already
	// committed, so a retry whose first answer was lost succeeds.
	key := w.PackKey(spaceID, packID)
	w.mtx.Lock()
	pack, ok := w.admitted[key]
	var listed bool
	if sp := w.spaces[spaceID]; sp != nil {
		_, listed = sp.listed[packID]
	}
	w.mtx.Unlock()
	if listed {
		return nil
	}
	if !ok || pack.spaceID != spaceID || !now.Before(pack.expires) {
		return ErrPackNotAdmitted
	}

	// Check the stored size and digest, hashing the object where the bucket
	// reports no checksum.
	stat, err := w.cfg.Bucket.HeadObject(ctx, w.cfg.BucketName, key)
	if err != nil {
		return errors.Wrapf(err, "stat pack %s", packID)
	}
	if uint64(stat.Size) != pack.entry.GetSizeBytes() { //nolint:gosec // a stored size is never negative.
		return errors.Wrapf(ErrPackMismatch, "pack %s holds %d bytes, not %d", packID, stat.Size, pack.entry.GetSizeBytes())
	}
	sum := stat.ChecksumSHA256
	if len(sum) == 0 {
		if sum, err = w.hashObject(ctx, key); err != nil {
			return errors.Wrapf(err, "hash pack %s", packID)
		}
	}
	if !bytes.Equal(sum, pack.sum) {
		return errors.Wrapf(ErrPackMismatch, "pack %s does not match its SHA-256", packID)
	}

	// List the pack, then forget the admission.
	if err := w.commit(ctx, &Commit{SpaceID: spaceID, Packs: []*packfile.PackfileEntry{pack.entry}, Now: now}); err != nil {
		return err
	}
	w.mtx.Lock()
	delete(w.admitted, key)
	w.mtx.Unlock()
	return nil
}

// ErrPackMismatch refuses a commit whose stored pack differs in size or
// SHA-256 from the pack the push declared.
var ErrPackMismatch = errors.New("the stored pack differs from the pushed pack")

// hashObject returns the SHA-256 of an object's bytes, streamed.
func (w *Writer) hashObject(ctx context.Context, key string) ([]byte, error) {
	// Stream the object through SHA-256.
	body, err := w.cfg.Bucket.GetObject(ctx, w.cfg.BucketName, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, body); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// postCheckpoint moves spaceID to checkpoint. A fixed writer's checkpoint
// carries the recorded owners' signatures; a writer admitted at runtime
// signs its own.
func (w *Writer) postCheckpoint(ctx context.Context, spaceID string, signer peer.ID, checkpoint *sobject.SOCheckpoint) error {
	// Pick the signature the checkpoint must carry.
	sp, err := w.space(spaceID)
	if err != nil {
		return err
	}
	w.mtx.Lock()
	fixed := slices.Contains(sp.writers, signer)
	w.mtx.Unlock()
	c := &Commit{SpaceID: spaceID, Checkpoint: checkpoint, Now: w.now()}
	if !fixed {
		c.Signer = signer.String()
	}
	return w.commit(ctx, c)
}

// commit applies c in the Store and serves the new pointer. Commits to one
// Space run one at a time from the Store write through the served pointer.
func (w *Writer) commit(ctx context.Context, c *Commit) error {
	// Hold the Space's commit slot.
	sp, err := w.space(c.SpaceID)
	if err != nil {
		return err
	}
	sp.commitMtx.Lock()
	defer sp.commitMtx.Unlock()

	// Write the pointer durably, then serve it.
	root, err := w.cfg.Store.Commit(ctx, c)
	if err != nil {
		return err
	}
	if w.cfg.Roots != nil {
		if err := w.cfg.Roots.SetOwned(root); err != nil {
			return err
		}
	}

	// Index the added packs.
	w.mtx.Lock()
	sp.root = root
	for _, pack := range c.Packs {
		sp.listed[pack.GetId()] = struct{}{}
	}
	w.mtx.Unlock()
	w.le.WithFields(logrus.Fields{
		"space":       c.SpaceID,
		"added":       len(c.Packs),
		"space-packs": len(root.GetPacks()),
		"checkpoint":  c.Checkpoint != nil,
	}).Debug("committed to a CDN Space")
	return nil
}

// mayWrite reports whether signer writes spaceID at now.
func (w *Writer) mayWrite(spaceID string, signer peer.ID, now time.Time) bool {
	// Take a fixed writer, or an admitted one before its admission ends.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	sp := w.spaces[spaceID]
	if sp == nil {
		return false
	}
	if slices.Contains(sp.writers, signer) {
		return true
	}
	expires, ok := sp.admitted[signer]
	return ok && now.Before(expires)
}

// validPackID reports whether id can name a pack's bucket key: letters,
// digits, '-', '_' and '.', not starting with '.', at most 128 of them.
func validPackID(id string) bool {
	if id == "" || id[0] == '.' || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}
