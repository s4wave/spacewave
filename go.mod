module github.com/aperturerobotics/hydra

go 1.13

replace (
	github.com/dgraph-io/badger => github.com/dgraph-io/badger v1.6.1-0.20190924140636-a425b0eafac0
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.0-20190831070958-91cde46649b8 // gopherjs-compat
)

require (
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20191003225553-b63fbd05827a
	github.com/aperturerobotics/controllerbus v0.1.6-0.20191002033011-c0e6f06edefd
	github.com/aperturerobotics/entitygraph v0.1.2-0.20190927211258-1d6a1c008f98
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.1.1+incompatible
	github.com/dgraph-io/badger v1.6.1-0.20190924140636-a425b0eafac0
	github.com/gogo/protobuf v1.3.1-0.20190908201246-8a5ed79f6888
	github.com/golang/protobuf v1.3.3-0.20190920234318-1680a479a2cf
	github.com/golang/snappy v0.0.1
	github.com/gopherjs/gopherjs v0.0.0-20190915194858-d3ddacdb130f
	github.com/libp2p/go-libp2p-core v0.2.4-0.20190930170843-2f75277a8d7b
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.1.2
	github.com/paralin/go-indexeddb v0.0.0-20191004040522-e04f17767803
	github.com/paralin/kcp-go-lite v1.0.2-0.20190927004254-2be397fe467b // indirect
	github.com/pkg/errors v0.8.2-0.20190227000051-27936f6d90f9
	github.com/sirupsen/logrus v1.4.2
	github.com/urfave/cli v1.21.0
	gonum.org/v1/gonum v0.0.0-20190929233944-b20cf7805fc4
	google.golang.org/grpc v1.24.0
)
