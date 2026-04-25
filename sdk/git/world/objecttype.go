package s4wave_git_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-git/v6"
	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	resource_git "github.com/s4wave/spacewave/sdk/git/resource"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// GitRepoTypeID is the object type ID for git/repo objects.
const GitRepoTypeID = git_world.GitRepoTypeID

// GitRepoType is the ObjectType for git/repo objects.
var GitRepoType = objecttype.NewObjectType(GitRepoTypeID, GitRepoFactory)

// GitRepoFactory creates a GitRepoResource from a world object.
func GitRepoFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	var repoInfo resource_git.RepoSnapshot
	_, _, err := git_world.AccessWorldObjectRepo(
		ctx, ws, objectKey, false,
		nil, nil, nil,
		func(repo *git.Repository) error {
			return resource_git.SnapshotRepo(repo, &repoInfo)
		},
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "access git repo")
	}

	resource := resource_git.NewGitRepoResource(ws, objectKey, &repoInfo)
	return resource.GetMux(), func() {}, nil
}
