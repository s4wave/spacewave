module github.com/aperturerobotics/hydra

replace (
	github.com/libp2p/go-libp2p-crypto => github.com/paralin/go-libp2p-crypto v0.0.0-20181130162722-b150863d61f7
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.0-20180604152109-a0545ef43e32
)

require (
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20190702103440-19d394d4bb6a
	github.com/aperturerobotics/controllerbus v0.0.0-20190412141224-a86f75f58cec
	github.com/aperturerobotics/entitygraph v0.0.0-20190314052401-c4dff866fe8f
	github.com/aperturerobotics/timestamp v0.2.2-0.20190226083629-0175fc7d961e
	github.com/blang/semver v3.5.1+incompatible
	github.com/dgraph-io/badger v1.6.1-0.20190803064941-e627d49fa7e9
	github.com/gogo/protobuf v1.2.2-0.20190611061853-dadb62585089
	github.com/golang/protobuf v1.3.2-0.20190701182201-6c65a5562fc0
	github.com/golang/snappy v0.0.1
	github.com/gopherjs/gopherjs v0.0.0-20190430165422-3e4dfb77656c
	github.com/gxed/hashland/keccakpg v0.0.2-0.20190410183708-45ac3eb2d3ef // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/libp2p/go-libp2p-crypto v0.0.1
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v0.1.0 // indirect
	github.com/mr-tron/base58 v1.1.1
	github.com/paralin/go-indexeddb v0.0.0-20190222014559-731fb221041d
	github.com/pkg/errors v0.8.2-0.20190227000051-27936f6d90f9
	github.com/sirupsen/logrus v1.4.1
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/urfave/cli v1.20.1-0.20190203184040-693af58b4d51
	gonum.org/v1/gonum v0.0.0-20190520094443-a5f8f3a4840b
	google.golang.org/grpc v1.19.0
)
