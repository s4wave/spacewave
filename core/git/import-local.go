package s4wave_git

import (
	"cmp"
	"context"
	stderrors "errors"
	"io"
	"slices"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/revlist"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

// ImportLocalRepoToRef imports the committed graph of a local Git repository
// into a World repo ref without copying configuration or contacting remotes.
func ImportLocalRepoToRef(
	ctx context.Context,
	ws worldStorageAccessor,
	localPath string,
) (repoRef *bucket.ObjectRef, headHash, branchName string, err error) {
	repo, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "open local repository")
	}
	closer, ok := repo.Storer.(io.Closer)
	if !ok {
		return nil, "", "", errors.New("local repository storage is not closeable")
	}
	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			err = stderrors.Join(err, errors.Wrap(closeErr, "close local repository"))
		}
	}()
	return importOpenedLocalRepo(ctx, ws, repo, nil)
}

func importOpenedLocalRepo(
	ctx context.Context,
	ws worldStorageAccessor,
	repo *git.Repository,
	afterObject func(),
) (repoRef *bucket.ObjectRef, headHash, branchName string, err error) {
	snapshot, err := captureLocalRepo(repo)
	if err != nil {
		return nil, "", "", err
	}
	headHash = snapshot.resolvedHead.Hash().String()
	if snapshot.rawHead.Type() == plumbing.SymbolicReference && snapshot.resolvedHead.Name().IsBranch() {
		branchName = snapshot.resolvedHead.Name().Short()
	}

	wanted, err := revlist.Objects(repo.Storer, snapshot.wants, nil)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "walk local repository objects")
	}
	wantedSet := makeHashSet(wanted)

	repoRef, err = world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) (cbErr error) {
		root := git_block.NewRepo()
		bcs.SetBlock(root, true)
		store, err := git_block.NewStore(ctx, nil, bcs, &memory.IndexStorage{}, nil)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := store.Close(); closeErr != nil {
				cbErr = stderrors.Join(cbErr, errors.Wrap(closeErr, "close imported repository"))
			}
		}()

		for _, hash := range wanted {
			if err := ctx.Err(); err != nil {
				return err
			}
			obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, hash)
			if err != nil {
				return errors.Wrapf(err, "read local Git object %s", hash)
			}
			storedHash, err := store.SetEncodedObject(obj)
			if err != nil {
				return errors.Wrapf(err, "store local Git object %s", hash)
			}
			if storedHash != hash {
				return errors.Errorf("stored local Git object %s as %s", hash, storedHash)
			}
			if afterObject != nil {
				afterObject()
			}
		}
		for _, ref := range snapshot.refs {
			if err := store.SetReference(ref); err != nil {
				return errors.Wrapf(err, "store local Git reference %s", ref.Name())
			}
		}
		if err := store.SetShallow(snapshot.shallow); err != nil {
			return errors.Wrap(err, "store local shallow boundary")
		}
		if err := verifyImportedRepo(repo, store, snapshot, wantedSet); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return store.Commit()
	})
	if err != nil {
		return nil, "", "", errors.Wrap(err, "import local repository")
	}
	return repoRef, headHash, branchName, nil
}

type localRepoSnapshot struct {
	resolvedHead *plumbing.Reference
	rawHead      *plumbing.Reference
	refs         []*plumbing.Reference
	wants        []plumbing.Hash
	shallow      []plumbing.Hash
}

func captureLocalRepo(repo *git.Repository) (*localRepoSnapshot, error) {
	resolvedHead, err := repo.Head()
	if err != nil {
		return nil, errors.Wrap(err, "resolve local HEAD")
	}
	rawHead, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return nil, errors.Wrap(err, "read local HEAD")
	}

	iter, err := repo.References()
	if err != nil {
		return nil, errors.Wrap(err, "list local references")
	}
	defer iter.Close()
	refsByName := make(map[plumbing.ReferenceName]*plumbing.Reference)
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		refsByName[ref.Name()] = ref
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "list local references")
	}
	currentRawHead, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return nil, errors.Wrap(err, "reread local HEAD")
	}
	if !sameReference(rawHead, currentRawHead) {
		return nil, errors.New("local HEAD changed during import")
	}
	refsByName[plumbing.HEAD] = rawHead

	refs := make([]*plumbing.Reference, 0, len(refsByName))
	wantSet := map[plumbing.Hash]struct{}{resolvedHead.Hash(): {}}
	for _, ref := range refsByName {
		refs = append(refs, ref)
		if ref.Type() == plumbing.HashReference {
			wantSet[ref.Hash()] = struct{}{}
		}
	}
	slices.SortFunc(refs, func(a, b *plumbing.Reference) int {
		return cmp.Compare(a.Name().String(), b.Name().String())
	})
	wants := make([]plumbing.Hash, 0, len(wantSet))
	for hash := range wantSet {
		wants = append(wants, hash)
	}
	slices.SortFunc(wants, func(a, b plumbing.Hash) int {
		return cmp.Compare(a.String(), b.String())
	})
	shallow, err := repo.Storer.Shallow()
	if err != nil {
		return nil, errors.Wrap(err, "read local shallow boundary")
	}
	return &localRepoSnapshot{
		resolvedHead: resolvedHead,
		rawHead:      rawHead,
		refs:         refs,
		wants:        wants,
		shallow:      shallow,
	}, nil
}

func verifyImportedRepo(
	source *git.Repository,
	destination storer.Storer,
	snapshot *localRepoSnapshot,
	wantedSet map[plumbing.Hash]struct{},
) error {
	got, err := revlist.Objects(destination, snapshot.wants, nil)
	if err != nil {
		return errors.Wrap(err, "walk imported repository objects")
	}
	if !sameHashSet(wantedSet, got) {
		return errors.New("imported repository object closure differs from source")
	}
	destinationHead, err := storer.ResolveReference(destination, plumbing.HEAD)
	if err != nil {
		return errors.Wrap(err, "resolve imported HEAD")
	}
	if destinationHead.Hash() != snapshot.resolvedHead.Hash() {
		return errors.New("imported HEAD differs from captured source HEAD")
	}
	destinationRawHead, err := destination.Reference(plumbing.HEAD)
	if err != nil {
		return errors.Wrap(err, "read imported HEAD")
	}
	if !sameReference(destinationRawHead, snapshot.rawHead) {
		return errors.New("imported raw HEAD differs from captured source HEAD")
	}
	currentHead, err := source.Head()
	if err != nil {
		return errors.Wrap(err, "reresolve local HEAD")
	}
	currentRawHead, err := source.Reference(plumbing.HEAD, false)
	if err != nil {
		return errors.Wrap(err, "reread local HEAD")
	}
	if currentHead.Hash() != snapshot.resolvedHead.Hash() || !sameReference(currentRawHead, snapshot.rawHead) {
		return errors.New("local HEAD changed during import")
	}
	return nil
}

func sameReference(a, b *plumbing.Reference) bool {
	return a != nil && b != nil && a.Name() == b.Name() && a.Type() == b.Type() &&
		a.Hash() == b.Hash() && a.Target() == b.Target()
}

func makeHashSet(hashes []plumbing.Hash) map[plumbing.Hash]struct{} {
	set := make(map[plumbing.Hash]struct{}, len(hashes))
	for _, hash := range hashes {
		set[hash] = struct{}{}
	}
	return set
}

func sameHashSet(want map[plumbing.Hash]struct{}, got []plumbing.Hash) bool {
	if len(want) != len(got) {
		return false
	}
	for _, hash := range got {
		if _, ok := want[hash]; !ok {
			return false
		}
	}
	return true
}
