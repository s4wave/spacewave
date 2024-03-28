module github.com/aperturerobotics/bldr

go 1.22

// https://github.com/evanw/esbuild/pull/3413 [rejected]
replace github.com/evanw/esbuild => github.com/aperturerobotics/esbuild v0.20.3-0.20240314215540-27d23cbf9cd6 // aperture

// https://github.com/golang/tools/pull/483
// https://go-review.googlesource.com/c/tools/+/570796
replace golang.org/x/tools => github.com/paralin/go-tools v0.19.1-0.20240314214115-ea4efd199cbf // avoid-cancel-log-spam

require github.com/aperturerobotics/hydra v0.0.0-20240328052204-38069db03310 // master

require (
	github.com/evanw/esbuild v0.20.0 // latest
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kolesnikovae/go-winjob v1.0.1-0.20200702113133-049537be0656 // master
	github.com/tetratelabs/wazero v1.6.0 // latest
)

// Note: the below is from the Hydra go.mod

require github.com/aperturerobotics/bifrost v0.25.4-0.20240326075319-f485a4156a47 // master

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.17.1-0.20231111110359-6e4ac609e0d7 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20231111105834-ccf9c4261495 // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.14.1-0.20230213145718-23097a679f40 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
	xorm.io/xorm => github.com/paralin/go-xorm v1.3.3-0.20230216084813-0cd923e7ced6 // ext-engines
)

// aperture: use compatibility forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20230526013106-bcbeda7f50f0 // aperture
	github.com/cayleygraph/quad => github.com/aperturerobotics/cayley-quad v1.2.5-0.20230524232228-dc08772d0195 // aperture
	// https://github.com/dgraph-io/badger/pull/2048
	github.com/dgraph-io/badger/v4 => github.com/paralin/badger/v4 v4.2.1-0.20240221040732-078580f8a58a // fix-wasm
	// https://github.com/dgraph-io/ristretto/pull/375
	github.com/dgraph-io/ristretto => github.com/paralin/ristretto v0.1.2-0.20240221033725-e9838e36e9d8 // fix-wasm
	github.com/go-git/go-billy/v5 => github.com/paralin/go-billy/v5 v5.5.1-0.20240225054224-3fbf975de769 // js
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.6.2-0.20230322095819-b641fd8f849b // gopherjs-compat
	github.com/hidal-go/hidalgo => github.com/aperturerobotics/hidalgo v0.3.1-0.20231111025334-8015549a1b51 // aperture
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.2.0 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.12.2-0.20220323132038-01665499027f // aperture
	go.bug.st/serial => github.com/paralin/go-serial v0.0.0-20240301222658-42bc112d18a5 // fix-wasm
)

require (
	github.com/cayleygraph/cayley v0.7.7
	github.com/dolthub/go-mysql-server v0.18.0 // indirect
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.36.7 // latest
	github.com/aperturerobotics/entitygraph v0.7.0 // indirect
	github.com/aperturerobotics/starpc v0.27.3 // latest
)

// aperture: use compatibility forks
replace (
	github.com/multiformats/go-multiaddr => github.com/paralin/go-multiaddr v0.10.2-0.20230807174004-e1767541c061 // aperture
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.0.0-20200820224411-1e751ff168ab // indirect: used by bifrost-nats-server
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20221228081037-b7c2df0c151f // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/nats-io/nkeys => github.com/nats-io/nkeys v0.3.0 // indirect: used by bifrost-nats-server
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20240110212540-c015327ec623 // aperture
	github.com/quic-go/quic-go => github.com/aperturerobotics/quic-go v0.41.1-0.20240125035303-1093432c45e9 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.33.1-0.20240322235918-b46c9358eab6 // aperture
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.31-0.20220527065730-0e2a1370bccb // aperture
)

require (
	github.com/aperturerobotics/ts-proto-common-types v0.20.2 // indirect; latest
	github.com/aperturerobotics/util v1.15.3-0.20240320183303-649409455fa5 // master
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/djherbis/buffer v1.2.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/compress v1.17.7
	github.com/libp2p/go-libp2p v0.33.1
	github.com/libp2p/go-yamux/v4 v4.0.2-0.20240322071716-53ef5820bd48 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.12.2 // indirect
	github.com/nats-io/nats-server/v2 v2.10.12 // indirect
	github.com/nats-io/nats.go v1.34.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/paralin/kcp-go-lite v5.4.20+incompatible // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/errors v0.9.1
	github.com/planetscale/vtprotobuf v0.6.0
	github.com/quic-go/quic-go v0.42.0 // indirect
	github.com/sirupsen/logrus v1.9.3
	github.com/templexxx/xor v0.0.0-20191217153810-f85b25db303b // indirect
	github.com/urfave/cli/v2 v2.27.1
	github.com/zeebo/blake3 v0.2.3
	golang.org/x/crypto v0.21.0 // indirect
	gonum.org/v1/gonum v0.15.0 // indirect
	google.golang.org/protobuf v1.33.0
	nhooyr.io/websocket v1.8.10
	storj.io/drpc v0.0.34 // indirect
)

