module github.com/aperturerobotics/bldr

go 1.23

toolchain go1.23.1

// This fork avoids importing net/http on wasm.
replace github.com/coder/websocket => github.com/paralin/nhooyr-websocket v1.8.13-0.20240820051708-db89d1b29ef8 // aperture-2

require github.com/aperturerobotics/common v0.19.1 // latest

// https://github.com/evanw/esbuild/pull/3413 [rejected]
replace github.com/evanw/esbuild => github.com/aperturerobotics/esbuild v0.24.1-0.20241128081004-9038c4b3a19a // aperture

require (
	github.com/aperturerobotics/hydra v0.0.0-20241124044113-69000824bb4a // master
	github.com/aperturerobotics/protobuf-go-lite v0.8.0 // master
)

require (
	github.com/Microsoft/go-winio v0.6.2
	github.com/evanw/esbuild v0.23.0 // latest
	github.com/fatih/color v1.17.0
	github.com/fsnotify/fsnotify v1.8.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kolesnikovae/go-winjob v1.0.1-0.20200702113133-049537be0656 // master
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3
	github.com/tetratelabs/wazero v1.7.3 // latest
	golang.org/x/mod v0.22.0 // latest
	golang.org/x/tools v0.27.0 // latest
)

// Note: the below is from the Hydra go.mod

require (
	github.com/aperturerobotics/bifrost v0.40.1-0.20241120022637-6f48264d0c5e // master
	github.com/aperturerobotics/cayley v0.9.1 // latest
	github.com/aperturerobotics/go-indexeddb v0.2.3 // indirect; master
	github.com/aperturerobotics/go-kvfile v0.7.3 // master
	github.com/aperturerobotics/json-iterator-lite v1.0.0 // indirect; latest
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
	go.etcd.io/bbolt v1.3.11 // indirect
	golang.org/x/sync v0.9.0
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.48.1 // latest
	github.com/aperturerobotics/entitygraph v0.10.1 // indirect; latest
	github.com/aperturerobotics/starpc v0.35.3 // latest
	github.com/aperturerobotics/util v1.26.4-0.20241121063346-dbe98d45029b // latest
)

// aperture: use compatibility forks
replace (
	github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master
	github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture
	github.com/libp2p/go-msgio => github.com/aperturerobotics/go-libp2p-msgio v0.0.0-20240511033615-1b69178aa5c8 // aperture
	github.com/multiformats/go-multiaddr => github.com/aperturerobotics/go-multiaddr v0.12.4-0.20240407071906-6f0354cc6755 // aperture
	github.com/multiformats/go-multihash => github.com/aperturerobotics/go-multihash v0.2.3 // aperture
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.0.0-20200820224411-1e751ff168ab // indirect: used by bifrost-nats-server
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20221228081037-b7c2df0c151f // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/nats-io/nkeys => github.com/nats-io/nkeys v0.3.0 // indirect: used by bifrost-nats-server
	github.com/quic-go/quic-go => github.com/aperturerobotics/quic-go v0.48.2-0.20241029082227-fa76c393ee89 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
)

require (
	filippo.io/edwards25519 v1.1.1-0.20231210192602-a7dfd8e4e6b4 // indirect
	github.com/klauspost/compress v1.17.11
	github.com/libp2p/go-libp2p v0.37.0
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.14.0 // indirect
	github.com/nats-io/nats-server/v2 v2.10.22 // indirect
	github.com/nats-io/nats.go v1.37.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pion/datachannel v1.5.9 // indirect
	github.com/pion/sdp/v3 v3.0.9 // indirect
	github.com/pion/webrtc/v4 v4.0.2 // indirect
	github.com/pkg/errors v0.9.1
	github.com/quic-go/quic-go v0.48.1 // indirect
	github.com/sirupsen/logrus v1.9.3
	github.com/urfave/cli/v2 v2.27.5
	github.com/zeebo/blake3 v0.2.4
	golang.org/x/crypto v0.29.0
	golang.org/x/exp v0.0.0-20241108190413-2d47ceb2692f
	gonum.org/v1/gonum v0.15.1 // indirect
)

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/coder/websocket v1.8.13-0.20241107130731-1253b774ead7
)

require (
	github.com/Workiva/go-datastructures v1.1.5 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
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
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/nats-io/jwt/v2 v2.4.1 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/onsi/ginkgo/v2 v2.20.2 // indirect
	github.com/pion/dtls/v3 v3.0.4 // indirect
	github.com/pion/ice/v4 v4.0.2 // indirect
	github.com/pion/interceptor v0.1.37 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.14 // indirect
	github.com/pion/rtp v1.8.9 // indirect
	github.com/pion/sctp v1.8.33 // indirect
	github.com/pion/srtp/v3 v3.0.4 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	github.com/tidwall/btree v1.7.0 // indirect
	github.com/tylertreat/BoomFilters v0.0.0-20210315201527-1a82519a3e43 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/mock v0.5.0 // indirect
	golang.org/x/net v0.31.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)
