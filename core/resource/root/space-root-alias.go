//go:build !js

package resource_root

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

const spaceRootAliasObjectStoreID = "space-root-aliases"

var spaceRootAliasKeyPrefix = []byte("alias/")

// ListSpaceRootAliases lists configured local state-root records.
func (s *CoreRootServer) ListSpaceRootAliases(
	ctx context.Context,
	_ *s4wave_root.ListSpaceRootAliasesRequest,
) (*s4wave_root.ListSpaceRootAliasesResponse, error) {
	records, err := s.snapshotSpaceRootAliases(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_root.ListSpaceRootAliasesResponse{Records: records}, nil
}

// WatchSpaceRootAliases streams configured local state-root records.
func (s *CoreRootServer) WatchSpaceRootAliases(
	_ *s4wave_root.WatchSpaceRootAliasesRequest,
	strm s4wave_root.SRPCRootResourceService_WatchSpaceRootAliasesStream,
) error {
	ctx := strm.Context()
	var prev []*s4wave_root.SpaceRootAliasRecord
	first := true
	for {
		_, waitCh := s.snapshotSpaceRootAliasWaitCh()
		records, err := s.snapshotSpaceRootAliases(ctx)
		if err != nil {
			return err
		}
		if first || !spaceRootAliasRecordsEqual(prev, records) {
			first = false
			prev = cloneSpaceRootAliasRecords(records)
			if err := strm.Send(&s4wave_root.WatchSpaceRootAliasesResponse{
				Records: records,
			}); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// UpsertSpaceRootAlias validates and persists a configured local state root.
func (s *CoreRootServer) UpsertSpaceRootAlias(
	ctx context.Context,
	req *s4wave_root.UpsertSpaceRootAliasRequest,
) (*s4wave_root.UpsertSpaceRootAliasResponse, error) {
	// Open the configured root alias store for the update.
	store, release, err := s.openSpaceRootAliasObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Begin the transaction that writes the root alias record.
	otx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	// Require the root alias record supplied by the caller.
	record := req.GetRecord()
	if record == nil {
		return nil, errors.New("space root alias record is required")
	}

	// Preserve the root alias creation time while updating its timestamp.
	now := time.Now().UnixMilli()
	createdAt := record.GetCreatedAtUnixMs()
	existing, found, err := readSpaceRootAliasRecord(ctx, otx, record.GetAliasId())
	if err != nil {
		return nil, err
	}
	if found && createdAt == 0 {
		createdAt = existing.GetCreatedAtUnixMs()
	}
	if createdAt == 0 {
		createdAt = now
	}

	// Validate and persist the configured root alias record.
	validated, err := validateSpaceRootAliasRecord(record, createdAt, now)
	if err != nil {
		return nil, err
	}
	data, err := validated.MarshalVT()
	if err != nil {
		return nil, err
	}
	if err := otx.Set(ctx, spaceRootAliasKey(validated.GetAliasId()), data); err != nil {
		return nil, err
	}
	if err := otx.Commit(ctx); err != nil {
		return nil, err
	}

	// Notify root alias watchers of the committed record.
	s.broadcastSpaceRootAliasChange()
	return &s4wave_root.UpsertSpaceRootAliasResponse{Record: validated}, nil
}

// RemoveSpaceRootAlias removes a configured local state root.
func (s *CoreRootServer) RemoveSpaceRootAlias(
	ctx context.Context,
	req *s4wave_root.RemoveSpaceRootAliasRequest,
) (*s4wave_root.RemoveSpaceRootAliasResponse, error) {
	// Require the root alias identifier before removing its record.
	aliasID := strings.TrimSpace(req.GetAliasId())
	if aliasID == "" {
		return nil, errors.New("space root alias id is required")
	}

	// Open the configured root alias store for removal.
	store, release, err := s.openSpaceRootAliasObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Begin the transaction that removes the root alias record.
	otx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	// Find and delete the persisted root alias record.
	key := spaceRootAliasKey(aliasID)
	_, found, err := otx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return &s4wave_root.RemoveSpaceRootAliasResponse{NotFound: true}, nil
	}
	if err := otx.Delete(ctx, key); err != nil {
		return nil, err
	}
	if err := otx.Commit(ctx); err != nil {
		return nil, err
	}

	// Notify root alias watchers of the committed removal.
	s.broadcastSpaceRootAliasChange()
	return &s4wave_root.RemoveSpaceRootAliasResponse{}, nil
}

func (s *CoreRootServer) openSpaceRootAliasObjectStore(
	ctx context.Context,
) (object.ObjectStore, func(), error) {
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		s.b,
		false,
		spaceRootAliasObjectStoreID,
		bldr_plugin.PluginVolumeID,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return objStoreHandle.GetObjectStore(), diRef.Release, nil
}

func (s *CoreRootServer) snapshotSpaceRootAliases(
	ctx context.Context,
) ([]*s4wave_root.SpaceRootAliasRecord, error) {
	// Open the configured root alias store for the snapshot.
	store, release, err := s.openSpaceRootAliasObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Begin a read transaction on the configured root alias store.
	otx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	// Decode the saved root alias records and refresh their status.
	records := make([]*s4wave_root.SpaceRootAliasRecord, 0)
	err = otx.ScanPrefix(ctx, spaceRootAliasKeyPrefix, func(_ []byte, value []byte) error {
		record := &s4wave_root.SpaceRootAliasRecord{}
		if err := record.UnmarshalVT(value); err != nil {
			s.le.WithError(err).Warn("ignoring invalid space root alias record")
			return nil
		}
		records = append(records, refreshSpaceRootAliasStatus(record))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Order root alias records by identifier for stable snapshots.
	slices.SortFunc(records, func(a, b *s4wave_root.SpaceRootAliasRecord) int {
		return strings.Compare(a.GetAliasId(), b.GetAliasId())
	})
	return records, nil
}

func validateSpaceRootAliasRecord(
	record *s4wave_root.SpaceRootAliasRecord,
	createdAt int64,
	now int64,
) (*s4wave_root.SpaceRootAliasRecord, error) {
	// Validate the root alias identifier, kind, and open mode.
	aliasID := strings.TrimSpace(record.GetAliasId())
	if aliasID == "" {
		return nil, errors.New("space root alias id is required")
	}
	if strings.Contains(aliasID, "/") {
		return nil, errors.New("space root alias id cannot contain /")
	}
	if record.GetKind() != s4wave_root.SpaceRootKind_SpaceRootKind_NATIVE_DIRECTORY &&
		record.GetKind() != s4wave_root.SpaceRootKind_SpaceRootKind_S4WAVE_FILE {
		return nil, errors.New("space root alias kind must be native directory or .s4wave file")
	}
	if record.GetOpenMode() != s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING {
		return nil, errors.New("space root alias open mode must be open existing")
	}

	// Require an absolute native path to an existing root.
	native := record.GetNative()
	if native == nil {
		return nil, errors.New("native path metadata is required")
	}
	path := filepath.Clean(strings.TrimSpace(native.GetPath()))
	if path == "." || path == "" {
		return nil, errors.New("native path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("native path must be absolute")
	}
	if err := validateSpaceRootAliasPath(record.GetKind(), path); err != nil {
		return nil, err
	}

	// Choose the root alias display name from the request or path.
	displayName := strings.TrimSpace(record.GetDisplayName())
	if displayName == "" {
		displayName = filepath.Base(path)
	}

	return &s4wave_root.SpaceRootAliasRecord{
		AliasId:         aliasID,
		DisplayName:     displayName,
		Kind:            record.GetKind(),
		OpenMode:        s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING,
		Native:          &s4wave_root.NativeSpaceRootMetadata{Path: path},
		Status:          s4wave_root.SpaceRootStatus_SpaceRootStatus_READY,
		StatusMessage:   "",
		Browser:         &s4wave_root.BrowserSpaceRootMetadata{},
		CreatedAtUnixMs: createdAt,
		UpdatedAtUnixMs: now,
	}, nil
}

// validateSpaceRootAliasPath requires an existing catalog beside a provider volume.
func validateSpaceRootAliasPath(kind s4wave_root.SpaceRootKind, path string) error {
	if kind == s4wave_root.SpaceRootKind_SpaceRootKind_S4WAVE_FILE {
		if filepath.Ext(path) != ".s4wave" {
			return errors.New("selected file must have the .s4wave extension")
		}
		info, err := os.Stat(path)
		if err != nil {
			return errors.Wrap(err, "stat selected .s4wave file")
		}
		if !info.Mode().IsRegular() {
			return errors.New("selected .s4wave path is not a regular file")
		}
		if filepath.Base(path) != entrypoint_state.Filename {
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), entrypoint_state.Filename)); err != nil {
				return errors.Wrap(err, "find sibling Session catalog state.s4wave")
			}
		}
		return validateExistingSpaceRootPath(filepath.Dir(path))
	}
	if filepath.Ext(path) == ".s4wave" {
		return errors.New("select the state-root directory or use the .s4wave file kind")
	}
	return validateExistingSpaceRootPath(path)
}

func validateExistingSpaceRootPath(path string) error {
	// Require the selected root path to be an existing directory.
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Errorf("selected path does not exist: %s", path)
		}
		return errors.Wrap(err, "stat selected space root")
	}
	if !info.IsDir() {
		return errors.Errorf("selected path is not a directory: %s", path)
	}

	// Recognize the directory by its plugin, logs, or volume files.
	if _, err := os.Stat(filepath.Join(path, "plugin")); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(path, "logs")); err == nil {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(path, "*.s4wave"))
	if err != nil {
		return errors.Wrap(err, "check state root volumes")
	}
	if len(matches) != 0 {
		return nil
	}
	return errors.New("selected directory does not look like a Spacewave state root")
}

