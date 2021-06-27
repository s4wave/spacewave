module github.com/aperturerobotics/identity

go 1.16

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20210518124640-25c39ec20d1d // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/aperturerobotics/bifrost v0.0.0-20210627002432-473d96043fa2
	github.com/aperturerobotics/controllerbus v0.8.2-0.20210604070940-5696853dc7ad
	github.com/golang/protobuf v1.4.2
	github.com/libp2p/go-libp2p-core v0.8.5
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
)
