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
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_state "github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// ResolveAssets locates a complete local v86 image or downloads it from the CDN.
func ResolveAssets(ctx context.Context, opts AssetOptions) (*AssetSet, error) {
	// Prefer explicitly supplied local images over downloaded cache entries.
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

	// Reuse a complete cached image unless a refresh was requested.
	if !opts.Refresh {
		if assets, ok := assetSetFromDir(opts.CacheDir, opts.ImageKey); ok {
			return assets, nil
		}
	}

	// Materialize the published image when no local source is usable.
	return hydrateAssetsFromCdn(ctx, opts)
}

// repoRootOrCwd finds the ancestor containing go.mod and bldr.star, or returns
// the current directory when no repository root is found.
func repoRootOrCwd() string {
	// Use the current directory as the search origin and fallback.
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}

	// Accept only an ancestor that owns both module and build configuration.
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

// assetSetFromDir returns the directory's boot files when the image is complete.
func assetSetFromDir(dir, imageKey string) (*AssetSet, bool) {
	// Resolve the conventional paths for a complete boot image.
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

	// A CDN rootfs reference can replace the local filesystem archive.
	if rootfsKey, err := os.ReadFile(filepath.Join(dir, "rootfs.object-key")); err == nil {
		assets.RootfsObjectKey = strings.TrimSpace(string(rootfsKey))
	}
	if filesExist(assets.Wasm, assets.SeaBIOS, assets.VGABIOS, assets.Kernel) &&
		(assets.RootfsObjectKey != "" || filesExist(assets.RootfsTar)) {
		return assets, true
	}
	return nil, false
}

// assetSetFromV86Dirs resolves boot files from v86 and v86fs checkouts.
func assetSetFromV86Dirs(v86Dir, v86fsDir, imageKey string) (*AssetSet, bool) {
	// Prefer the release emulator, with the checkout's debug build as fallback.
	wasm := filepath.Join(v86Dir, "build", "v86.wasm")
	if _, err := os.Stat(wasm); err != nil {
		wasm = filepath.Join(v86Dir, "build", "v86-debug.wasm")
	}
	// Resolve the conventional paths for a complete boot image.
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

// filesExist reports whether every path names a nonempty file.
func filesExist(paths ...string) bool {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
	}
	return true
}

// hydrateAssetsFromCdn materializes boot files and a rootfs object reference
// in the configured cache directory.
func hydrateAssetsFromCdn(ctx context.Context, opts AssetOptions) (*AssetSet, error) {
	// Prepare an output directory before opening CDN resources.
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create v86 wazero cache dir")
	}

	// Hold one published world across image selection and file reads.
	ws, release, err := mountCdnWorld(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer release()

	// Select a published V86Image before following its asset edges.
	imageKey, err := resolveV86ImageKey(ctx, ws, opts.ImageKey)
	if err != nil {
		return nil, err
	}
	// Download boot files while leaving the large rootfs addressable by object key.
	specs := []assetSpec{
		{Pred: string(s4wave_vm.PredV86ImageWasm), FileName: "v86.wasm", OutName: "v86.wasm"},
		{Pred: string(s4wave_vm.PredV86ImageBiosSeabios), FileName: "seabios.bin", OutName: "seabios.bin"},
		{Pred: string(s4wave_vm.PredV86ImageBiosVgabios), FileName: "vgabios.bin", OutName: "vgabios.bin"},
		{Pred: string(s4wave_vm.PredV86ImageKernel), FileName: "bzImage", OutName: "bzImage"},
	}
	for _, spec := range specs {
		if err := writeCdnAsset(ctx, opts.Le, ws, imageKey, spec, opts.CacheDir); err != nil {
			return nil, err
		}
	}

	// Retain the rootfs identity for lazy filesystem access.
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

	// Return only a cache entry with every required boot artifact.
	assets, ok := assetSetFromDir(opts.CacheDir, imageKey)
	if !ok {
		return nil, errors.Errorf("hydrated v86 assets incomplete in %s", opts.CacheDir)
	}
	assets.RootfsObjectKey = rootfsKey
	return assets, nil
}

