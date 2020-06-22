module github.com/aperturerobotics/identity

go 1.14

// temporary pin to v1.3.5 (pre-google v2 changes)
replace github.com/golang/protobuf => github.com/golang/protobuf v1.3.5 // 1.3.5 - pre 1.4.x

require (
	github.com/aperturerobotics/bifrost v0.0.0-20200621002652-11d125a82fc0
	github.com/aperturerobotics/controllerbus v0.4.1
	github.com/golang/protobuf v1.4.2
	github.com/kr/text v0.2.0 // indirect
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	google.golang.org/protobuf v1.23.0
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)