func refreshSpaceRootAliasStatus(
	record *s4wave_root.SpaceRootAliasRecord,
) *s4wave_root.SpaceRootAliasRecord {
	// Mark root alias modes unsupported by this runtime.
	out := record.CloneVT()
	if (out.GetKind() != s4wave_root.SpaceRootKind_SpaceRootKind_NATIVE_DIRECTORY &&
		out.GetKind() != s4wave_root.SpaceRootKind_SpaceRootKind_S4WAVE_FILE) ||
		out.GetOpenMode() != s4wave_root.SpaceRootOpenMode_SpaceRootOpenMode_OPEN_EXISTING {
		out.Status = s4wave_root.SpaceRootStatus_SpaceRootStatus_UNSUPPORTED
		out.StatusMessage = "configured root mode is not supported by this app"
		return out
	}

	// Check the root alias path and report missing or invalid roots.
	path := out.GetNative().GetPath()
	if err := validateSpaceRootAliasPath(out.GetKind(), path); err != nil {
		out.Status = s4wave_root.SpaceRootStatus_SpaceRootStatus_INVALID
		if os.IsNotExist(errors.Cause(err)) {
			out.Status = s4wave_root.SpaceRootStatus_SpaceRootStatus_MISSING
		}
		out.StatusMessage = err.Error()
		return out
	}

	// Mark the validated root alias ready for use.
	out.Status = s4wave_root.SpaceRootStatus_SpaceRootStatus_READY
	out.StatusMessage = ""
	return out
}

