package v86_wazero

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_state "github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

const (
	DefaultCdnBaseURL       = "https://cdn-staging.spacewave.app"
	DefaultCdnSpaceID       = "01kpn3x0y79yr94ps1yae206vp"
	DefaultV86ImageKey      = "v86image-01kszf4rsev1s7zkq2ms2y5r0w"
	DefaultAssetCacheSubdir = ".tmp/v86-wazero"
)

// AssetSet is the materialized v86 boot image used by the wazero harness.
type AssetSet struct {
	Dir             string
	ImageKey        string
	Wasm            string
	SeaBIOS         string
	VGABIOS         string
	Kernel          string
	RootfsTar       string
	RootfsJSON      string
	RootfsFlatDir   string
	RootfsObjectKey string
}

// AssetOptions configures where the harness finds the real v86 image.
type AssetOptions struct {
	CacheDir string
	AssetDir string
	V86Dir   string
	V86FSDir string

	CdnBaseURL string
	CdnSpaceID string
	ImageKey   string
	Refresh    bool
}

func OptionsFromEnv() AssetOptions {
	refresh := strings.EqualFold(strings.TrimSpace(os.Getenv("V86_WAZERO_REFRESH")), "true")
	return AssetOptions{
		CacheDir:   strings.TrimSpace(os.Getenv("V86_WAZERO_CACHE_DIR")),
		AssetDir:   strings.TrimSpace(os.Getenv("V86_WAZERO_ASSET_DIR")),
		V86Dir:     strings.TrimSpace(os.Getenv("V86_DIR")),
		V86FSDir:   strings.TrimSpace(os.Getenv("V86FS_DIR")),
		CdnBaseURL: strings.TrimSpace(os.Getenv("V86_WAZERO_CDN_BASE_URL")),
		CdnSpaceID: strings.TrimSpace(os.Getenv("V86_WAZERO_CDN_SPACE_ID")),
		ImageKey:   strings.TrimSpace(os.Getenv("V86_WAZERO_IMAGE_KEY")),
		Refresh:    refresh,
	}
}

func ResolveAssets(ctx context.Context, opts AssetOptions) (*AssetSet, error) {
	opts = opts.withDefaults()
	if opts.AssetDir != "" {
		if assets, ok := assetSetFromDir(opts.AssetDir, opts.ImageKey); ok {
			return assets, nil
		}
	}
	if opts.V86Dir != "" && opts.V86FSDir != "" {
		if assets, ok := assetSetFromV86Dirs(opts.V86Dir, opts.V86FSDir, opts.ImageKey); ok {
			return assets, nil
		}
	}
	if !opts.Refresh {
		if assets, ok := assetSetFromDir(opts.CacheDir, opts.ImageKey); ok {
			return assets, nil
		}
	}
	return hydrateAssetsFromCdn(ctx, opts)
}

func (o AssetOptions) withDefaults() AssetOptions {
	if o.CdnBaseURL == "" {
		o.CdnBaseURL = DefaultCdnBaseURL
	}
	if o.CdnSpaceID == "" {
		o.CdnSpaceID = DefaultCdnSpaceID
	}
	if o.ImageKey == "" {
		o.ImageKey = DefaultV86ImageKey
	}
	if o.CacheDir == "" {
		o.CacheDir = filepath.Join(repoRootOrCwd(), DefaultAssetCacheSubdir)
	}
	return o
}

func repoRootOrCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "bldr.star")); err == nil {
				return dir
			}
		}
		next := filepath.Dir(dir)
		if next == dir {
			return wd
		}
	}
}

