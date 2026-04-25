package space_unixfs

import (
	"context"
	"sort"
	"strings"

	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	git_repofs "github.com/s4wave/spacewave/sdk/git/repofs"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	"github.com/sirupsen/logrus"
)

func listProjectedObjects(ctx context.Context, ws world.WorldState) ([]string, error) {
	iter := ws.IterateObjects(ctx, "", false)
	defer iter.Close()

	var objectKeys []string
	for iter.Next() {
		key := iter.Key()
		if strings.HasPrefix(key, world_types.TypesPrefix) {
			continue
		}
		objectKeys = append(objectKeys, key)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	metadata, err := world_types.GetObjectMetadataBatch(ctx, ws, objectKeys)
	if err != nil {
		return nil, err
	}

	var projected []string
	for _, md := range metadata {
		if !supportsProjectedType(md.TypeID) {
			continue
		}
		projected = append(projected, md.ObjectKey)
	}
	sort.Strings(projected)
	return projected, nil
}

func supportsProjectedType(typeID string) bool {
	switch typeID {
	case unixfs_world.FSNodeTypeID, unixfs_world.FSObjectTypeID, unixfs_world.FSHostVolumeTypeID:
		return true
	case s4wave_git_world.GitRepoTypeID, s4wave_git_world.GitWorktreeTypeID:
		return true
	default:
		return false
	}
}

func openObjectCursor(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objectKey string,
) (unixfs.FSCursor, error) {
	typeID, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	if !supportsProjectedType(typeID) {
		return nil, unixfs_errors.ErrNotExist
	}

	switch typeID {
	case s4wave_git_world.GitRepoTypeID:
		return openGitRepoCursor(ctx, ws, objectKey)
	case s4wave_git_world.GitWorktreeTypeID:
		return openGitWorktreeCursor(ctx, le, ws, objectKey)
	}

	fsType, _, err := unixfs_world.LookupFsType(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	return unixfs_world.NewFSCursor(le, ws, objectKey, fsType, nil, false), nil
}

func buildProjectedDirents(children map[string]*projectedChild) []*projectedDirent {
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.Strings(names)

	dirents := make([]*projectedDirent, 0, len(names))
	for _, name := range names {
		dirents = append(dirents, newProjectedDirent(name, unixfs.NewFSCursorNodeType_Dir()))
	}
	return dirents
}

func splitObjectKey(objectKey string) []string {
	if objectKey == "" {
		return nil
	}
	return strings.Split(objectKey, "/")
}

func hasPathPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func findExactObjectKey(path []string, objectKeys []string) (string, bool) {
	want := strings.Join(path, "/")
	for _, objectKey := range objectKeys {
		if objectKey == want {
			return objectKey, true
		}
	}
	return "", false
}

func openGitWorktreeCursor(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objectKey string,
) (unixfs.FSCursor, error) {
	workdirRef, err := git_world.WorktreeLookupWorkdirRef(ctx, ws, objectKey)
	if err != nil {
		return nil, err
	}
	return unixfs_world.FollowUnixfsRef(ctx, le, ws, workdirRef, "", false)
}

func openGitRepoCursor(ctx context.Context, ws world.WorldState, objectKey string) (unixfs.FSCursor, error) {
	return git_repofs.OpenRepoFSCursor(ctx, ws, objectKey, false)
}
