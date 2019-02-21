module github.com/aperturerobotics/hydra

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.0-20180604152109-a0545ef43e32

replace github.com/libp2p/go-libp2p-crypto => github.com/paralin/go-libp2p-crypto v0.0.0-20181130162722-b150863d61f7

require (
	github.com/AndreasBriese/bbloom v0.0.0-20180913140656-343706a395b7 // indirect
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20190221021926-791287605946
	github.com/aperturerobotics/controllerbus v0.0.0-20190207110403-58d8957af709
	github.com/aperturerobotics/entitygraph v0.0.0-20190201112111-a07cf386595c
	github.com/aperturerobotics/timestamp v0.2.1
	github.com/blang/semver v3.5.2-0.20180723201105-3c1074078d32+incompatible
	github.com/dgraph-io/badger v1.5.5-0.20190214192501-3196cc1d7a5f
	github.com/dgryski/go-farm v0.0.0-20190104051053-3adb47b1fb0f // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/golang/protobuf v1.2.1-0.20190205222052-c823c79ea157
	github.com/golang/snappy v0.0.1
	github.com/gopherjs/gopherjs v0.0.0-20181103185306-d547d1d9531e
	github.com/libp2p/go-libp2p-crypto v2.0.5-0.20190218135128-e333f2201582+incompatible
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/mr-tron/base58 v1.1.1-0.20190103133359-fe73eb131202
	github.com/paralin/go-indexeddb v0.0.0-20181227124316-8931fda5ab36
	github.com/pkg/errors v0.8.2-0.20190217225212-856c240a51a2
	github.com/sirupsen/logrus v1.3.1-0.20190220172253-4f5fd631f164
	github.com/urfave/cli v1.20.1-0.20190203184040-693af58b4d51
	golang.org/x/net v0.0.0-20190213061140-3a22650c66bd
	gonum.org/v1/gonum v0.0.0-20190220170532-48e517cc5e35
	gonum.org/v1/netlib v0.0.0-20190219113230-9992c5f5eae4 // indirect
	google.golang.org/grpc v1.18.0
)
