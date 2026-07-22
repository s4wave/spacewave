package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

const (
	identityFilename = "release-wasm-identity.json"
	currentFilename  = "current"
	generationsDir   = "generations"
)

type publishHooks struct {
	beforeRename             func()
	afterRenameBeforeCurrent func()
}

// Publish copies a complete release and prerender output pair into an immutable
// generation, then atomically makes that generation current.
func Publish(storeDir, releaseDir, prerenderDir string, identity *Identity) (string, string, error) {
	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	return generation.ReleaseDir, generation.PrerenderDir, err
}

// PublishGeneration copies, validates, and atomically publishes one immutable
// release artifact generation.
func PublishGeneration(storeDir, releaseDir, prerenderDir string, identity *Identity) (Generation, error) {
	return publish(storeDir, releaseDir, prerenderDir, identity, publishHooks{})
}

func publish(storeDir, releaseDir, prerenderDir string, identity *Identity, hooks publishHooks) (Generation, error) {
	if identity == nil || identity.Digest == "" || identity.Digest != identity.contentDigest() {
		return Generation{}, errors.New("invalid release artifact identity")
	}
	if err := validateRequiredFiles(releaseDir, prerenderDir); err != nil {
		return Generation{}, err
	}
	if err := os.MkdirAll(filepath.Join(storeDir, generationsDir), 0o755); err != nil {
		return Generation{}, errors.Wrap(err, "create release artifact store")
	}

	stageDir, err := os.MkdirTemp(storeDir, ".publish-")
	if err != nil {
		return Generation{}, errors.Wrap(err, "create release artifact staging directory")
	}
	removeStage := true
	defer func() {
		if removeStage {
			os.RemoveAll(stageDir)
		}
	}()

	stagedRelease := filepath.Join(stageDir, "release")
	stagedPrerender := filepath.Join(stageDir, "prerender")
	if err := copyTree(releaseDir, stagedRelease); err != nil {
		return Generation{}, errors.Wrap(err, "stage release output")
	}
	if err := copyTree(prerenderDir, stagedPrerender); err != nil {
		return Generation{}, errors.Wrap(err, "stage prerender output")
	}

	releaseDigest, err := treeDigest(stagedRelease)
	if err != nil {
		return Generation{}, errors.Wrap(err, "digest staged release output")
	}
	prerenderDigest, err := treeDigest(stagedPrerender)
	if err != nil {
		return Generation{}, errors.Wrap(err, "digest staged prerender output")
	}
	if err := os.WriteFile(
		filepath.Join(stagedRelease, identityFilename),
		marshalManifest(identity, releaseDigest, prerenderDigest),
		0o644,
	); err != nil {
		return Generation{}, errors.Wrap(err, "write staged release artifact identity")
	}
	if err := Validate(stagedRelease, stagedPrerender, identity); err != nil {
		return Generation{}, errors.Wrap(err, "validate staged release artifact")
	}

	generation := identity.Digest + "-" + strings.TrimPrefix(filepath.Base(stageDir), ".publish-")
	generationDir := filepath.Join(storeDir, generationsDir, generation)
	if hooks.beforeRename != nil {
		hooks.beforeRename()
	}
	if err := os.Rename(stageDir, generationDir); err != nil {
		return Generation{}, errors.Wrap(err, "commit release artifact generation")
	}
	removeStage = false
	if hooks.afterRenameBeforeCurrent != nil {
		hooks.afterRenameBeforeCurrent()
	}
	if err := writeCurrent(storeDir, generation); err != nil {
		return Generation{}, err
	}
	return Generation{
		ID:           generation,
		ReleaseDir:   filepath.Join(generationDir, "release"),
		PrerenderDir: filepath.Join(generationDir, "prerender"),
	}, nil
}

