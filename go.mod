module github.com/aperturerobotics/forge

go 1.16

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/cayleygraph/cayley => github.com/cayleygraph/cayley v0.7.7-0.20200226001555-fac546436001 // master
	github.com/cayleygraph/quad => github.com/cayleygraph/quad v1.2.4 // master
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.9.1-0.20210423125124-7df8212d500b // fixes
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.20.1-0.20210422032919-e6160120d238 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

// aperture: use aperture forks
replace (
	github.com/ProtonMail/go-crypto => github.com/paralin/go-crypto v0.0.0-20210419035808-7676e9e7b35c // gopherjs-compat
	github.com/genjidb/genji => github.com/paralin/genji v0.11.1-0.20210411060343-af694b14af9e // ext-engines
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/aperturerobotics/bifrost v0.0.0-20210515211241-9a00df9e3a47
	github.com/aperturerobotics/controllerbus v0.8.1-0.20210503093825-eb22ea57dce4
	github.com/aperturerobotics/entitygraph v0.1.3
	github.com/aperturerobotics/hydra v0.0.0-20210511230908-39f291064c82
	github.com/blang/semver v3.5.1+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/golang/protobuf v1.5.2
	github.com/libp2p/go-libp2p-core v0.8.6-0.20210415043615-525a0b130172
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	github.com/valyala/fastjson v1.6.4-0.20210112210304-6dae91c8e11a
	google.golang.org/grpc v1.30.0
)
