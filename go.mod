module github.com/aperturerobotics/bldr

go 1.26.0

// This uses wasi-reactor
require (
	github.com/aperturerobotics/go-quickjs-wasi-reactor v0.12.2-0.20260216043809-e2be8a854e6e // master
	github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs v0.0.0-20260216043809-e2be8a854e6e // master
)

// https://github.com/wazero/wazero/pull/2479
// https://github.com/wazero/wazero/pull/2481
replace github.com/tetratelabs/wazero => github.com/aperturerobotics/wazero v0.0.0-20260304193718-46de011b30f6 // aperture-2

require (
	github.com/aperturerobotics/abseil-cpp v0.0.0-20260131110040-4bb56e2f9017 // indirect
	github.com/aperturerobotics/bbolt v0.0.0-20260423202023-7ebe1503eea2 // indirect
	github.com/aperturerobotics/bldr-saucer v0.4.4 // master
	github.com/aperturerobotics/cli v1.1.0
	github.com/aperturerobotics/common v0.32.7 // latest
	github.com/aperturerobotics/cpp-yamux v0.0.0-20260223122921-58339cfd0e5d // master
	github.com/aperturerobotics/esbuild v0.24.1-0.20260219011422-6d4b923e2023 // https://github.com/evanw/esbuild/pull/3413 [rejected]
	github.com/aperturerobotics/go-multiaddr v0.17.0 // indirect
	github.com/aperturerobotics/go-protoc-gen-prost v0.0.0-20260329113538-218ccd8f20e0 // indirect
	github.com/aperturerobotics/go-protoc-wasi v0.0.0-20260329113540-600516012db3 // indirect
	github.com/aperturerobotics/hydra v0.0.0-20260425015526-ed95ca357ff1 // master
	github.com/aperturerobotics/protobuf v0.0.0-20260203024654-8201686529c4 // indirect
	github.com/aperturerobotics/protobuf-go-lite v0.13.0 // master
	github.com/aperturerobotics/saucer v0.0.0-20260317232052-4db05a4e0b4c // indirect; bldr/saucer-streaming; indirect
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/fatih/color v1.17.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/sergi/go-diff v1.4.0
	github.com/tetratelabs/wazero v1.11.0
	golang.org/x/mod v0.35.0 // latest
	golang.org/x/tools v0.43.0 // latest
)

// Note: the below is from the Hydra go.mod

require (
	github.com/aperturerobotics/bifrost v0.47.8-0.20260423185612-1f43d73f06cd // master
	github.com/aperturerobotics/cayley v0.12.1-0.20260425001904-6547b3cc760a // latest
	github.com/aperturerobotics/go-indexeddb v0.2.3 // master
	github.com/aperturerobotics/go-kvfile v0.10.1-0.20260423183349-fcbaa93292c0 // master
	github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20260223122953-12a7c334f634 // indirect; latest
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/aperturerobotics/go-mysql-server v0.18.2-0.20240821042240-d51583de8ec0 // aperture
	github.com/dolthub/vitess => github.com/aperturerobotics/vitess v0.0.0-20240821040752-39ac045ae8fe // aperture
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
)

// aperture: use compatibility forks
// https://github.com/dgraph-io/badger/pull/2048
replace github.com/dgraph-io/badger/v4 => github.com/aperturerobotics/badger-go/v4 v4.0.0-20241029084129-c1a1dbed1aac // main

require (
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/bits-and-blooms/bitset v1.14.3 // indirect
	github.com/bits-and-blooms/bloom/v3 v3.7.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgraph-io/badger/v4 v4.9.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-billy/v6 v6.0.0-20260424211911-732291493fb8
	github.com/paralin/gonum-graph-simple v0.0.0-20240410084948-b970da5ebf33 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/restic/chunker v0.4.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	golang.org/x/sync v0.20.0
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.53.1 // latest
	github.com/aperturerobotics/entitygraph v0.11.0 // indirect; latest
	github.com/aperturerobotics/starpc v0.49.6 // latest
	github.com/aperturerobotics/util v1.34.3 // latest
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/blang/semver/v4 v4.0.0
	github.com/klauspost/compress v1.18.5
	github.com/libp2p/go-yamux/v4 v4.0.2 // indirect
	github.com/mr-tron/base58 v1.3.0
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pion/datachannel v1.6.0 // indirect
	github.com/pion/sdp/v3 v3.0.18 // indirect
	github.com/pion/webrtc/v4 v4.2.11 // indirect
	github.com/pkg/errors v0.9.1
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/sirupsen/logrus v1.9.5-0.20260309202648-9f0600962f75
	github.com/zeebo/blake3 v0.2.4
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
)

require (
	github.com/aperturerobotics/fastjson v0.1.1
	github.com/aperturerobotics/fsnotify v1.9.1-0.20260329111252-827e5e9feeab
	github.com/aperturerobotics/go-websocket v1.8.15-0.20260329113544-74dbfb8f11c6
	github.com/aperturerobotics/go-winjob v0.0.0-20260419023811-62a6b1330891
	github.com/mattn/go-isatty v0.0.20
	github.com/playwright-community/playwright-go v0.5700.1
	go.starlark.net v0.0.0-20260326113308-fadfc96def35
)

require (
	github.com/aperturerobotics/go-multibase v0.4.0 // indirect
	github.com/bwesterb/go-ristretto v1.2.3 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/dgraph-io/ristretto/v2 v2.2.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.4 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/gomodule/redigo v1.9.3 // indirect
	github.com/google/flatbuffers v25.2.10+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hack-pad/safejs v0.1.1 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-sqlite3 v2.0.3+incompatible // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/ncruces/go-sqlite3 v0.33.2 // indirect
	github.com/ncruces/go-sqlite3-wasm v1.0.4-0.20260329114232-2491c387476c // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/pion/dtls/v3 v3.1.2 // indirect
	github.com/pion/ice/v4 v4.2.2 // indirect
	github.com/pion/interceptor v0.1.44 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.1.0 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.16 // indirect
	github.com/pion/rtp v1.10.1 // indirect
	github.com/pion/sctp v1.9.4 // indirect
	github.com/pion/srtp/v3 v3.0.10 // indirect
	github.com/pion/stun/v3 v3.1.1 // indirect
	github.com/pion/transport/v4 v4.0.1 // indirect
	github.com/pion/turn/v4 v4.1.4 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/tidwall/btree v1.8.1 // indirect
	github.com/tylertreat/BoomFilters v0.0.0-20251117164519-53813c36cc1b // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.43.1-0.20260414013634-54fe89f84115 // indirect
	golang.org/x/time v0.12.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.45.0 // indirect
)
