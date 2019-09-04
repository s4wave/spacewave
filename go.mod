module github.com/aperturerobotics/hydra

go 1.13

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.0-20190831070958-91cde46649b8 // gopherjs-compat

require (
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20190923005326-385133567580
	github.com/aperturerobotics/controllerbus v0.1.5-0.20190922225907-1c6cc20b88b2
	github.com/aperturerobotics/entitygraph v0.1.1-0.20190909222015-b58513aa9083
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.1.1+incompatible
	github.com/dgraph-io/badger v1.6.1-0.20190810110519-74ed6da2c776
	github.com/gogo/protobuf v1.3.1-0.20190908201246-8a5ed79f6888
	github.com/golang/protobuf v1.3.3-0.20190827175835-822fe56949f5
	github.com/golang/snappy v0.0.1
	github.com/gopherjs/gopherjs v0.0.0-20190915194858-d3ddacdb130f
	github.com/libp2p/go-libp2p-core v0.2.3-0.20190828160545-b74f60b9cc2b
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.1.2
	github.com/paralin/go-indexeddb v0.0.0-20190222014559-731fb221041d
	github.com/paralin/kcp-go-lite v4.3.2-0.20190202132049-1e12d0a0fd45+incompatible // indirect
	github.com/pkg/errors v0.8.2-0.20190227000051-27936f6d90f9
	github.com/sirupsen/logrus v1.4.1
	github.com/urfave/cli v1.21.1-0.20190830145355-3eca1090a37a
	gonum.org/v1/gonum v0.0.0-20190808205415-ced62fe5104b
	google.golang.org/grpc v1.23.0
)
