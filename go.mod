module github.com/aperturerobotics/auth

go 1.16

// Copied from hydra go.mod

// aperture: use forks for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.22.1-0.20210728081144-c7bd4637cac2 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
// google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/aperturerobotics/bifrost v0.0.0-20210822042239-1a9033e1747b
	github.com/aperturerobotics/controllerbus v0.8.4-0.20210729091933-eb89d362c5c2
	github.com/aperturerobotics/hydra v0.0.0-20210822104735-b8dfcc3fe62f // indirect
	github.com/aperturerobotics/identity v0.0.0-20210703095428-14d79497eb5b
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/evanw/esbuild v0.12.20 // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/protobuf v1.5.2
	github.com/gopherjs/gopherjs v0.0.0-20210821201017-0d7b41766e00 // indirect
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/keybase/go-triplesec v0.0.0-20200218020411-6687d79e9f55
	github.com/libp2p/go-libp2p-core v0.9.0
	github.com/manifoldco/promptui v0.8.0
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
)
