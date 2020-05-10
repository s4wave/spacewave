module github.com/aperturerobotics/hydra

go 1.13

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20191223064048-69f11664fe90 // gopherjs-compat

require (
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20200408014048-203ffb6567b4
	github.com/aperturerobotics/controllerbus v0.3.0
	github.com/aperturerobotics/entitygraph v0.1.2
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.0.1-rc1
	github.com/golang/protobuf v1.3.5
	github.com/golang/snappy v0.0.2-0.20190904063534-ff6b7dc882cf
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00
	github.com/hidal-go/hidalgo v0.0.0-20190814174001-42e03f3b5eaa
	github.com/libp2p/go-libp2p-core v0.3.1-0.20191214080825-6f2516674ace
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.1.3
	github.com/paralin/go-indexeddb v0.0.0-20191012003246-aae1d9757c46
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/urfave/cli v1.22.4
	go.etcd.io/bbolt v1.3.4-0.20191128235701-0b7b41e21b57
	gonum.org/v1/gonum v0.6.1-0.20191215081219-55b691b5812b
	google.golang.org/grpc v1.28.0
)
