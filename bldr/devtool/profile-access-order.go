//go:build !js

package devtool

import (
	"context"
	stderrors "errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	packfile_order "github.com/s4wave/spacewave/core/provider/spacewave/packfile/order"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
)

var chunkImportPattern = regexp.MustCompile(`(?:\./)?chunks/[^"'` + "`" + `)]+\.mjs`)

// BuildProfileAccessOrderCommand builds the profile-access-order command.
func (a *DevtoolArgs) BuildProfileAccessOrderCommand() *cli.Command {
	return &cli.Command{
		Name:  "profile-access-order",
		Usage: "record manifest startup file access order",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "manifest",
				Usage:       "manifest ID to profile",
				Value:       a.AccessOrderManifestID,
				Destination: &a.AccessOrderManifestID,
			},
			&cli.StringFlag{
				Name:        "platform",
				Usage:       "manifest platform ID to profile",
				Value:       a.AccessOrderPlatformID,
				Destination: &a.AccessOrderPlatformID,
			},
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "access-order .bin output path",
				Value:       a.AccessOrderOutputPath,
				Destination: &a.AccessOrderOutputPath,
			},
		},
		Action: func(c *cli.Context) error {
			return a.ExecuteProfileAccessOrder(c.Context)
		},
	}
}

// ExecuteProfileAccessOrder records startup access order for one built manifest.
func (a *DevtoolArgs) ExecuteProfileAccessOrder(ctx context.Context) (err error) {
	le := a.Logger
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}

	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, false)
	if err != nil {
		return err
	}
	defer b.Release()

	manifestID := a.AccessOrderManifestID
	if manifestID == "" {
		manifestID = "spacewave-browser"
	}
	platformID := a.AccessOrderPlatformID
	if platformID == "" {
		platformID = "web/js/wasm"
	}

	manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		b.GetWorldState(),
		manifestID,
		[]string{platformID},
		b.GetPluginHostObjectKey(),
	)
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		if len(manifestErrs) != 0 {
			return errors.Wrap(manifestErrs[0], "collect manifest")
		}
		return errors.Errorf("manifest not found: %s %s", manifestID, platformID)
	}

	outPath := a.AccessOrderOutputPath
	if outPath == "" {
		outPath = filepath.Join(stateDir, "access-order", accessOrderFilename(manifestID, platformID))
	}
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(repoRoot, outPath)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return errors.Wrap(err, "create access-order output directory")
	}

	collected := manifests[0]
	var record *packfile_order.AccessOrderRecord
	err = bldr_manifest_world.AccessManifest(
		ctx,
		le,
		b.GetWorldState().AccessWorldState,
		collected.ManifestRef,
		func(
			ctx context.Context,
			bls *bucket_lookup.Cursor,
			bcs *block.Cursor,
			manifest *bldr_manifest.Manifest,
			distFS *unixfs.FSHandle,
			assetsFS *unixfs.FSHandle,
		) error {
			record, err = buildStartupAccessOrderRecord(ctx, manifest, collected.ManifestRef.GetRootRef(), distFS, assetsFS)
			return err
		},
	)
	if err != nil {
		return err
	}
	if record == nil {
		return errors.New("profile did not produce an access-order record")
	}
	return packfile_order.WriteAccessOrderRecordFile(outPath, record)
}

func buildStartupAccessOrderRecord(
	ctx context.Context,
	manifest *bldr_manifest.Manifest,
	manifestRoot *block.BlockRef,
	distFS *unixfs.FSHandle,
	assetsFS *unixfs.FSHandle,
) (*packfile_order.AccessOrderRecord, error) {
	meta := manifest.GetMeta()
	r := newStartupAccessRecorder()
	entrypoint := manifest.GetEntrypoint()
	if entrypoint != "" {
		r.add(packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST, entrypoint, packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ENTRYPOINT, entrypoint)
	}

	if err := recordDynamicImports(ctx, r, distFS, entrypoint); err != nil {
		return nil, err
	}
	if err := recordAssetRoot(ctx, r, assetsFS); err != nil {
		return nil, err
	}

	return &packfile_order.AccessOrderRecord{
		ManifestId:      meta.GetManifestId(),
		PlatformId:      meta.GetPlatformId(),
		BuildType:       meta.GetBuildType(),
		ManifestRootRef: manifestRoot.Clone(),
		ManifestRev:     meta.GetRev(),
		Entries:         r.entries,
	}, nil
}

