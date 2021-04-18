module github.com/aperturerobotics/hydra

go 1.13

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/cayleygraph/cayley => github.com/cayleygraph/cayley v0.7.7-0.20200226001555-fac546436001 // master
	github.com/cayleygraph/quad => github.com/cayleygraph/quad v1.2.4 // master
	github.com/dolthub/go-mysql-server => github.com/dolthub/go-mysql-server v0.8.1-0.20210224010146-aea8175542fe // master
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20201108050212-99af7cca6ec8 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

// aperture: use aperture forks
replace (
	github.com/genjidb/genji => github.com/paralin/genji v0.11.1-0.20210411060343-af694b14af9e // ext-engines
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

require (
	github.com/Workiva/go-datastructures v1.0.52
	github.com/aperturerobotics/bifrost v0.0.0-20210328052221-f32ae99033c5
	github.com/aperturerobotics/controllerbus v0.8.1-0.20210228012617-50317d57e0e4
	github.com/aperturerobotics/entitygraph v0.1.3
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7-0.20200226001555-fac546436001
	github.com/cayleygraph/quad v1.2.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/dolthub/go-mysql-server v0.0.0-00010101000000-000000000000
	github.com/dolthub/vitess v0.0.0-20210218042642-d99ce5b7b004
	github.com/emirpasic/gods v1.12.0
	github.com/genjidb/genji v0.8.1-0.20201112071311-72319d2a2285
	github.com/go-git/go-billy/v5 v5.1.0
	github.com/go-git/go-git/v5 v5.3.1-0.20210416221548-9618dbb80cfb
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/golang/protobuf v1.4.2
	github.com/golang/snappy v0.0.3
	github.com/gomodule/redigo v1.8.4
	github.com/gopherjs/gopherjs v0.0.0-20210413103415-7d3cbed7d026
	github.com/hidal-go/hidalgo v0.0.0-20201109092204-05749a6d73df
	github.com/libp2p/go-libp2p-core v0.7.0
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.2.0
	github.com/paralin/go-indexeddb v0.0.0-20201108213622-b8aa4a40cb6e
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.4.1 // indirect
	github.com/restic/chunker v0.4.0
	github.com/sirupsen/logrus v1.7.0
	github.com/urfave/cli v1.22.4
	github.com/vmihailenco/msgpack/v5 v5.1.4
	go.etcd.io/bbolt v1.3.5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/tools v0.1.1-0.20210416224228-07295caad09c // indirect
	gonum.org/v1/gonum v0.8.1
	gonum.org/v1/netlib v0.0.0-20210302091547-ede94419cf37 // indirect
	google.golang.org/grpc v1.30.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gorm.io/gorm v1.21.8
)
