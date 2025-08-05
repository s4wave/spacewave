module github.com/aperturerobotics/bldr

go 1.24.4

// This fork avoids importing net/http on wasm.
replace github.com/coder/websocket => github.com/paralin/nhooyr-websocket v1.8.13-0.20240820051708-db89d1b29ef8 // aperture-2

// https://github.com/evanw/esbuild/pull/3413 [rejected]
replace github.com/evanw/esbuild => github.com/aperturerobotics/esbuild v0.24.1-0.20250428230204-8c668b313064 // aperture

// https://github.com/tetratelabs/wazero/issues/1500#issuecomment-3041125375
replace github.com/tetratelabs/wazero => github.com/aperturerobotics/wazero v0.0.0-20250706223739-81a39a0d5d54 // aperture

require (
	github.com/aperturerobotics/cli v1.0.0
	github.com/aperturerobotics/common v0.22.9
	github.com/aperturerobotics/hydra v0.0.0-20250713224130-2c38f1cadef5 // master
	github.com/aperturerobotics/protobuf-go-lite v0.10.1 // master
)

require (
	github.com/Microsoft/go-winio v0.6.2
	github.com/evanw/esbuild v0.25.0 // latest
	github.com/fatih/color v1.17.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kolesnikovae/go-winjob v1.0.1-0.20200702113133-049537be0656 // master
	github.com/paralin/go-quickjs-wasi v0.10.2-0.20250704023701-828038c31a0f // latest
	github.com/sergi/go-diff v1.4.0
	github.com/tetratelabs/wazero v1.9.0
	golang.org/x/mod v0.26.0 // latest
	golang.org/x/tools v0.35.0 // latest
)

// Note: the below is from the Hydra go.mod

require (
	github.com/aperturerobotics/bifrost v0.42.4-0.20250713225909-9284b43aab6e // master
	github.com/aperturerobotics/cayley v0.9.1 // latest
	github.com/aperturerobotics/go-indexeddb v0.2.3 // indirect; master
	github.com/aperturerobotics/go-kvfile v0.9.2 // master
	github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20241223092408-d525fa878b3e // indirect; latest
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/aperturerobotics/go-mysql-server v0.18.2-0.20240821042240-d51583de8ec0 // aperture
	github.com/dolthub/vitess => github.com/aperturerobotics/vitess v0.0.0-20240821040752-39ac045ae8fe // aperture
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
	xorm.io/xorm => github.com/paralin/go-xorm v1.3.3-0.20230216084813-0cd923e7ced6 // ext-engines
)

// aperture: use compatibility forks
replace (
	// https://github.com/dgraph-io/badger/pull/2048
	github.com/dgraph-io/badger/v4 => github.com/aperturerobotics/badger-go/v4 v4.0.0-20241029084129-c1a1dbed1aac // main
	github.com/hidal-go/hidalgo => github.com/aperturerobotics/hidalgo v0.3.1-0.20231111025334-8015549a1b51 // aperture
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.12.2-0.20220323132038-01665499027f // aperture
)

require (
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/bits-and-blooms/bitset v1.14.3 // indirect
	github.com/bits-and-blooms/bloom/v3 v3.7.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgraph-io/badger/v4 v4.3.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-billy/v5 v5.6.0
	github.com/paralin/gonum-graph-simple v0.0.0-20240410084948-b970da5ebf33 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/restic/chunker v0.4.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	go.etcd.io/bbolt v1.4.0-beta.0.0.20241216162354-7b2d3609bf79 // indirect
	golang.org/x/sync v0.16.0
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.50.4-0.20250706040906-c1ba863ae6ba // latest
	github.com/aperturerobotics/entitygraph v0.11.0 // indirect; latest
	github.com/aperturerobotics/starpc v0.39.7 // latest
	github.com/aperturerobotics/util v1.31.0 // latest
)

// aperture: use compatibility forks
replace (
	github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master
	github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture
	github.com/libp2p/go-msgio => github.com/aperturerobotics/go-libp2p-msgio v0.0.0-20240511033615-1b69178aa5c8 // aperture
	github.com/multiformats/go-multiaddr => github.com/aperturerobotics/go-multiaddr v0.12.4-0.20240407071906-6f0354cc6755 // aperture
	github.com/multiformats/go-multihash => github.com/aperturerobotics/go-multihash v0.2.3 // aperture
	github.com/quic-go/quic-go => github.com/aperturerobotics/quic-go v0.48.2-0.20241029082227-fa76c393ee89 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
)

require (
	filippo.io/edwards25519 v1.1.1-0.20250211130249-04b037b40df0 // indirect
	github.com/blang/semver/v4 v4.0.0
	github.com/coder/websocket v1.8.13
	github.com/klauspost/compress v1.18.0
	github.com/libp2p/go-libp2p v0.42.1
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.16.0 // indirect
	github.com/oklog/ulid/v2 v2.1.0
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pion/datachannel v1.5.10 // indirect
	github.com/pion/sdp/v3 v3.0.14 // indirect
	github.com/pion/webrtc/v4 v4.1.2 // indirect
	github.com/pkg/errors v0.9.1
	github.com/quic-go/quic-go v0.53.0 // indirect
	github.com/sirupsen/logrus v1.9.3
	github.com/zeebo/blake3 v0.2.4
	golang.org/x/crypto v0.40.0
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	gonum.org/v1/gonum v0.16.0 // indirect
)

require (
	github.com/cyphar/filepath-securejoin v0.2.5 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/dgraph-io/ristretto/v2 v2.0.0-alpha // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/gomodule/redigo v1.9.2 // indirect
	github.com/google/flatbuffers v24.3.25+incompatible // indirect
	github.com/google/pprof v0.0.0-20241017200806-017d972448fc // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hack-pad/safejs v0.1.1 // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.2-0.20240826150533-e92055b23e0e // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/onsi/ginkgo/v2 v2.20.2 // indirect
	github.com/pion/dtls/v3 v3.0.6 // indirect
	github.com/pion/ice/v4 v4.0.10 // indirect
	github.com/pion/interceptor v0.1.40 // indirect
	github.com/pion/logging v0.2.3 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.8.18 // indirect
	github.com/pion/sctp v1.8.39 // indirect
	github.com/pion/srtp/v3 v3.0.5 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.0.0 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	github.com/tidwall/btree v1.7.0 // indirect
	github.com/tylertreat/BoomFilters v0.0.0-20210315201527-1a82519a3e43 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/mock v0.5.0 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)
