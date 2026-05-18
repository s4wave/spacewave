//go:build js && !bldr_indexeddb

package browser_storage

import (
	"runtime"
	"testing"

	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
)

func TestOpfsStorageUsesAsyncIOForTinyGo(t *testing.T) {
	conf, err := NewOpfsStorage("prefix/").BuildVolumeConfig("state", nil)
	if err != nil {
		t.Fatal(err)
	}

	opfsConf, ok := conf.(*volume_opfs.Config)
	if !ok {
		t.Fatalf("BuildVolumeConfig returned %T, want *volume_opfs.Config", conf)
	}
	if got, want := opfsConf.GetAsyncIo(), runtime.Compiler == "tinygo"; got != want {
		t.Fatalf("AsyncIo = %v, want %v", got, want)
	}
}