func assetSetFromDir(dir, imageKey string) (*AssetSet, bool) {
	assets := &AssetSet{
		Dir:           dir,
		ImageKey:      imageKey,
		Wasm:          filepath.Join(dir, "v86.wasm"),
		SeaBIOS:       filepath.Join(dir, "seabios.bin"),
		VGABIOS:       filepath.Join(dir, "vgabios.bin"),
		Kernel:        filepath.Join(dir, "bzImage"),
		RootfsTar:     filepath.Join(dir, "rootfs.tar"),
		RootfsJSON:    filepath.Join(dir, "fs.json"),
		RootfsFlatDir: filepath.Join(dir, "flat"),
	}
	if rootfsKey, err := os.ReadFile(filepath.Join(dir, "rootfs.object-key")); err == nil {
		assets.RootfsObjectKey = strings.TrimSpace(string(rootfsKey))
	}
	if filesExist(assets.Wasm, assets.SeaBIOS, assets.VGABIOS, assets.Kernel) &&
		(assets.RootfsObjectKey != "" || filesExist(assets.RootfsTar)) {
		return assets, true
	}
	return nil, false
}

func assetSetFromV86Dirs(v86Dir, v86fsDir, imageKey string) (*AssetSet, bool) {
	wasm := filepath.Join(v86Dir, "build", "v86.wasm")
	if _, err := os.Stat(wasm); err != nil {
		wasm = filepath.Join(v86Dir, "build", "v86-debug.wasm")
	}
	assets := &AssetSet{
		Dir:           filepath.Dir(filepath.Dir(wasm)),
		ImageKey:      imageKey,
		Wasm:          wasm,
		SeaBIOS:       filepath.Join(v86Dir, "bios", "seabios.bin"),
		VGABIOS:       filepath.Join(v86Dir, "bios", "vgabios.bin"),
		Kernel:        filepath.Join(v86fsDir, "bzImage"),
		RootfsTar:     filepath.Join(v86fsDir, "rootfs.tar"),
		RootfsJSON:    filepath.Join(v86fsDir, "fs.json"),
		RootfsFlatDir: filepath.Join(v86fsDir, "flat"),
	}
	if filesExist(assets.Wasm, assets.SeaBIOS, assets.VGABIOS, assets.Kernel, assets.RootfsTar) {
		return assets, true
	}
	return nil, false
}

func filesExist(paths ...string) bool {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func hydrateAssetsFromCdn(ctx context.Context, opts AssetOptions) (*AssetSet, error) {
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create v86 wazero cache dir")
	}
	ws, release, err := mountCdnWorld(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer release()

	imageKey, err := resolveV86ImageKey(ctx, ws, opts.ImageKey)
	if err != nil {
		return nil, err
	}
	specs := []assetSpec{
		{Pred: string(s4wave_vm.PredV86ImageWasm), FileName: "v86.wasm", OutName: "v86.wasm"},
		{Pred: string(s4wave_vm.PredV86ImageBiosSeabios), FileName: "seabios.bin", OutName: "seabios.bin"},
		{Pred: string(s4wave_vm.PredV86ImageBiosVgabios), FileName: "vgabios.bin", OutName: "vgabios.bin"},
		{Pred: string(s4wave_vm.PredV86ImageKernel), FileName: "bzImage", OutName: "bzImage"},
	}
	for _, spec := range specs {
		if err := writeCdnAsset(ctx, ws, imageKey, spec, opts.CacheDir); err != nil {
			return nil, err
		}
	}
	rootfsKey, err := lookupEdge(ctx, ws, imageKey, string(s4wave_vm.PredV86ImageRootfs))
	if err != nil {
		return nil, err
	}
	if rootfsKey == "" {
		return nil, errors.Errorf("v86 image %q missing %s edge", imageKey, s4wave_vm.PredV86ImageRootfs.String())
	}
	if err := os.WriteFile(filepath.Join(opts.CacheDir, "rootfs.object-key"), []byte(rootfsKey+"\n"), 0o644); err != nil {
		return nil, errors.Wrap(err, "write rootfs object key marker")
	}
	assets, ok := assetSetFromDir(opts.CacheDir, imageKey)
	if !ok {
		return nil, errors.Errorf("hydrated v86 assets incomplete in %s", opts.CacheDir)
	}
	assets.RootfsObjectKey = rootfsKey
	return assets, nil
}

func mountCdnWorld(ctx context.Context, opts AssetOptions) (world_state.WorldState, func(), error) {
	store, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: opts.CdnBaseURL,
		SpaceID:    opts.CdnSpaceID,
		PointerTTL: -1,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "build cdn block store")
	}
	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    opts.CdnSpaceID,
		BlockStore: store,
	})
	if err != nil {
		store.Close()
		return nil, nil, errors.Wrap(err, "build cdn shared object")
	}
	le := logrus.NewEntry(logrus.StandardLogger())
	we, err := cdn_sharedobject.NewWorldEngine(ctx, le, nil, so, space_world_optypes.LookupWorldOp)
	if err != nil {
		store.Close()
		return nil, nil, errors.Wrap(err, "mount cdn world")
	}
	ws := world_state.NewEngineWorldState(we.Engine, true)
	return ws, func() {
		we.Release()
		store.Close()
	}, nil
}

