module github.com/aperturerobotics/identity

go 1.16

// The following is from the Bifrost go.mod

// aperture: use compatibility forks
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.23.1-0.20210907061838-0a0338bd72f0 // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => github.com/paralin/grpc-go v1.30.1-0.20210804030014-1587a7c16b66 // aperture
)

require (
	github.com/aperturerobotics/bifrost v0.0.0-20211017060149-ad3e712f8ed2
	github.com/aperturerobotics/controllerbus v0.8.7-0.20211017055653-c2791257a7c4
	github.com/golang/protobuf v1.5.2
	github.com/kr/text v0.2.0 // indirect
	github.com/libp2p/go-libp2p-core v0.9.0
	github.com/multiformats/go-multiaddr v0.3.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
