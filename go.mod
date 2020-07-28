module github.com/aperturerobotics/hydra

go 1.13

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200706003739-05fb54d407a9 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20200728021714-7db2bdfa8cd7 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/Workiva/go-datastructures v1.0.52
	github.com/aperturerobotics/bifrost v0.0.0-20200728210142-84d0f733c452
	github.com/aperturerobotics/controllerbus v0.7.1-0.20200728205218-566a71985221
	github.com/aperturerobotics/entitygraph v0.1.3
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.0.1-rc1.0.20200724140651-d8e8324d4556
	github.com/golang/protobuf v1.4.2
	github.com/golang/snappy v0.0.2-0.20200707131729-196ae77b8a26
	github.com/gomodule/redigo v1.8.2
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00
	github.com/hidal-go/hidalgo v0.0.0-20190814174001-42e03f3b5eaa
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.2.0
	github.com/paralin/go-indexeddb v0.0.0-20191012003246-aae1d9757c46
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.4
	go.etcd.io/bbolt v1.3.2
	gonum.org/v1/gonum v0.7.0
	google.golang.org/grpc v1.30.0
)