// Current returns the atomically published generation when it is complete and
// matches the expected content identity.
func Current(storeDir string, expected *Identity) (string, string, error) {
	data, err := os.ReadFile(filepath.Join(storeDir, currentFilename))
	if err != nil {
		return "", "", errors.Wrap(err, "read current release artifact generation")
	}
	generation := strings.TrimSpace(string(data))
	if generation == "" || filepath.Base(generation) != generation || generation == "." || generation == ".." {
		return "", "", errors.New("invalid current release artifact generation")
	}
	generationDir := filepath.Join(storeDir, generationsDir, generation)
	releaseDir := filepath.Join(generationDir, "release")
	prerenderDir := filepath.Join(generationDir, "prerender")
	if err := Validate(releaseDir, prerenderDir, expected); err != nil {
		return "", "", err
	}
	return releaseDir, prerenderDir, nil
}

// ValidGenerations returns every valid immutable generation matching the
// expected content identity.
func ValidGenerations(storeDir string, expected *Identity) ([]Generation, error) {
	if expected == nil {
		return nil, errors.New("missing expected release artifact identity")
	}
	root := filepath.Join(storeDir, generationsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "read release artifact generations")
	}
	var generations []Generation
	for _, entry := range entries {
		name := entry.Name()
		parts := strings.SplitN(name, "-", 2)
		if parts[0] != expected.Digest {
			continue
		}
		if len(parts) != 2 || parts[1] == "" {
			return nil, errors.Errorf("invalid release artifact generation %q", name)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.Errorf("release artifact generation %q is a symbolic link", name)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, errors.Wrapf(err, "inspect release artifact generation %q", name)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, errors.Errorf("release artifact generation %q is not a directory", name)
		}
		generationDir := filepath.Join(root, name)
		releaseDir := filepath.Join(generationDir, "release")
		prerenderDir := filepath.Join(generationDir, "prerender")
		if err := Validate(releaseDir, prerenderDir, expected); err != nil {
			return nil, err
		}
		generations = append(generations, Generation{
			ID:           name,
			ReleaseDir:   releaseDir,
			PrerenderDir: prerenderDir,
		})
	}
	return generations, nil
}

// Validate rejects incomplete, modified, or stale artifact directory pairs.
func Validate(releaseDir, prerenderDir string, expected *Identity) error {
	if expected == nil {
		return errors.New("missing expected release artifact identity")
	}
	if err := validateRequiredFiles(releaseDir, prerenderDir); err != nil {
		return err
	}
	manifest, releaseDigest, prerenderDigest, err := readManifest(filepath.Join(releaseDir, identityFilename))
	if err != nil {
		return err
	}
	if differences := expected.Differences(manifest); len(differences) != 0 {
		return errors.Errorf("release artifact identity mismatch: %s", strings.Join(differences, ", "))
	}

	actualReleaseDigest, err := treeDigest(releaseDir)
	if err != nil {
		return errors.Wrap(err, "digest release output")
	}
	if actualReleaseDigest != releaseDigest {
		return errors.New("release artifact output digest mismatch")
	}
	actualPrerenderDigest, err := treeDigest(prerenderDir)
	if err != nil {
		return errors.Wrap(err, "digest prerender output")
	}
	if actualPrerenderDigest != prerenderDigest {
		return errors.New("prerender artifact output digest mismatch")
	}
	return nil
}

func validateRequiredFiles(releaseDir, prerenderDir string) error {
	descriptorPath := filepath.Join(releaseDir, "browser-release.json")
	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		return errors.Wrap(err, "read browser-release.json")
	}
	var parser fastjson.Parser
	descriptor, err := parser.ParseBytes(data)
	if err != nil {
		return errors.Wrap(err, "parse browser-release.json")
	}
	if descriptor.GetInt("schemaVersion") <= 0 ||
		len(descriptor.GetStringBytes("generationId")) == 0 ||
		len(descriptor.GetStringBytes("shellAssets", "entrypoint")) == 0 ||
		len(descriptor.GetStringBytes("shellAssets", "serviceWorker")) == 0 ||
		len(descriptor.GetStringBytes("shellAssets", "sharedWorker")) == 0 {
		return errors.New("browser-release.json is incomplete")
	}
	indexInfo, err := os.Stat(filepath.Join(prerenderDir, "index.html"))
	if err != nil {
		return errors.Wrap(err, "stat prerender index.html")
	}
	if !indexInfo.Mode().IsRegular() || indexInfo.Size() == 0 {
		return errors.New("prerender index.html is empty or not a regular file")
	}
	return nil
}

