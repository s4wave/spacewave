module github.com/aperturerobotics/hydra

require (
	github.com/AndreasBriese/bbloom v0.0.0-20180913140656-343706a395b7 // indirect
	github.com/Workiva/go-datastructures v1.0.50
	github.com/aperturerobotics/bifrost v0.0.0-20190115160833-22ae457ac1f2
	github.com/aperturerobotics/controllerbus v0.0.0-20190108033723-2cf5f56f7860
	github.com/aperturerobotics/entitygraph v0.0.0-20181226225716-1e77d0ca8bd7
	github.com/aperturerobotics/timestamp v0.2.1
	github.com/blang/semver v3.5.2-0.20180723201105-3c1074078d32+incompatible
	github.com/dgraph-io/badger v1.5.5-0.20190109015002-b85f5ae73a55
	github.com/dgryski/go-farm v0.0.0-20190104051053-3adb47b1fb0f // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/golang/protobuf v1.2.1-0.20190109072247-347cf4a86c1c
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db
	github.com/gopherjs/gopherjs v0.0.0-20181103185306-d547d1d9531e
	github.com/libp2p/go-libp2p-crypto v2.0.1+incompatible
	github.com/mr-tron/base58 v1.1.1-0.20190103133359-fe73eb131202
	github.com/paralin/go-indexeddb v0.0.0-20181227124316-8931fda5ab36
	github.com/pkg/errors v0.8.2-0.20190109061628-ffb6e22f0193
	github.com/sirupsen/logrus v1.3.0
	github.com/urfave/cli v1.20.1-0.20181029213200-b67dcf995b6a
	golang.org/x/net v0.0.0-20190110044637-be1c187aa6c6
	gonum.org/v1/gonum v0.0.0-20190105094335-1fc0fba783fc
	gonum.org/v1/netlib v0.0.0-20181224185128-3431cf544c75 // indirect
	google.golang.org/grpc v1.17.0
)

replace github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.0-20190110102829-0484db56787c

replace github.com/libp2p/go-libp2p-crypto => github.com/paralin/go-libp2p-crypto v0.0.0-20190110112134-4f99fef99f04