require (
	github.com/Microsoft/go-winio v0.6.1
	github.com/aperturerobotics/go-kvfile v0.4.1
	github.com/aperturerobotics/timestamp v0.8.3-0.20240319022017-2c198e26587a
	github.com/cayleygraph/quad v1.2.5
	github.com/fatih/color v1.16.0
	github.com/fsnotify/fsnotify v1.7.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-billy/v5 v5.5.1-0.20231211202411-9f60ea941235
	github.com/sergi/go-diff v1.3.1
	golang.org/x/exp v0.0.0-20240325151524-a685a6edb6d8
	golang.org/x/mod v0.16.0
	golang.org/x/sync v0.6.0
	golang.org/x/tools v0.19.0
)

require (
	filippo.io/edwards25519 v1.1.1-0.20231210192602-a7dfd8e4e6b4 // indirect
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/Workiva/go-datastructures v1.1.3 // indirect
	github.com/bits-and-blooms/bitset v1.13.0 // indirect
	github.com/bits-and-blooms/bloom/v3 v3.7.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/creack/goselect v0.1.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/dgraph-io/badger/v4 v4.2.0 // indirect
	github.com/dgraph-io/ristretto v0.1.2-0.20240116140435-c67e07994f91 // indirect
	github.com/dolthub/flatbuffers/v23 v23.3.3-dh.2 // indirect
	github.com/dolthub/go-icu-regex v0.0.0-20230524105445-af7e7991c97e // indirect
	github.com/dolthub/jsonpath v0.0.2-0.20230525180605-8dc13778fd72 // indirect
	github.com/dolthub/vitess v0.0.0-20231109003730-c0fa018b5ef6 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-kit/kit v0.12.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gobuffalo/logger v1.0.6 // indirect
	github.com/gobuffalo/packd v1.0.1 // indirect
	github.com/gobuffalo/packr/v2 v2.8.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gomodule/redigo v1.9.2 // indirect
	github.com/google/flatbuffers v1.12.1 // indirect
	github.com/google/pprof v0.0.0-20240207164012-fb44976bdcd5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hidal-go/hidalgo v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.16.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/klauspost/reedsolomon v1.12.0 // indirect
	github.com/lestrrat-go/strftime v1.0.4 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/markbates/errx v1.1.0 // indirect
	github.com/markbates/oncer v1.0.0 // indirect
	github.com/markbates/safe v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/minio-go/v7 v7.0.69 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multicodec v0.9.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/nats-io/jwt/v2 v2.4.1 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/onsi/ginkgo/v2 v2.15.0 // indirect
	github.com/paralin/go-indexeddb v1.1.0 // indirect
	github.com/pauleyj/gobee v0.0.0-20190212035730-6270c53072a4 // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.10 // indirect
	github.com/pion/ice/v3 v3.0.3 // indirect
	github.com/pion/interceptor v0.1.27 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.12 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.14 // indirect
	github.com/pion/rtp v1.8.4 // indirect
	github.com/pion/sctp v1.8.13 // indirect
	github.com/pion/sdp/v3 v3.0.8 // indirect
	github.com/pion/srtp/v3 v3.0.1 // indirect
	github.com/pion/stun/v2 v2.0.0 // indirect
	github.com/pion/transport/v2 v2.2.4 // indirect
	github.com/pion/transport/v3 v3.0.1 // indirect
	github.com/pion/turn/v3 v3.0.1 // indirect
	github.com/pion/webrtc/v4 v4.0.0-beta.14 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/restic/chunker v0.4.0 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/rs/xid v1.5.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/templexxx/cpu v0.1.0 // indirect
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/xorsimd v0.4.2 // indirect
	github.com/tylertreat/BoomFilters v0.0.0-20210315201527-1a82519a3e43 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/xtaci/smux v1.5.24 // indirect
	github.com/zeebo/errs v1.2.2 // indirect
	go.bug.st/serial v1.6.2 // indirect
	go.etcd.io/bbolt v1.3.9 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel v1.7.0 // indirect
	go.opentelemetry.io/otel/trace v1.7.0 // indirect
	go.uber.org/mock v0.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/net v0.22.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/term v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231120223509-83a465c0220f // indirect
	google.golang.org/grpc v1.59.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/src-d/go-errors.v1 v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/gorm v1.25.8 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)
