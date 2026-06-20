module github.com/s4wave/spacewave

go 1.26.3

replace github.com/go-git/go-git/v6 => github.com/aperturerobotics/go-git/v6 v6.0.0-alpha.3.0.20260619055138-cbfe9ded8794

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/aperturerobotics/go-mysql-server v0.20.1-0.20260620015219-5ec6a474cd27 // aperture-5
	github.com/dolthub/vitess => github.com/aperturerobotics/vitess v0.0.0-20260620015210-7020baa15de7 // aperture
)

// aperture: use compatibility forks
// https://github.com/dgraph-io/badger/pull/2048
replace github.com/dgraph-io/badger/v4 => github.com/aperturerobotics/badger-go/v4 v4.0.0-20241029084129-c1a1dbed1aac // main

replace github.com/dgraph-io/ristretto/v2 => github.com/aperturerobotics/ristretto/v2 v2.0.0-20260619193556-b7aa4f19bce9 // main

// https://github.com/wazero/wazero/pull/2479
// https://github.com/wazero/wazero/pull/2481
replace github.com/tetratelabs/wazero => github.com/aperturerobotics/wazero v0.0.0-20260304193718-46de011b30f6 // aperture-2

require (
	github.com/aperturerobotics/abseil-cpp v0.0.0-20260131110040-4bb56e2f9017 // indirect
	github.com/aperturerobotics/bbolt v0.0.0-20260617204204-71b28cbadcb4 // master
	github.com/aperturerobotics/bldr-saucer v0.4.4
	github.com/aperturerobotics/cayley v0.14.1-0.20260604043655-9ed25e8f78ac // latest
	github.com/aperturerobotics/cli v1.1.0 // latest
	github.com/aperturerobotics/common v0.34.0 // latest
	github.com/aperturerobotics/controllerbus v0.53.4 // latest
	github.com/aperturerobotics/cpp-yamux v0.0.0-20260223122921-58339cfd0e5d
	github.com/aperturerobotics/esbuild v0.24.1-0.20260219011422-6d4b923e2023 // https://github.com/evanw/esbuild/pull/3413 [rejected]
	github.com/aperturerobotics/fastjson v0.1.2-0.20260619070337-5d150feb5e98
	github.com/aperturerobotics/fsnotify v1.9.1-0.20260506231828-931cb4bf1761 // master
	github.com/aperturerobotics/go-brotli-decoder v1.2.2
	github.com/aperturerobotics/go-indexeddb v0.2.3 // master
	github.com/aperturerobotics/go-kvfile v0.10.1-0.20260423183349-fcbaa93292c0 // master
	github.com/aperturerobotics/go-multiaddr v0.17.1-0.20260514224402-c193991c3ce5
	github.com/aperturerobotics/go-multibase v0.4.0 // indirect
	github.com/aperturerobotics/go-protoc-gen-prost v0.0.0-20260329113538-218ccd8f20e0 // indirect
	github.com/aperturerobotics/go-protoc-wasi v0.0.0-20260329113540-600516012db3 // indirect
	github.com/aperturerobotics/go-quickjs-wasi-reactor v0.15.1
	github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs v0.0.0-20260614231932-9ac47eecfff5
	github.com/aperturerobotics/go-websocket v1.8.15-0.20260619192713-a096778f08c1
	github.com/aperturerobotics/go-winjob v0.0.0-20260419024155-c5bdc0ba375d
	github.com/aperturerobotics/goscript v0.2.5-0.20260619072609-cdca210ee33e
	github.com/aperturerobotics/json-iterator-lite v1.1.0 // latest
	github.com/aperturerobotics/protobuf v0.0.0-20260203024654-8201686529c4 // indirect
	github.com/aperturerobotics/protobuf-go-lite v0.14.0 // master
	github.com/aperturerobotics/saucer v0.0.0-20260317232052-4db05a4e0b4c // indirect
	github.com/aperturerobotics/starpc v0.49.18 // latest
	github.com/aperturerobotics/util v1.34.10-0.20260610174257-740fe32c1ea1 // latest
)

require (
	filippo.io/age v1.3.1
	filippo.io/edwards25519 v1.2.0
)

require (
	golang.org/x/crypto v0.53.0
	golang.org/x/mod v0.37.0 // latest
	golang.org/x/sync v0.21.0
	golang.org/x/sys v0.46.0
	golang.org/x/term v0.44.0
	golang.org/x/tools v0.46.0 // latest
)

