package bldr_manifest

import (
	"context"
	"io/fs"
	"strconv"
	"strings"

	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/aperturerobotics/hydra/block"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v5"
	"github.com/pkg/errors"
)

// ValidateManifestID validates a manifest ID.
func ValidateManifestID(id string, allowEmpty bool) error {
	if id == "" {
		if allowEmpty {
			return nil
		}
		return ErrEmptyManifestID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "manifest_id")
	}
	return nil
}

// Validate validates the request.
func (f *FetchManifestRequest) Validate(allowEmptyID bool) error {
	if err := f.GetManifestMeta().Validate(allowEmptyID); err != nil {
		return err
	}
	return nil
}

// Validate validates the FetchManifest response.
func (r *FetchManifestResponse) Validate() error {
	if !r.GetRemoved() && r.GetValueId() != 0 {
		if err := r.GetValue().Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates the FetchManifestValue.
func (r *FetchManifestValue) Validate() error {
	if r.GetManifestRef().GetEmpty() {
		return errors.New("manifest_ref: cannot be empty")
	}
	if err := r.GetManifestRef().Validate(); err != nil {
		return errors.Wrap(err, "manifest_ref")
	}
	return nil
}

// NewManifest constructs a new Manifest.
func NewManifest(meta *ManifestMeta, entrypoint string) *Manifest {
	return &Manifest{
		Meta:       meta,
		Entrypoint: entrypoint,
	}
}

// NewManifestBlock constructs a new Manifest block.
func NewManifestBlock() block.Block {
	return &Manifest{}
}

// NewManifestKey builds a key for a manifest associated with another object.
func NewManifestKey(baseObjKey string, manifestMeta *ManifestMeta) string {
	buildType := manifestMeta.GetBuildType()
	if buildType == "" {
		buildType = string(BuildType_DEV)
	}
	manifestKeyPts := []string{
		baseObjKey,
		"manifest",
		manifestMeta.GetManifestId(),
		buildType,
		strconv.FormatUint(manifestMeta.GetRev(), 10),
	}
	if platformID := manifestMeta.GetPlatformId(); platformID != "" {
		manifestKeyPts = append(manifestKeyPts, platformID)
	}
	return strings.Join(manifestKeyPts, "/")
}

// UnmarshalManifest unmarshals a Manifest block from the cursor.
func UnmarshalManifest(ctx context.Context, bcs *block.Cursor) (*Manifest, error) {
	return block.UnmarshalBlock[*Manifest](ctx, bcs, NewManifestBlock)
}

// CreateManifestWithBilly creates the manifest at the block cursor with billy filesystem instances.
// Note: supports symlinks if the fs implements billy.Symlink.
// Note: nil fs will leave that portion of the manifest empty.
func CreateManifestWithBilly(
	ctx context.Context,
	bcs *block.Cursor,
	meta *ManifestMeta,
	entrypoint string,
	distFs, assetsFs billy.Filesystem,
	ts *timestamp.Timestamp,
) (*Manifest, error) {
	manifest := NewManifest(meta, entrypoint)
	bcs.SetBlock(manifest, true)

	// setup the distribution filesystem.
	if err := unixfs_block.CreateFromBillyFS(ctx, bcs.FollowRef(3, nil), distFs, ts); err != nil {
		return nil, err
	}

	// setup the assets filesystem.
	if err := unixfs_block.CreateFromBillyFS(ctx, bcs.FollowRef(4, nil), assetsFs, ts); err != nil {
		return nil, err
	}

	// done
	return manifest, nil
}

// CreateManifestWithIoFS creates the manifest at the block cursor with io/fs.FS instances.
// Note: does not yet support symlinks: https://github.com/golang/go/issues/49580
func CreateManifestWithIoFS(
	ctx context.Context,
	bcs *block.Cursor,
	meta *ManifestMeta,
	entrypoint string,
	distFs, assetsFs fs.FS,
	ts *timestamp.Timestamp,
) (*Manifest, error) {
	manifest := NewManifest(meta, entrypoint)
	bcs.SetBlock(manifest, true)

	// setup the distribution filesystem.
	if err := unixfs_block.CreateFromFS(ctx, bcs.FollowRef(3, nil), distFs, ts); err != nil {
		return nil, err
	}

	// setup the assets filesystem.
	if err := unixfs_block.CreateFromFS(ctx, bcs.FollowRef(4, nil), assetsFs, ts); err != nil {
		return nil, err
	}

	// done
	return manifest, nil
}

// Validate validates the Manifest.
func (m *Manifest) Validate() error {
	if err := m.GetMeta().Validate(false); err != nil {
		return errors.Wrap(err, "meta")
	}
	if err := m.GetDistFsRef().Validate(true); err != nil {
		return errors.Wrap(err, "dist_fs_ref")
	}
	if err := m.GetAssetsFsRef().Validate(true); err != nil {
		return errors.Wrap(err, "assets_fs_ref")
	}
	if m.GetEntrypoint() == "" {
		return ErrEmptyEntrypoint
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (m *Manifest) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *Manifest) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (m *Manifest) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 3:
		m.DistFsRef = ptr
	case 4:
		m.AssetsFsRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (m *Manifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	n := make(map[uint32]*block.BlockRef)
	n[3] = m.GetDistFsRef()
	n[4] = m.GetAssetsFsRef()
	return n, nil
}

// FollowDistFs follows the DistFsRef.
func (m *Manifest) FollowDistFs(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(3, m.GetDistFsRef())
}

// FollowAssetsFs follows the AssetsFsRef.
func (m *Manifest) FollowAssetsFsRef(bcs *block.Cursor) *block.Cursor {
	return bcs.FollowRef(4, m.GetAssetsFsRef())
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (m *Manifest) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 3:
		return unixfs_block.NewFSNodeBlock
	case 4:
		return unixfs_block.NewFSNodeBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = ((*Manifest)(nil))
	_ block.BlockWithRefs = ((*Manifest)(nil))
)
