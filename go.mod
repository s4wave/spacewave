module github.com/aperturerobotics/hydra

go 1.16

// aperture: use aperture forks
replace (
	github.com/bits-and-blooms/bitset => github.com/paralin/go-blooms-bitset v1.2.1-0.20210621003254-d10d8d6ab8b7 // aperture
	github.com/bits-and-blooms/bloom/v3 => github.com/paralin/go-bloom/v3 v3.0.2-0.20210621003511-7e4e43980591 // aperture
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.16-0.20210728072548-664b46444f01 // gopherjs-compat
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.10.1-0.20210907050511-cd581af7fb28 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20210907050252-057c3d88bdec // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.13.1-0.20210906212411-d9723e75eaa0 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210703095932-8592b046e48a // ext-engines
)

// aperture: use js-compat forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20211228221707-1d49e4ac116b // aperture
	// github.com/cayleygraph/quad => github.com/paralin/cayley-quad v1.2.5-0.20211209073857-a28a5348625f // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.4.3-0.20211116083949-5904ad760e00 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/marten-seemann/qtls-go1-16 => github.com/paralin/qtls-go1-16 v0.1.5-0.20210728071944-419a2c247411 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20210804024047-dc49ac2ea3b4 // gopherjs-compat
	github.com/sirupsen/logrus => github.com/paralin/logrus v1.8.2-0.20210804014116-ae269fb01c6c // gopherjs-compat
)

// Note: the below is from the Bifrost go.mod

// aperture: use compatibility forks
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/libp2p/go-libp2p-core => github.com/paralin/go-libp2p-core v0.12.1-0.20211209071220-3b91008fd2c4 // aperture
	github.com/libp2p/go-libp2p-tls => github.com/paralin/go-libp2p-tls v0.3.1-0.20211020072724-21716cf18549 // js-compat
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.23.1-0.20210907061838-0a0338bd72f0 // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => github.com/paralin/grpc-go v1.30.1-0.20210804030014-1587a7c16b66 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.27-0.20220104045627-466c7ca18e92 // aperture
)

require (
	bazil.org/fuse v0.0.0-20200524192727-fb710f7dfd05
	github.com/Workiva/go-datastructures v1.0.53
	github.com/aperturerobotics/bifrost v0.1.0
	github.com/aperturerobotics/controllerbus v0.8.7-0.20220120091329-a050987b33ee
	github.com/aperturerobotics/entitygraph v0.1.4-0.20210530040557-f19da9c2be6d
	github.com/aperturerobotics/timestamp v0.3.4
	github.com/bits-and-blooms/bitset v1.2.0
	github.com/bits-and-blooms/bloom/v3 v3.0.0-00010101000000-000000000000
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.0.0-00010101000000-000000000000
	github.com/cayleygraph/quad v1.2.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/dolthub/go-mysql-server v0.0.0-00010101000000-000000000000
	github.com/dolthub/vitess v0.0.0-20210823180838-e36a9ec06b90
	github.com/dustin/go-humanize v1.0.0
	github.com/emirpasic/gods v1.12.0
	github.com/genjidb/genji v0.0.0-00010101000000-000000000000
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.0.0-00010101000000-000000000000
	github.com/golang/protobuf v1.5.2
	github.com/gomodule/redigo v1.8.8
	github.com/hidal-go/hidalgo v0.0.0-20201109092204-05749a6d73df
	github.com/klauspost/compress v1.13.6
	github.com/libp2p/go-libp2p-core v0.12.0
	github.com/mr-tron/base58 v1.2.0
	github.com/paralin/go-indexeddb v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/restic/chunker v0.4.0
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	github.com/vmihailenco/msgpack/v5 v5.3.5
	github.com/zeebo/blake3 v0.2.1
	go.etcd.io/bbolt v1.3.6
	golang.org/x/crypto v0.0.0-20220112180741-5e0467b6c7ce
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gonum.org/v1/gonum v0.9.3
	google.golang.org/grpc v1.31.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gorm.io/gorm v1.22.5
	nhooyr.io/websocket v1.8.8-0.20210410000328-8dee580a7f74 // indirect
	storj.io/drpc v0.0.26
)