func writeCurrent(storeDir, generation string) error {
	f, err := os.CreateTemp(storeDir, ".current-")
	if err != nil {
		return errors.Wrap(err, "create current release artifact pointer")
	}
	path := f.Name()
	remove := true
	defer func() {
		if remove {
			os.Remove(path)
		}
	}()
	if _, err := f.WriteString(generation + "\n"); err != nil {
		f.Close()
		return errors.Wrap(err, "write current release artifact pointer")
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return errors.Wrap(err, "sync current release artifact pointer")
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close current release artifact pointer")
	}
	if err := os.Rename(path, filepath.Join(storeDir, currentFilename)); err != nil {
		return errors.Wrap(err, "publish current release artifact pointer")
	}
	remove = false
	return nil
}

func marshalManifest(identity *Identity, releaseDigest, prerenderDigest string) []byte {
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("schemaVersion", arena.NewNumberInt(identity.SchemaVersion))
	obj.Set("digest", arena.NewString(identity.Digest))
	obj.Set("compiler", arena.NewString(identity.Compiler))
	obj.Set("mode", arena.NewString(identity.Mode))
	obj.Set("sourceDigest", arena.NewString(identity.SourceDigest))
	obj.Set("buildDigest", arena.NewString(identity.BuildDigest))
	obj.Set("lockfileDigest", arena.NewString(identity.LockfileDigest))
	obj.Set("bldrDigest", arena.NewString(identity.BldrDigest))
	obj.Set("prerenderInputsDigest", arena.NewString(identity.PrerenderInputsDigest))
	obj.Set("releaseOutputDigest", arena.NewString(releaseDigest))
	obj.Set("prerenderOutputDigest", arena.NewString(prerenderDigest))
	return append(obj.MarshalTo(nil), '\n')
}

func readManifest(path string) (*Identity, string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "read release artifact identity")
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "parse release artifact identity")
	}
	identity := &Identity{
		SchemaVersion:         value.GetInt("schemaVersion"),
		Digest:                string(value.GetStringBytes("digest")),
		Compiler:              string(value.GetStringBytes("compiler")),
		Mode:                  string(value.GetStringBytes("mode")),
		SourceDigest:          string(value.GetStringBytes("sourceDigest")),
		BuildDigest:           string(value.GetStringBytes("buildDigest")),
		LockfileDigest:        string(value.GetStringBytes("lockfileDigest")),
		BldrDigest:            string(value.GetStringBytes("bldrDigest")),
		PrerenderInputsDigest: string(value.GetStringBytes("prerenderInputsDigest")),
	}
	releaseDigest := string(value.GetStringBytes("releaseOutputDigest"))
	prerenderDigest := string(value.GetStringBytes("prerenderOutputDigest"))
	if identity.Digest == "" || identity.SourceDigest == "" || identity.BuildDigest == "" ||
		identity.LockfileDigest == "" || identity.BldrDigest == "" || identity.PrerenderInputsDigest == "" ||
		releaseDigest == "" || prerenderDigest == "" {
		return nil, "", "", errors.New("release artifact identity is incomplete")
	}
	return identity, releaseDigest, prerenderDigest, nil
}

func treeDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == identityFilename {
			return nil
		}
		return hashFile(h, root, rel)
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if !info.Mode().IsRegular() {
			return errors.Errorf("artifact output %s has unsupported mode %s", path, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		return closeInErr
	})
}
