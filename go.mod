module github.com/aperturerobotics/bldr

go 1.17

// JS / Browser-specific replacements

replace (
	github.com/dgraph-io/badger/v2 => github.com/paralin/badger/v2 v2.0.1-rc1.0.20200609030847-c83b793e278e // fix-32bit
	github.com/rgraphql/magellan => github.com/rgraphql/magellan v0.6.3-0.20200830093112-c88cc1f643b2 // master
	github.com/rgraphql/rgraphql => github.com/rgraphql/rgraphql v1.0.8 // master
)

// Copied from hydra go.mod

// aperture: use js-compat forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20210804025450-76a92a481ea5 // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.3.1-0.20210804011724-d84485be5d08 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/libp2p/go-libp2p-tls => github.com/paralin/go-libp2p-tls v0.1.4-0.20210728062949-a42c760a733f // js-compat
	github.com/marten-seemann/qtls-go1-16 => github.com/paralin/qtls-go1-16 v0.1.5-0.20210728071944-419a2c247411 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20210804024047-dc49ac2ea3b4 // gopherjs-compat
	github.com/sirupsen/logrus => github.com/paralin/logrus v1.8.2-0.20210804014116-ae269fb01c6c // gopherjs-compat
	google.golang.org/grpc => github.com/paralin/grpc-go v1.30.1-0.20210804030014-1587a7c16b66 // aperture
)

// aperture: use aperture forks
replace (
	github.com/bits-and-blooms/bitset => github.com/paralin/go-blooms-bitset v1.2.1-0.20210621003254-d10d8d6ab8b7 // aperture
	github.com/bits-and-blooms/bloom/v3 => github.com/paralin/go-bloom/v3 v3.0.2-0.20210621003511-7e4e43980591 // aperture
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.10.1-0.20210715210115-22d267bf1416 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20210611010940-f1489325f50b // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.12.1-0.20210715210024-97123bb291e7 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210605044355-486b076ae739 // ext-engines
)

// aperture: use forks for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.22.1-0.20210728081144-c7bd4637cac2 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
// google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/Microsoft/go-winio v0.5.0
	github.com/aperturerobotics/auth v0.0.0-20210703095553-e512075b9cbf
	github.com/aperturerobotics/bifrost v0.0.0-20210822042239-1a9033e1747b
	github.com/aperturerobotics/controllerbus v0.8.4-0.20210729091933-eb89d362c5c2
	github.com/aperturerobotics/hydra v0.0.0-20210822042441-ab150f0981cb
	github.com/blang/semver v3.5.1+incompatible
	github.com/evanw/esbuild v0.12.20
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.5.2
	github.com/gopherjs/gopherjs v0.0.0-20210821201017-0d7b41766e00
	github.com/manifoldco/promptui v0.8.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
)

require (
	github.com/Workiva/go-datastructures v1.0.53 // indirect
	github.com/aperturerobotics/entitygraph v0.1.4-0.20210530040557-f19da9c2be6d // indirect
	github.com/aperturerobotics/timestamp v0.2.4-0.20210530040952-1422410fbd4a // indirect
	github.com/btcsuite/btcd v0.20.1-beta // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/dgraph-io/badger/v2 v2.2007.2 // indirect
	github.com/dgraph-io/ristretto v0.0.2-0.20200115201040-8f368f2f2ab3 // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/emirpasic/gods v1.12.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/snappy v0.0.3 // indirect
	github.com/gomodule/redigo v1.8.4 // indirect
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/gopherjs/websocket v0.0.0-20191103002815-9a42957e2b3a // indirect
	github.com/gorilla/websocket v1.4.3-0.20210424162022-e8629af678b7 // indirect
	github.com/ipfs/go-cid v0.0.7 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a // indirect
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/keybase/go-triplesec v0.0.0-20200218020411-6687d79e9f55 // indirect
	github.com/libp2p/go-buffer-pool v0.0.2 // indirect
	github.com/libp2p/go-libp2p-core v0.9.0 // indirect
	github.com/libp2p/go-libp2p-tls v0.2.0 // indirect
	github.com/libp2p/go-openssl v0.0.7 // indirect
	github.com/lucas-clemente/quic-go v0.23.0 // indirect
	github.com/lunixbochs/vtclean v0.0.0-20180621232353-2d01aacdc34a // indirect
	github.com/marten-seemann/qtls-go1-15 v0.1.5 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.4 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.0-rc.1 // indirect
	github.com/mattn/go-colorable v0.0.9 // indirect
	github.com/mattn/go-isatty v0.0.4 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/minio/sha256-simd v0.1.1 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.0.3 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multiaddr v0.3.1 // indirect
	github.com/multiformats/go-multibase v0.0.3 // indirect
	github.com/multiformats/go-multihash v0.0.14 // indirect
	github.com/multiformats/go-varint v0.0.6 // indirect
	github.com/neelance/astrewrite v0.0.0-20160511093645-99348263ae86 // indirect
	github.com/neelance/sourcemap v0.0.0-20200213170602-2833bce08e4c // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.4 // indirect
	github.com/paralin/go-indexeddb v1.0.2-0.20210804030838-1a4bc20c4524 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/xtaci/smux v1.5.15 // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/net v0.0.0-20210805182204-aaa1db679c0d // indirect
	golang.org/x/sys v0.0.0-20210809222454-d867a43fc93e // indirect
	golang.org/x/tools v0.1.6-0.20210820212750-d4cc65f0b2ff // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gonum.org/v1/gonum v0.9.3 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	mvdan.cc/gofumpt v0.1.1-0.20210401090014-0952458e3d6b // indirect
)
