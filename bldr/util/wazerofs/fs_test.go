package wazerofs

import (
	"context"
	"os"
	"testing"

	quickjs_wasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/tetratelabs/wazero"
	wazero_exp_sys "github.com/tetratelabs/wazero/experimental/sys"
	wazero_exp_sysfs "github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// testFileContent stores the expected content for the test file
const testFileContent = "hello world test content"

// jsScript stores the JavaScript code to run in the QuickJS VM
const jsScript = `
console.log("hello world from quickjs");

// Read the test file and verify its contents
const file = std.open('test.txt', 'r');
const content = file.readAsString();
file.close();

const expected = 'hello world test content';
if (content === expected) {
	console.log('File content verification passed!');
} else {
	console.log('File content verification failed! Expected:', expected, 'Got:', content);
	std.exit(1);
}
`

func TestWazeroFS(t *testing.T) {
	// Share one context across UnixFS operations and the Wazero test runtime.
	ctx := context.Background()

	// create fs root
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// Open a Billy-backed UnixFS cursor for the test filesystem.
	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	defer fsc.Release()

	// Open a UnixFS handle for the Wazero filesystem adapter.
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsh.Release()

	// create a sample JavaScript file
	err = billy_util.WriteFile(bfs, "index.js", []byte(jsScript), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a test file with expected content
	err = billy_util.WriteFile(bfs, "test.txt", []byte(testFileContent), 0o644)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Create the wazero fs adapter
	wazeroFs := NewFS(ctx, fsh, nil)

	// Combine the above into our baseline config, overriding defaults.
	// By default, I/O streams are discarded and there's no file system.
	config := wazero.NewModuleConfig().
		WithName("").
		WithStdout(os.Stderr).
		WithStderr(os.Stderr)

	// Mount the UnixFS adapter at the Wazero module filesystem root.
	fsConfig := wazero.NewFSConfig().(wazero_exp_sysfs.FSConfig).WithSysFSMount(wazeroFs, "/")
	config = config.WithFSConfig(fsConfig)
	_ = wazeroFs

	// Instantiate WASI, which implements system call APIs.
	// This is required for the Wasm module to print to the console.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Instantiate the Wasm module.
	// This will automatically run the "_start" function of the module.
	mod, err := r.InstantiateWithConfig(
		ctx,
		quickjs_wasi.QuickJSWASM,
		config.WithArgs(quickjs_wasi.QuickJSWASMFilename, "--std", "index.js"),
		// config.WithArgs(quickjs_wasi.QuickJSWASMFilename, "--std", "-e", jsScript),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = mod
}

// TestFileReadOnlyRejectsWrites verifies that a read-only open rejects writes.
func TestFileReadOnlyRejectsWrites(t *testing.T) {
	// Build a memory filesystem holding one file.
	bfs := memfs.New()
	if err := billy_util.WriteFile(bfs, "test.txt", []byte(testFileContent), 0o644); err != nil {
		t.Fatal(err.Error())
	}

	// Serve the filesystem through the Wazero adapter.
	ctx := context.Background()
	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	defer fsc.Release()
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsh.Release()
	wfs := NewFS(ctx, fsh, nil)

	// Expect both write paths to fail on a read-only open.
	ro, errno := wfs.OpenFile("test.txt", wazero_exp_sys.O_RDONLY, 0)
	if errno != 0 {
		t.Fatalf("open read-only: %v", errno)
	}
	defer ro.Close()
	if _, errno := ro.Write([]byte("x")); errno != wazero_exp_sys.EBADF {
		t.Fatalf("read-only Write errno = %v, want EBADF", errno)
	}
	if _, errno := ro.Pwrite([]byte("x"), 0); errno != wazero_exp_sys.EBADF {
		t.Fatalf("read-only Pwrite errno = %v, want EBADF", errno)
	}

	// Expect a read-write open to accept the write.
	rw, errno := wfs.OpenFile("test.txt", wazero_exp_sys.O_RDWR, 0)
	if errno != 0 {
		t.Fatalf("open read-write: %v", errno)
	}
	defer rw.Close()
	if _, errno := rw.Pwrite([]byte("x"), 0); errno != 0 {
		t.Fatalf("read-write Pwrite errno = %v", errno)
	}
}