func readSpaceRootAliasRecord(
	ctx context.Context,
	otx interface {
		Get(context.Context, []byte) ([]byte, bool, error)
	},
	aliasID string,
) (*s4wave_root.SpaceRootAliasRecord, bool, error) {
	// Read and decode the saved root alias identified by the caller.
	aliasID = strings.TrimSpace(aliasID)
	if aliasID == "" {
		return nil, false, nil
	}
	data, found, err := otx.Get(ctx, spaceRootAliasKey(aliasID))
	if err != nil || !found {
		return nil, found, err
	}
	record := &s4wave_root.SpaceRootAliasRecord{}
	if err := record.UnmarshalVT(data); err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func spaceRootAliasKey(aliasID string) []byte {
	return []byte("alias/" + aliasID)
}

func (s *CoreRootServer) snapshotSpaceRootAliasWaitCh() (func(), <-chan struct{}) {
	var broadcastFn func()
	var waitCh <-chan struct{}
	s.spaceRootAliasBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcastFn = broadcast
		waitCh = getWaitCh()
	})
	return broadcastFn, waitCh
}

func (s *CoreRootServer) broadcastSpaceRootAliasChange() {
	s.spaceRootAliasBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

func cloneSpaceRootAliasRecords(
	records []*s4wave_root.SpaceRootAliasRecord,
) []*s4wave_root.SpaceRootAliasRecord {
	out := make([]*s4wave_root.SpaceRootAliasRecord, 0, len(records))
	for _, record := range records {
		out = append(out, record.CloneVT())
	}
	return out
}

func spaceRootAliasRecordsEqual(
	a []*s4wave_root.SpaceRootAliasRecord,
	b []*s4wave_root.SpaceRootAliasRecord,
) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].EqualVT(b[i]) {
			return false
		}
	}
	return true
}