// mountCdnWorld mounts the v86 image's shared object world for edge lookup.
func mountCdnWorld(ctx context.Context, opts AssetOptions) (world_state.WorldState, func(), error) {
	// Own the CDN transport until the mounted world is released.
	store, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: opts.CdnBaseURL,
		SpaceID:    opts.CdnSpaceID,
		PointerTTL: -1,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "build cdn block store")
	}

	// Bind the published head to the transport's block store.
	so, err := cdn_sharedobject.NewCdnSharedObject(cdn_sharedobject.CdnSharedObjectOptions{
		SpaceID:    opts.CdnSpaceID,
		BlockStore: store,
	})
	if err != nil {
		store.Close()
		return nil, nil, errors.Wrap(err, "build cdn shared object")
	}

	// Keep the read engine and its backing transport under one release callback.
	we, err := cdn_sharedobject.NewWorldEngine(ctx, opts.Le, nil, so)
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

// resolveV86ImageKey prefers an existing requested image, otherwise selecting
// the lexically greatest V86Image object key.
func resolveV86ImageKey(ctx context.Context, ws world_state.WorldState, preferred string) (string, error) {
	// Honor an existing explicit image before scanning the CDN catalogue.
	if preferred != "" {
		{
			objectState, found, err := ws.GetObject(ctx, preferred)
			world_state.ReleaseObjectState(objectState)
			if err != nil {
				return "", errors.Wrap(err, "probe preferred v86 image")
			} else if found {
				return preferred, nil
			}
		}
	}

	// Choose deterministically when the preferred object is unavailable.
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

// assetSpec names one guest image file and the graph predicate linking it.
type assetSpec struct {
	// Pred identifies the image-to-file graph edge.
	Pred string
	// FileName selects the file inside the UnixFS object.
	FileName string
	// OutName names the materialized file in the cache.
	OutName string
}

// writeCdnAsset writes an image edge's UnixFS file into the cache directory.
func writeCdnAsset(ctx context.Context, le *logrus.Entry, ws world_state.WorldState, imageKey string, spec assetSpec, dir string) error {
	// Resolve the file object from the selected image's graph edge.
	assetKey, err := lookupEdge(ctx, ws, imageKey, spec.Pred)
	if err != nil {
		return err
	}
	if assetKey == "" {
		return errors.Errorf("v86 image %q missing %s edge", imageKey, spec.Pred)
	}

	// Materialize the selected file under its conventional boot name.
	data, err := readUnixFSAsset(ctx, le, ws, assetKey, spec.FileName)
	if err != nil {
		return errors.Wrapf(err, "read %s asset object %q", spec.FileName, assetKey)
	}
	return os.WriteFile(filepath.Join(dir, spec.OutName), data, 0o644)
}

// lookupEdge returns the object key at the far end of a single quad edge.
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

// readUnixFSAsset extracts a single file from a unixfs object by name.
func readUnixFSAsset(ctx context.Context, le *logrus.Entry, ws world_state.WorldState, objectKey, fileName string) ([]byte, error) {
	// Hold the object's filesystem while reading its selected asset.
	fsh, err := openFSHandleForObject(ctx, le, ws, objectKey)
	if err != nil {
		return nil, err
	}
	defer fsh.Release()

	// Prefer the conventional filename over a single-file object fallback.
	bfs := unixfs_billy.NewBillyFS(ctx, fsh, "", time.Time{})
	if data, err := billy_util.ReadFile(bfs, fileName); err == nil {
		return data, nil
	}

	// Accept an alternate name only when the object contains exactly one file.
	entries, err := bfs.ReadDir(".")
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 || entries[0].IsDir() {
		return nil, errors.Errorf("asset object %q does not contain %q or a single file", objectKey, fileName)
	}
	return billy_util.ReadFile(bfs, entries[0].Name())
}

// openFSHandleForObject opens the unixfs handle rooted at an object key.
func openFSHandleForObject(ctx context.Context, le *logrus.Entry, ws world_state.WorldState, objectKey string) (*unixfs.FSHandle, error) {
	// Resolve the stored filesystem type before constructing its cursor.
	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, errors.Wrap(err, "lookup fs type")
	}

	// Transfer cursor ownership into the filesystem handle.
	fsCursor := unixfs_world.NewFSCursor(le, ws, objectKey, fsType, nil, false)
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, errors.Wrap(err, "create fs handle")
	}
	return fsh, nil
}