func resolveV86ImageKey(ctx context.Context, ws world_state.WorldState, preferred string) (string, error) {
	if preferred != "" {
		if _, found, err := ws.GetObject(ctx, preferred); err != nil {
			return "", errors.Wrap(err, "probe preferred v86 image")
		} else if found {
			return preferred, nil
		}
	}
	keys, err := world_types.ListObjectsWithType(ctx, ws, s4wave_vm.V86ImageTypeID)
	if err != nil {
		return "", errors.Wrap(err, "list cdn v86 images")
	}
	if len(keys) == 0 {
		return "", errors.New("cdn space has no V86Image objects")
	}
	slices.Sort(keys)
	return keys[len(keys)-1], nil
}

type assetSpec struct {
	Pred     string
	FileName string
	OutName  string
}

func writeCdnAsset(ctx context.Context, ws world_state.WorldState, imageKey string, spec assetSpec, dir string) error {
	assetKey, err := lookupEdge(ctx, ws, imageKey, spec.Pred)
	if err != nil {
		return err
	}
	if assetKey == "" {
		return errors.Errorf("v86 image %q missing %s edge", imageKey, spec.Pred)
	}
	data, err := readUnixFSAsset(ctx, ws, assetKey, spec.FileName)
	if err != nil {
		return errors.Wrapf(err, "read %s asset object %q", spec.FileName, assetKey)
	}
	return os.WriteFile(filepath.Join(dir, spec.OutName), data, 0o644)
}

func lookupEdge(ctx context.Context, ws world_state.WorldState, subject, pred string) (string, error) {
	quads, err := ws.LookupGraphQuads(ctx, world_state.NewGraphQuadWithKeys(subject, pred, "", ""), 1)
	if err != nil {
		return "", errors.Wrapf(err, "lookup %s edge", pred)
	}
	if len(quads) == 0 {
		return "", nil
	}
	return world_state.GraphValueToKey(quads[0].GetObj())
}

func readUnixFSAsset(ctx context.Context, ws world_state.WorldState, objectKey, fileName string) ([]byte, error) {
	fsh, err := openFSHandleForObject(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	defer fsh.Release()

	bfs := unixfs_billy.NewBillyFS(ctx, fsh, "", time.Time{})
	if data, err := billy_util.ReadFile(bfs, fileName); err == nil {
		return data, nil
	}
	entries, err := bfs.ReadDir(".")
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 || entries[0].IsDir() {
		return nil, errors.Errorf("asset object %q does not contain %q or a single file", objectKey, fileName)
	}
	return billy_util.ReadFile(bfs, entries[0].Name())
}

func openFSHandleForObject(ctx context.Context, ws world_state.WorldState, objectKey string) (*unixfs.FSHandle, error) {
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "lookup fs type")
	}
	le := logrus.NewEntry(logrus.StandardLogger())
	fsCursor := unixfs_world.NewFSCursor(le, ws, objectKey, fsType, nil, false)
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, errors.Wrap(err, "create fs handle")
	}
	return fsh, nil
}
