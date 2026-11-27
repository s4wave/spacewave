//go:build !js && !tinygo

package spacewave

import (
	"bytes"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// runtimeRoots are the packages a program links to embed the runtime: the
// session, the local provider, Space and World engines, the peer object
// store, the entrypoint, the updater, the launcher and the status projector.
var runtimeRoots = []string{
	"./bldr/desktop/update",
	"./bldr/dist/entrypoint",
	"./bldr/entrypoint/compose",
	"./bldr/entrypoint/storagepath",
	"./bldr/plugin",
	"./core/cdn/bstore/controller",
	"./core/cdn/world/controller",
	"./core/provider/local",
	"./core/provider/spacewave/launcher",
	"./core/provider/spacewave/launcher/controller",
	"./core/resource/desktop/statusprojector",
	"./core/session/controller",
	"./core/space/sobject",
	"./core/space/world",
	"./core/space/world/blocktype",
	"./core/space/world/optypes",
	"./db/object/peer",
	"./net/rpc",
}

// runtimeModules are the modules the runtime roots may link, grouped by what
// the runtime uses them for. A program that embeds the runtime carries every
// module here, so a new one fails the test until it is listed with its use,
// and one no longer linked fails until it is removed.
var runtimeModules = []string{
	// The runtime itself and its bus, RPC, encoding and logging libraries.
	"github.com/aperturerobotics/common",
	"github.com/aperturerobotics/controllerbus",
	"github.com/aperturerobotics/fastjson",
	"github.com/aperturerobotics/json-iterator-lite",
	"github.com/aperturerobotics/protobuf-go-lite",
	"github.com/aperturerobotics/starpc",
	"github.com/aperturerobotics/util",
	"github.com/google/uuid",
	"github.com/pkg/errors",
	"github.com/s4wave/spacewave",
	"github.com/satori/go.uuid",
	"github.com/sirupsen/logrus",
	"golang.org/x/exp",
	"golang.org/x/sync",
	"golang.org/x/sys",
	"golang.org/x/time",

	// Keys, signatures, encryption and content hashes.
	"filippo.io/age",
	"filippo.io/edwards25519",
	"filippo.io/hpke",
	"github.com/aperturerobotics/go-multibase",
	"github.com/bwesterb/go-ristretto",
	"github.com/cespare/xxhash/v2",
	"github.com/cloudflare/circl",
	"github.com/klauspost/cpuid/v2",
	"github.com/mr-tron/base58",
	"github.com/multiformats/go-multihash",
	"github.com/multiformats/go-varint",
	"github.com/spaolacci/murmur3",
	"github.com/zeebo/blake3",
	"golang.org/x/crypto",
	"lukechampine.com/blake3",

	// Peer links: QUIC, WebRTC, websockets and stream multiplexing.
	"github.com/aperturerobotics/go-multiaddr",
	"github.com/aperturerobotics/go-websocket",
	"github.com/libp2p/go-buffer-pool",
	"github.com/libp2p/go-yamux/v5",
	"github.com/pion/datachannel",
	"github.com/pion/dtls/v3",
	"github.com/pion/ice/v4",
	"github.com/pion/interceptor",
	"github.com/pion/logging",
	"github.com/pion/mdns/v2",
	"github.com/pion/randutil",
	"github.com/pion/rtcp",
	"github.com/pion/rtp",
	"github.com/pion/sctp",
	"github.com/pion/sdp/v3",
	"github.com/pion/srtp/v3",
	"github.com/pion/stun/v4",
	"github.com/pion/transport/v5",
	"github.com/pion/turn/v5",
	"github.com/pion/webrtc/v4",
	"github.com/quic-go/quic-go",
	"github.com/wlynxg/anet",
	"golang.org/x/net",

	// Local stores, the World graph, UnixFS and block compression.
	"github.com/aperturerobotics/bbolt",
	"github.com/aperturerobotics/cayley",
	"github.com/aperturerobotics/fsnotify",
	"github.com/aperturerobotics/go-kvfile",
	"github.com/dustin/go-humanize",
	"github.com/go-git/go-billy/v6",
	"github.com/klauspost/compress",
	"github.com/pierrec/lz4/v4",
	"github.com/restic/chunker",
	"github.com/tidwall/btree",

	// The plugin host's WebAssembly and QuickJS runtimes, which also run SQLite.
	"github.com/aperturerobotics/go-quickjs-wasi-reactor",
	"github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs",
	"github.com/tetratelabs/wazero",

	// Windows pipes and job objects for plugin processes.
	"github.com/Microsoft/go-winio",
	"github.com/aperturerobotics/go-winjob",

	// The entrypoint's command line, terminal colors and YAML config.
	"github.com/aperturerobotics/cli",
	"github.com/fatih/color",
	"github.com/ghodss/yaml",
	"github.com/mattn/go-colorable",
	"github.com/mattn/go-isatty",
	"github.com/xrash/smetrics",
	"gopkg.in/yaml.v2",

	// The local provider's SFTP, FTPS and SQL block stores.
	"github.com/go-sql-driver/mysql",
	"github.com/jlaffaye/ftp",
	"github.com/kr/fs",
	"github.com/pkg/sftp",
}

// TestRuntimeRootsLinkOnlyRuntimeModules checks that the Windows build of the
// runtime roots links exactly the modules in runtimeModules, and reports each
// unlisted module with the shortest chain of imports that reaches it.
func TestRuntimeRootsLinkOnlyRuntimeModules(t *testing.T) {
	// One line per package: path, whether it is only a dependency, module
	// (empty in the standard library) and imports.
	format := `{{.ImportPath}}|{{.DepOnly}}|{{with .Module}}{{.Path}}{{end}}|{{join .Imports ","}}`
	args := append([]string{"list", "-deps", "-f", format}, runtimeRoots...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}

	// imports maps each package to its imports and module maps it to its
	// module. reached starts as the roots and grows into the walk's queue.
	imports := make(map[string][]string)
	module := make(map[string]string)
	var reached []string
	for line := range strings.Lines(string(out)) {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) != 4 {
			t.Fatalf("unexpected go list line: %q", line)
		}
		pkg := fields[0]
		module[pkg] = fields[2]
		if fields[3] != "" {
			imports[pkg] = strings.Split(fields[3], ",")
		}
		if fields[1] == "false" {
			reached = append(reached, pkg)
		}
	}
	if len(reached) != len(runtimeRoots) {
		t.Fatalf("go list resolved %d of %d roots", len(reached), len(runtimeRoots))
	}

	// unlinked holds the listed modules no package has reached yet.
	unlinked := make(map[string]struct{}, len(runtimeModules))
	for _, mod := range runtimeModules {
		unlinked[mod] = struct{}{}
	}
	allowed := func(mod string) bool {
		return mod == "" || slices.Contains(runtimeModules, mod)
	}

	// Walk breadth first from the roots without entering an unlisted module,
	// so each is reported once, by its shortest chain.
	parent := make(map[string]string, len(imports))
	for _, root := range reached {
		parent[root] = ""
	}
	reported := make(map[string]struct{})
	for i := 0; i < len(reached); i++ {
		pkg := reached[i]
		delete(unlinked, module[pkg])
		for _, imp := range imports[pkg] {
			if _, seen := parent[imp]; seen {
				continue
			}
			parent[imp] = pkg
			mod := module[imp]
			if allowed(mod) {
				reached = append(reached, imp)
				continue
			}
			if _, done := reported[mod]; done {
				continue
			}
			reported[mod] = struct{}{}
			chain := []string{imp}
			for at := pkg; at != ""; at = parent[at] {
				chain = append(chain, at)
			}
			slices.Reverse(chain)
			t.Errorf("runtime roots link unlisted module %s:\n  %s", mod, strings.Join(chain, "\n  -> "))
		}
	}
	for _, mod := range runtimeModules {
		if _, ok := unlinked[mod]; ok {
			t.Errorf("runtime roots no longer link %s: remove it from runtimeModules", mod)
		}
	}
}
