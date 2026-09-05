//go:build !js

package dist_entrypoint

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

func TestNativeAssetsResource(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "app")
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "app-link")
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	assets := nativeAssetsFS{
		FS:         fstest.MapFS{"config-set.bin": {Data: []byte("config")}},
		executable: func() (string, error) { return link, nil },
	}
	t.Chdir(t.TempDir())
	if _, err := assets.Open("assets.kvfile"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing volume error = %v", err)
	}
	data := []byte("root block")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ref.MarshalKey()
	if err != nil {
		t.Fatal(err)
	}
	var volume bytes.Buffer
	writer := kvfile.NewWriter(&volume)
	if err := writer.WriteValue(store_kvkey.NewDefaultKVKey().GetBlockKey(key), bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets.kvfile"), volume.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := assets.Open("assets.kvfile")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := file.(io.ReaderAt); !ok {
		t.Fatal("volume must retain random-access reads")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	config, err := fs.ReadFile(assets, "config-set.bin")
	if err != nil || string(config) != "config" {
		t.Fatalf("embedded config = %q, error = %v", config, err)
	}
	resolve := newStaticBlockStoreReaderBuilder(nil, assets, false, ref)
	_, closeReader, err := resolve(t.Context(), func() {})
	if err != nil {
		t.Fatal(err)
	}
	closeReader()
	otherRef, err := block.BuildBlockRef([]byte("other root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = newStaticBlockStoreReaderBuilder(nil, assets, false, otherRef)(t.Context(), func() {})
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("mismatched volume error = %v", err)
	}
}
