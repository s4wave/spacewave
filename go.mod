module github.com/aperturerobotics/hydra

replace (
	github.com/libp2p/go-libp2p-crypto => github.com/paralin/go-libp2p-crypto v0.0.0-20181130162722-b150863d61f7
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.0-20180604152109-a0545ef43e32
)

require (
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20190831064502-6e674ba7f6c0
	github.com/aperturerobotics/controllerbus v0.0.0-20190820025710-22efcef818fb
	github.com/aperturerobotics/entitygraph v0.0.0-20190314052401-c4dff866fe8f
	github.com/aperturerobotics/timestamp v0.2.2-0.20190226083629-0175fc7d961e
	github.com/blang/semver v3.5.1+incompatible
	github.com/dgraph-io/badger v1.6.1-0.20190810110519-74ed6da2c776
	github.com/gogo/protobuf v1.2.2-0.20190730201129-28a6bbf47e48
	github.com/golang/protobuf v1.3.3-0.20190805180045-4c88cc3f1a34
	github.com/golang/snappy v0.0.1
	github.com/gopherjs/gopherjs v0.0.0-20190812055157-5d271430af9f
	github.com/gxed/hashland/keccakpg v0.0.2-0.20190410183708-45ac3eb2d3ef // indirect
	github.com/libp2p/go-libp2p-crypto v0.0.1
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v0.1.0 // indirect
	github.com/mr-tron/base58 v1.1.1
	github.com/paralin/go-indexeddb v0.0.0-20190222014559-731fb221041d
	github.com/pkg/errors v0.8.2-0.20190227000051-27936f6d90f9
	github.com/sirupsen/logrus v1.4.1
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/urfave/cli v1.21.1-0.20190812203037-6cc7e987c4fa
	gonum.org/v1/gonum v0.0.0-20190808205415-ced62fe5104b
	google.golang.org/grpc v1.23.0
)
