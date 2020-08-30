module github.com/aperturerobotics/auth

go 1.14

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20200823084006-3bf6fe7f6a79 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/aperturerobotics/bifrost v0.0.0-20200823084156-e28df1d443b1
	github.com/aperturerobotics/controllerbus v0.8.1-0.20200802060256-360612dc3698
	github.com/aperturerobotics/identity v0.0.0-20200830091547-f103519c71b5
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/golang/protobuf v1.4.2
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/keybase/go-triplesec v0.0.0-20200218020411-6687d79e9f55
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/manifoldco/promptui v0.7.0
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.4
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a
)
