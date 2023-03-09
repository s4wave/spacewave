module github.com/aperturerobotics/bldr

go 1.19

require github.com/aperturerobotics/hydra v0.0.0-20230306075056-a2ed7c0deaf9 // master

// Note: the below is from the Hydra go.mod

require github.com/aperturerobotics/bifrost v0.12.2-0.20230308003930-6bfd9afa4fb8 // master

// cayley has not been updated to support v0.2.0
require github.com/hidal-go/hidalgo v0.0.0-20190814174001-42e03f3b5eaa // indirect

// aperture: use ext-engines forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20220321114736-873b5e61a63c // aperture
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.14.1-0.20230303011014-cc0ae7f6c49e // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20230303010909-96180325b868 // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.14.1-0.20230213145718-23097a679f40 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.0.0-20200820224411-1e751ff168ab
)

// aperture: use compatibility forks
replace (
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.4.3-0.20211116083949-5904ad760e00 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.2.0 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.12.2-0.20220323132038-01665499027f // aperture
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.24.0 // master
	github.com/aperturerobotics/entitygraph v0.3.3 // indirect
	github.com/aperturerobotics/starpc v0.18.2
)

// aperture: use compatibility forks
replace (
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20221228081037-b7c2df0c151f // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	github.com/quic-go/quic-go => github.com/aperturerobotics/quic-go v0.32.1-0.20230202115409-62a93ecbe333 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.1-0.20221224130652-ff61cbb763af // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.28.2-0.20230301012226-7fb3cdbd9197 // aperture
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.31-0.20220527065730-0e2a1370bccb // aperture
)

require (
	github.com/Microsoft/go-winio v0.5.0
	github.com/aperturerobotics/timestamp v0.6.5-0.20230301015330-4cf6f8e67e89 // master
	github.com/aperturerobotics/util v1.0.6-0.20230301000649-0187d548293f // master
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7-0.20221003143241-94f1b4905386
	github.com/cayleygraph/quad v1.2.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/evanw/esbuild v0.17.8
	github.com/fatih/color v1.12.0
	github.com/fsnotify/fsnotify v1.6.0
	github.com/ghodss/yaml v1.0.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/libp2p/go-libp2p v0.26.2
	github.com/pkg/errors v0.9.1
	github.com/sergi/go-diff v1.3.1
	github.com/sirupsen/logrus v1.9.0
	github.com/urfave/cli/v2 v2.25.0
	github.com/zeebo/blake3 v0.2.3
	golang.org/x/exp v0.0.0-20230127193734-31bee513bff7
	golang.org/x/mod v0.8.0
	golang.org/x/sync v0.1.0
	golang.org/x/tools v0.6.0
	google.golang.org/protobuf v1.28.1
	nhooyr.io/websocket v1.8.8-0.20221213223501-14fb98eba64e
)

require github.com/mr-tron/base58 v1.2.0

require (
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/Workiva/go-datastructures v1.0.53 // indirect
	github.com/aperturerobotics/ts-proto-common-types v0.2.0 // indirect
	github.com/bits-and-blooms/bitset v1.4.0 // indirect
	github.com/bits-and-blooms/bloom/v3 v3.3.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/dennwc/base v1.0.0 // indirect
	github.com/dgraph-io/badger/v2 v2.2007.4 // indirect
	github.com/dgraph-io/ristretto v0.0.3-0.20200630154024-f66de99634de // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/djherbis/buffer v1.2.0 // indirect
	github.com/dolthub/go-mysql-server v0.14.1-0.20230218000648-8448267c2200 // indirect
	github.com/dolthub/vitess v0.0.0-20210823180838-e36a9ec06b90 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/go-billy/v5 v5.4.0 // indirect
	github.com/go-kit/kit v0.12.0 // indirect
	github.com/go-sql-driver/mysql v1.6.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gobuffalo/envy v1.7.1 // indirect
	github.com/gobuffalo/logger v1.0.1 // indirect
	github.com/gobuffalo/packd v0.3.0 // indirect
	github.com/gobuffalo/packr/v2 v2.7.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gomodule/redigo v1.8.9 // indirect
	github.com/google/flatbuffers v2.0.6+incompatible // indirect
	github.com/google/pprof v0.0.0-20221203041831-ce31453925ec // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ipfs/go-cid v0.3.2 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.4 // indirect
	github.com/joho/godotenv v1.3.0 // indirect
	github.com/klauspost/compress v1.16.0 // indirect
	github.com/klauspost/cpuid v1.2.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.1 // indirect
	github.com/klauspost/reedsolomon v1.9.2 // indirect
	github.com/lestrrat-go/strftime v1.0.4 // indirect
	github.com/libp2p/go-buffer-pool v0.1.1-0.20220919134021-a29bd39bcbb7 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.1-0.20220919134236-1c09f2ab3ec1 // indirect
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.8.0 // indirect
	github.com/multiformats/go-multibase v0.1.2-0.20220823162309-7160a7347ed1 // indirect
	github.com/multiformats/go-multicodec v0.7.1-0.20221017174837-a2baec7ca709 // indirect
	github.com/multiformats/go-multihash v0.2.2-0.20221030163302-608669da49b6 // indirect
	github.com/multiformats/go-multistream v0.4.1 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/nats-io/jwt/v2 v2.0.3 // indirect
	github.com/nats-io/nats-server/v2 v2.7.4 // indirect
	github.com/nats-io/nats.go v1.13.0 // indirect
	github.com/nats-io/nkeys v0.3.0 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852 // indirect
	github.com/onsi/ginkgo/v2 v2.5.1 // indirect
	github.com/paralin/go-indexeddb v1.0.1 // indirect
	github.com/paralin/kcp-go-lite v4.3.4+incompatible // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pauleyj/gobee v0.0.0-20190212035730-6270c53072a4 // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/planetscale/vtprotobuf v0.4.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/quic-go/qtls-go1-18 v0.2.0 // indirect
	github.com/quic-go/qtls-go1-19 v0.2.1 // indirect
	github.com/quic-go/qtls-go1-20 v0.1.1 // indirect
	github.com/quic-go/quic-go v0.33.0 // indirect
	github.com/restic/chunker v0.4.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/spf13/cobra v1.4.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07 // indirect
	github.com/templexxx/cpu v0.0.1 // indirect
	github.com/templexxx/cpufeat v0.0.0-20180724012125-cef66df7f161 // indirect
	github.com/templexxx/xor v0.0.0-20191217153810-f85b25db303b // indirect
	github.com/templexxx/xorsimd v0.4.1 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tylertreat/BoomFilters v0.0.0-20181028192813-611b3dbe80e8 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/xtaci/smux v1.5.21 // indirect
	github.com/zeebo/errs v1.2.2 // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.opentelemetry.io/otel v1.7.0 // indirect
	go.opentelemetry.io/otel/trace v1.7.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.1.1-0.20221020023724-80b9fac54d29 // indirect
	gonum.org/v1/gonum v0.12.0 // indirect
	google.golang.org/genproto v0.0.0-20220414192740-2d67ff6cf2b4 // indirect
	google.golang.org/grpc v1.45.0 // indirect
	gopkg.in/src-d/go-errors.v1 v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gorm.io/gorm v1.24.3 // indirect
	lukechampine.com/blake3 v1.1.8-0.20220321170924-7afca5966e5e // indirect
	storj.io/drpc v0.0.30 // indirect
)