type startupAccessRecorder struct {
	entries []*packfile_order.AccessOrderEntry
	byKey   map[string]*packfile_order.AccessOrderEntry
}

func newStartupAccessRecorder() *startupAccessRecorder {
	return &startupAccessRecorder{byKey: make(map[string]*packfile_order.AccessOrderEntry)}
}

func (r *startupAccessRecorder) add(filesystem packfile_order.AccessOrderFilesystem, fpath string, reason packfile_order.AccessOrderReason, detail string) {
	fpath = path.Clean(strings.TrimPrefix(fpath, "./"))
	if fpath == "." || fpath == "" {
		return
	}
	key := filesystem.String() + "\x00" + fpath + "\x00" + reason.String()
	if entry := r.byKey[key]; entry != nil {
		entry.AccessCount++
		return
	}
	entry := &packfile_order.AccessOrderEntry{
		Ordinal:      uint64(len(r.entries)),
		Filesystem:   filesystem,
		Path:         fpath,
		Reason:       reason,
		ReasonDetail: detail,
		AccessCount:  1,
	}
	r.entries = append(r.entries, entry)
	r.byKey[key] = entry
}

func recordDynamicImports(ctx context.Context, r *startupAccessRecorder, distFS *unixfs.FSHandle, entrypoint string) error {
	if entrypoint != "" {
		entrypointHandle, _, err := distFS.LookupPath(ctx, entrypoint)
		if err != nil {
			if !stderrors.Is(err, fs.ErrNotExist) {
				return errors.Wrap(err, "lookup entrypoint")
			}
		} else {
			defer entrypointHandle.Release()
			dat, err := unixfs.ReadFile(ctx, entrypointHandle)
			if err != nil {
				return errors.Wrap(err, "read entrypoint")
			}
			for _, match := range chunkImportPattern.FindAllString(string(dat), -1) {
				r.add(packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST, match, packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, match)
			}
		}
	}

	chunkPaths, err := listChunkModulePaths(ctx, distFS)
	if err != nil {
		return err
	}
	for _, chunkPath := range chunkPaths {
		r.add(packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST, chunkPath, packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, chunkPath)
	}
	return nil
}

func recordAssetRoot(ctx context.Context, r *startupAccessRecorder, assetsFS *unixfs.FSHandle) error {
	if assetsFS == nil {
		return nil
	}
	entries, err := readDirNames(ctx, assetsFS, "")
	if err != nil {
		if stderrors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, name := range entries {
		r.add(packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_ASSETS, name, packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ASSET, "asset root")
	}
	return nil
}

func listChunkModulePaths(ctx context.Context, distFS *unixfs.FSHandle) ([]string, error) {
	entries, err := readDirNames(ctx, distFS, "chunks")
	if err != nil {
		if stderrors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, name := range entries {
		if strings.HasSuffix(name, ".mjs") {
			paths = append(paths, path.Join("chunks", name))
		}
	}
	slices.Sort(paths)
	return paths, nil
}

func readDirNames(ctx context.Context, root *unixfs.FSHandle, dirPath string) ([]string, error) {
	dir := root
	if dirPath != "" {
		var err error
		dir, _, err = root.LookupPath(ctx, dirPath)
		if err != nil {
			return nil, err
		}
		defer dir.Release()
	}

	_, ops, err := dir.GetOps(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		if ent.GetName() != "" {
			names = append(names, ent.GetName())
		}
		return nil
	}); err != nil {
		return nil, err
	}
	slices.Sort(names)
	return names, nil
}

func accessOrderFilename(manifestID, platformID string) string {
	repl := strings.NewReplacer("/", "-", " ", "-", "\t", "-")
	return repl.Replace(manifestID+"-"+platformID) + ".bin"
}
