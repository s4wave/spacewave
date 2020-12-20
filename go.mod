module github.com/aperturerobotics/hydra

go 1.13

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/cayleygraph/cayley => github.com/cayleygraph/cayley v0.7.7-0.20200226001555-fac546436001 // master
	github.com/cayleygraph/quad => github.com/cayleygraph/quad v1.2.4 // master
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20201108050212-99af7cca6ec8 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

// aperture: use aperture forks
replace (
	github.com/genjidb/genji => github.com/paralin/genji v0.9.1-0.20201204103415-1400dcdc7f9c // ext-engines-1
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

require (
	github.com/Workiva/go-datastructures v1.0.52
	github.com/aperturerobotics/bifrost v0.0.0-20201108000126-054dabcf6795
	github.com/aperturerobotics/controllerbus v0.8.1-0.20200802060256-360612dc3698
	github.com/aperturerobotics/entitygraph v0.1.3
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7-0.20200226001555-fac546436001
	github.com/cayleygraph/quad v1.2.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/emirpasic/gods v1.12.0
	github.com/genjidb/genji v0.9.0
	github.com/golang/protobuf v1.4.2
	github.com/golang/snappy v0.0.3-0.20201103224600-674baa8c7fc3
	github.com/gomodule/redigo v1.8.3-0.20201029100755-0b0ad3d61a93
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00
	github.com/hidal-go/hidalgo v0.0.0-20190814174001-42e03f3b5eaa
	github.com/libp2p/go-libp2p-core v0.7.0
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.2.0
	github.com/paralin/go-indexeddb v0.0.0-20201108212958-697f06b5b3c5
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/urfave/cli v1.22.4
	go.etcd.io/bbolt v1.3.5
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	gonum.org/v1/gonum v0.8.1
	google.golang.org/grpc v1.30.0
)
