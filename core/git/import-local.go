package s4wave_git

import (
	"cmp"
	"context"
	stderrors "errors"
	"io"
	"slices"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
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
	// Open the source and bind its file handles to this import.
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

	// Import only committed objects and references into the World.
	return importOpenedLocalRepo(ctx, ws, repo, nil)
}

// importOpenedLocalRepo captures, copies, and verifies a committed repository graph.
func importOpenedLocalRepo(
	ctx context.Context,
	ws worldStorageAccessor,
	repo *git.Repository,
	afterObject func(),
) (repoRef *bucket.ObjectRef, headHash, branchName string, err error) {
	// Freeze the reference roots and HEAD identity before reading objects.
	snapshot, err := captureLocalRepo(repo)
	if err != nil {
		return nil, "", "", err
	}
	headHash = snapshot.resolvedHead.Hash().String()
	if snapshot.rawHead.Type() == plumbing.SymbolicReference && snapshot.resolvedHead.Name().IsBranch() {
		branchName = snapshot.resolvedHead.Name().Short()
	}

	// Resolve the complete reachable closure, including non-HEAD references.
	wanted, err := revlist.Objects(repo.Storer, snapshot.wants, nil)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "walk local repository objects")
	}
	wantedSet := makeHashSet(wanted)

	// Publish the repository only after its pack and references verify.
	repoRef, err = world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) (cbErr error) {
		// Construct a repository inside the World object transaction.
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

		// Keep Git's existing deltas instead of expanding the graph into one
		// block tree per object. A one-object window avoids an expensive search
		// for new deltas while retaining the source's existing compression.
		writer, err := store.PackfileWriter()
		if err != nil {
			return err
		}
		source := &importObjectStore{Storer: repo.Storer, ctx: ctx, afterObject: afterObject}
		if _, err := packfile.NewEncoder(writer, source, false).Encode(wanted, 1); err != nil {
			return errors.Wrap(err, "encode local repository pack")
		}
		if err := writer.Close(); err != nil {
			return errors.Wrap(err, "store local repository pack")
		}

		// Restore the captured reference names and shallow boundary.
		for _, ref := range snapshot.refs {
			if err := store.SetReference(ref); err != nil {
				return errors.Wrapf(err, "store local Git reference %s", ref.Name())
			}
		}
		if err := store.SetShallow(snapshot.shallow); err != nil {
			return errors.Wrap(err, "store local shallow boundary")
		}

		// Reject incomplete graphs, source HEAD drift, and canceled imports.
		if err := verifyImportedRepo(repo, store, snapshot, wantedSet); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return store.Commit()
	})

	// Return the published reference together with the captured checkout identity.
	if err != nil {
		return nil, "", "", errors.Wrap(err, "import local repository")
	}
	return repoRef, headHash, branchName, nil
}

// localRepoSnapshot holds the captured references and shallow graph roots.
type localRepoSnapshot struct {
	// resolvedHead identifies the selected commit.
	resolvedHead *plumbing.Reference
	// rawHead preserves detached or symbolic HEAD form.
	rawHead *plumbing.Reference
	// refs contains every captured reference, including HEAD.
	refs []*plumbing.Reference
	// wants contains the distinct commit and tag roots.
	wants []plumbing.Hash
	// shallow marks the permitted history boundaries.
	shallow []plumbing.Hash
}

// captureLocalRepo snapshots HEAD, references, and the shallow boundary.
func captureLocalRepo(repo *git.Repository) (*localRepoSnapshot, error) {
	// Capture both HEAD forms before enumerating other references.
	resolvedHead, err := repo.Head()
	if err != nil {
		return nil, errors.Wrap(err, "resolve local HEAD")
	}
	rawHead, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return nil, errors.Wrap(err, "read local HEAD")
	}

	// Collect references by name so HEAD has exactly one captured value.
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

	// Detect HEAD changes while the reference iterator was running.
	currentRawHead, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return nil, errors.Wrap(err, "reread local HEAD")
	}
	if !sameReference(rawHead, currentRawHead) {
		return nil, errors.New("local HEAD changed during import")
	}
	refsByName[plumbing.HEAD] = rawHead

	// Derive deterministic reference and object-root ordering.
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

	// Retain shallow boundaries alongside the captured roots.
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

// verifyImportedRepo checks graph closure and stable source and destination HEADs.
func verifyImportedRepo(
	source *git.Repository,
	destination storer.Storer,
	snapshot *localRepoSnapshot,
	wantedSet map[plumbing.Hash]struct{},
) error {
	// Require the destination to resolve the complete captured object closure.
	got, err := revlist.Objects(destination, snapshot.wants, nil)
	if err != nil {
		return errors.Wrap(err, "walk imported repository objects")
	}
	if !sameHashSet(wantedSet, got) {
		return errors.New("imported repository object closure differs from source")
	}
	// Verify the destination preserves resolved and symbolic HEAD identity.
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

	// Fence publication against a source checkout change during the import.
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

// sameReference compares complete reference identity without resolving symbols.
func sameReference(a, b *plumbing.Reference) bool {
	return a != nil && b != nil && a.Name() == b.Name() && a.Type() == b.Type() &&
		a.Hash() == b.Hash() && a.Target() == b.Target()
}

// makeHashSet indexes hashes for closure comparison.
func makeHashSet(hashes []plumbing.Hash) map[plumbing.Hash]struct{} {
	set := make(map[plumbing.Hash]struct{}, len(hashes))
	for _, hash := range hashes {
		set[hash] = struct{}{}
	}
	return set
}

// sameHashSet checks whether a traversal matches the captured unique closure.
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

// importObjectStore binds object reads during pack selection to the import lifetime.
type importObjectStore struct {
	// Storer supplies repository metadata and object operations to the encoder.
	storer.Storer

	// ctx bounds object selection to the enclosing import operation.
	ctx context.Context
	// afterObject observes successful reads for cancellation regression tests.
	afterObject func()
}

// EncodedObject reads a complete object while the import remains active.
func (s *importObjectStore) EncodedObject(typ plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	// Stop selection before another source object is read.
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}

	// Read the complete object and expose successful progress to the test seam.
	obj, err := s.Storer.EncodedObject(typ, hash)
	if err == nil && s.afterObject != nil {
		s.afterObject()
	}
	return obj, err
}

// DeltaObject retains a source delta when available, including alternate lookup.
func (s *importObjectStore) DeltaObject(typ plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	// Stop selection before another source object is read.
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}

	// Prefer existing deltas and use complete objects for alternate databases.
	if delta, ok := s.Storer.(storer.DeltaObjectStorer); ok {
		// Read the stored delta representation when the source supports it.
		obj, err := delta.DeltaObject(typ, hash)

		// Delta lookup may exclude alternate object databases; the ordinary
		// lookup still resolves their complete objects.
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return s.EncodedObject(typ, hash)
		}

		// Report a successful object read to the test seam.
		if err == nil && s.afterObject != nil {
			s.afterObject()
		}
		return obj, err
	}
	return s.EncodedObject(typ, hash)
}

// _ is a type assertion
var _ storer.DeltaObjectStorer = (*importObjectStore)(nil)
