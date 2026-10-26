//go:build !goscript

// Package bldr_util_qjsbytecode caches ES modules compiled to bytecode by the
// QuickJS-NG WASI reactor, so a later QuickJS instance links a module without
// parsing its source.
package bldr_util_qjsbytecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	quickjswasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
	quickjs "github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Ext is the file extension of a cached module.
const Ext = ".qbc"

// reactorSum identifies the QuickJS reactor build that reads and writes the
// cached bytecode.
var reactorSum = sync.OnceValue(func() [sha256.Size]byte {
	return sha256.Sum256(quickjswasi.QuickJSWASM)
})

// Cache stores module bytecode in a directory, with one subdirectory for each
// owner, such as a plugin. Each module has one file, named by the SHA-256 of
// the reactor build, the module name and the module source. QuickJS trusts
// the bytecode it reads, so only this process writes the cache.
type Cache struct {
	// dir is the cache root directory.
	dir string
}

// NewCache returns a cache rooted at dir.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

// OwnerDir returns the directory of an owner's modules.
func (c *Cache) OwnerDir(owner string) string {
	return filepath.Join(c.dir, owner)
}

// LoadModule registers the module name with the source src in qjs, from the
// owner's cached bytecode or by compiling src and caching its bytecode. It
// returns the module's cache file name and whether the cache held it.
//
// An entry QuickJS rejects is discarded and compiled again, and a failure to
// write the cache is logged. A module that does not compile returns the
// error.
func (c *Cache) LoadModule(ctx context.Context, le *logrus.Entry, qjs *quickjs.QuickJS, owner, name string, src []byte) (string, bool, error) {
	// Load the cached bytecode, discarding an entry QuickJS rejects.
	file := FileName(name, src)
	ownerDir := c.OwnerDir(owner)
	cachePath := filepath.Join(ownerDir, file)
	bytecode, err := os.ReadFile(cachePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return file, false, err
	}
	if err == nil {
		loadErr := qjs.LoadModule(ctx, bytecode)
		if loadErr == nil {
			return file, true, nil
		}
		le.WithError(loadErr).WithField("module", name).Warn("discarding cached quickjs module bytecode")
		_ = os.Remove(cachePath)
	}

	// Compile the module and cache its bytecode.
	bytecode, err = qjs.CompileModule(ctx, string(src), name)
	if err != nil {
		return file, false, err
	}
	err = os.MkdirAll(ownerDir, 0o755)
	if err == nil {
		err = writeFileAtomic(cachePath, bytecode)
	}
	if err != nil {
		le.WithError(err).WithField("module", name).Warn("cannot cache quickjs module bytecode")
	}
	return file, false, nil
}

// Prune removes the owner's cached modules whose file names are not in used.
func (c *Cache) Prune(owner string, used map[string]struct{}) error {
	// List the owner's cache directory, which may not exist yet.
	ownerDir := c.OwnerDir(owner)
	entries, err := os.ReadDir(ownerDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	// Remove each cached module the owner no longer uses.
	for _, entry := range entries {
		file := entry.Name()
		if _, ok := used[file]; !ok && strings.HasSuffix(file, Ext) {
			_ = os.Remove(filepath.Join(ownerDir, file))
		}
	}
	return nil
}

// DeleteOwner removes the cached modules of an owner.
func (c *Cache) DeleteOwner(owner string) error {
	return os.RemoveAll(c.OwnerDir(owner))
}

// FileName returns the cache file name of a module.
func FileName(name string, src []byte) string {
	// Hash the reactor build, the module name and the source.
	sum := reactorSum()
	h := sha256.New()
	_, _ = h.Write(sum[:])
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(src)
	return hex.EncodeToString(h.Sum(nil)) + Ext
}

// writeFileAtomic writes a file through a temporary file and a rename, so a
// concurrent reader sees the whole file or none of it.
func writeFileAtomic(name string, data []byte) error {
	// Write the data to a temporary file beside the target.
	tmp, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	// Rename it over the target, or remove it on failure.
	if err == nil {
		err = os.Rename(tmp.Name(), name)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
