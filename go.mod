module github.com/aperturerobotics/hydra

go 1.16

require (
	github.com/aperturerobotics/bifrost v0.1.2-0.20220321132941-6d0326ef645d
	github.com/aperturerobotics/controllerbus v0.9.0
	github.com/aperturerobotics/entitygraph v0.2.0
	github.com/aperturerobotics/timestamp v0.4.0
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.11.1-0.20220315071359-d18204a140a5 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20220315035103-ee808c4b8def // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.13.1-0.20210906212411-d9723e75eaa0 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210703095932-8592b046e48a // ext-engines
)

// aperture: use compatibility forks
replace (
	github.com/bits-and-blooms/bloom/v3 => github.com/paralin/go-bloom/v3 v3.1.1-0.20220321113354-ddfde510cc94 // aperture
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20220321114736-873b5e61a63c // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.4.3-0.20211116083949-5904ad760e00 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.16-0.20210728072548-664b46444f01 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20210804024047-dc49ac2ea3b4 // gopherjs-compat
)

// Note: the below is from the Bifrost go.mod

// aperture: use compatibility forks
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/libp2p/go-libp2p-core => github.com/paralin/go-libp2p-core v0.14.1-0.20220321111733-8010b7b24680 // aperture
	github.com/libp2p/go-libp2p-tls => github.com/paralin/go-libp2p-tls v0.3.2-0.20220321112951-db0cab39ed18 // js-compat
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.23.1-0.20220321112440-8295926e98d6 // aperture
	github.com/marten-seemann/qtls-go1-16 => github.com/paralin/qtls-go1-16 v0.1.5-0.20210728071944-419a2c247411 // gopherjs-compat
	github.com/marten-seemann/qtls-go1-17 => github.com/paralin/qtls-go1-17 v0.1.1-0.20220321132518-c12ea7282574 // gopherjs-compat
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	github.com/sirupsen/logrus => github.com/paralin/logrus v1.8.2-0.20210804014116-ae269fb01c6c // gopherjs-compat
	github.com/zeebo/blake3 => github.com/paralin/go-blake3 v0.2.3-0.20220321123929-a1d1fabeda71 // js-compat
	golang.org/x/crypto => github.com/aperturerobotics/golang-x-crypto v0.0.0-20220321111526-87c0d0398f72 // gopherjs-compat
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.30-0.20220301023015-b1e9d6bd9478 // aperture
)

require (
	bazil.org/fuse v0.0.0-20200524192727-fb710f7dfd05
	github.com/Workiva/go-datastructures v1.0.53
	github.com/bits-and-blooms/bitset v1.2.2
	github.com/bits-and-blooms/bloom/v3 v3.0.0-00010101000000-000000000000
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.0.0-00010101000000-000000000000
	github.com/cayleygraph/quad v1.2.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/dolthub/go-mysql-server v0.0.0-00010101000000-000000000000
	github.com/dolthub/vitess v0.0.0-20220310224229-3e7f4e04f4a5
	github.com/dustin/go-humanize v1.0.0
	github.com/emirpasic/gods v1.12.0
	github.com/genjidb/genji v0.0.0-00010101000000-000000000000
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.0.0-00010101000000-000000000000
	github.com/golang/protobuf v1.5.2
	github.com/gomodule/redigo v1.8.8
	github.com/hidal-go/hidalgo v0.0.0-20190814174001-42e03f3b5eaa
	github.com/klauspost/compress v1.15.1
	github.com/libp2p/go-libp2p-core v0.14.0
	github.com/mr-tron/base58 v1.2.0
	github.com/paralin/go-indexeddb v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/restic/chunker v0.4.0
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	github.com/vmihailenco/msgpack/v5 v5.3.5
	github.com/zeebo/blake3 v0.2.2
	go.etcd.io/bbolt v1.3.6
	golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gonum.org/v1/gonum v0.11.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gorm.io/gorm v1.23.3
	storj.io/drpc v0.0.30
)
