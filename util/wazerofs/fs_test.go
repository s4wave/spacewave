package wazerofs

import (
	"context"
	"os"
	"testing"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	"github.com/go-git/go-billy/v5/memfs"
	billy_util "github.com/go-git/go-billy/v5/util"
	quickjs_wasi "github.com/paralin/go-quickjs-wasi"
	"github.com/tetratelabs/wazero"
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
	ctx := context.Background()

	// create fs root
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err.Error())
	}

	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	defer fsc.Release()

	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsh.Release()

	// create a sample JavaScript file
	err = billy_util.WriteFile(bfs, "index.js", []byte(jsScript), 0644)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a test file with expected content
	err = billy_util.WriteFile(bfs, "test.txt", []byte(testFileContent), 0644)
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
