module github.com/aperturerobotics/identity

go 1.14

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200706003739-05fb54d407a9 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20200706055849-42a34d166a60 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/aperturerobotics/bifrost v0.0.0-20200621002652-11d125a82fc0
	github.com/aperturerobotics/controllerbus v0.4.1
	github.com/golang/protobuf v1.4.2
	github.com/kr/text v0.2.0 // indirect
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)