require (
	bazil.org/fuse v0.0.0-20230120002735-62a210ff1fd5
	github.com/go-sql-driver/mysql v1.10.0
	github.com/pion/logging v0.2.4
	github.com/pion/transport/v4 v4.0.2
	go.starlark.net v0.0.0-20260613233743-8ba36ccb83fb
	golang.org/x/exp v0.0.0-20260611194520-c48552f49976
	gonum.org/v1/gonum v0.17.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gorm.io/gorm v1.31.1
	gotest.tools/v3 v3.5.2
	modernc.org/sqlite v1.52.0
)

require (
	github.com/cloudflare/circl v1.6.3
	github.com/creack/pty v1.1.24
	github.com/dgraph-io/badger/v4 v4.9.2
	github.com/dgraph-io/ristretto/v2 v2.4.0
	github.com/dolthub/go-mysql-server v0.20.0
	github.com/dolthub/vitess v0.0.0-20260617012411-2f308f6cdc23
	github.com/dustin/go-humanize v1.0.1
	github.com/emirpasic/gods v1.18.1
	github.com/fatih/color v1.19.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-billy/v6 v6.0.0-alpha.1.0.20260519112248-0095b064a6c6 // main
	github.com/go-git/go-git/v6 v6.0.0-alpha.4 // main
	github.com/goccy/go-json v0.10.6
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/google/uuid v1.6.0
	github.com/hack-pad/safejs v0.1.1
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/klauspost/compress v1.18.6
	github.com/manifoldco/promptui v0.9.0
	github.com/mattn/go-isatty v0.0.22
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/mr-tron/base58 v1.3.0
	github.com/ncruces/go-sqlite3 v0.35.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pierrec/lz4/v4 v4.1.27
	github.com/pion/datachannel v1.6.0
	github.com/pion/sdp/v3 v3.0.18
	github.com/pion/webrtc/v4 v4.2.15
	github.com/pkg/errors v0.9.1
	github.com/playwright-community/playwright-go v0.5700.1
	github.com/quic-go/quic-go v0.60.0
	github.com/restic/chunker v0.4.0
	github.com/sasha-s/go-deadlock v0.3.9
	github.com/satori/go.uuid v1.2.0
	github.com/sergi/go-diff v1.4.0
	github.com/sirupsen/logrus v1.9.5-0.20260508084601-d4a50659cfd6
	github.com/spf13/afero v1.15.0
	github.com/spf13/cast v1.10.0
	github.com/tetratelabs/wazero v1.12.0
	github.com/tidwall/btree v1.8.1
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/whilp/git-urls v1.0.0
	github.com/zeebo/blake3 v0.2.4
)

replace github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.5-0.20260430110313-9c892333814d

require (
	filippo.io/hpke v0.4.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/bwesterb/go-ristretto v1.2.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/dolthub/flatbuffers/v23 v23.3.3-dh.2 // indirect
	github.com/dolthub/go-icu-regex v0.0.0-20260610153742-72563bc7ca83 // indirect
	github.com/dolthub/jsonpath v0.0.2-0.20240227200619-19675ab05c71 // indirect
	github.com/go-jose/go-jose/v3 v3.0.5 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/flatbuffers v25.2.10+incompatible // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lestrrat-go/strftime v1.2.0 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/ncruces/go-sqlite3-wasm/v3 v3.1.35302 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/petermattis/goid v0.0.0-20250813065127-a731cc31b4fe // indirect
	github.com/pion/dtls/v3 v3.1.4 // indirect
	github.com/pion/ice/v4 v4.2.7 // indirect
	github.com/pion/interceptor v0.1.45 // indirect
	github.com/pion/mdns/v2 v2.1.0 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.16 // indirect
	github.com/pion/rtp v1.10.2 // indirect
	github.com/pion/sctp v1.10.0 // indirect
	github.com/pion/srtp/v3 v3.0.11 // indirect
	github.com/pion/stun/v3 v3.1.5 // indirect
	github.com/pion/turn/v5 v5.0.9 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	github.com/zeebo/assert v1.3.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/trace v1.41.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/telemetry v0.0.0-20260610154732-fb80ec83bdd9 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	gopkg.in/src-d/go-errors.v1 v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/dolthub/jsonpath => github.com/aperturerobotics/jsonpath v0.1.5-0.20260620020844-bd6430df1008

replace github.com/pion/webrtc/v4 => github.com/aperturerobotics/pion-webrtc/v4 v4.2.16-0.20260620023100-dab2ab5b7c65
