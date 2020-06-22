module github.com/aperturerobotics/auth

go 1.14

// temporary pin to v1.3.5 (pre-google v2 changes)
replace github.com/golang/protobuf => github.com/golang/protobuf v1.3.5 // 1.3.5 - pre 1.4.x

require (
	github.com/aperturerobotics/bifrost v0.0.0-20200621002652-11d125a82fc0
	github.com/aperturerobotics/controllerbus v0.4.1
	github.com/aperturerobotics/identity v0.0.0-20200622052711-225aa5eca742
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/golang/protobuf v1.4.2
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/keybase/go-triplesec v0.0.0-20200218020411-6687d79e9f55
	github.com/libp2p/go-libp2p-core v0.6.0
	github.com/manifoldco/promptui v0.7.0
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.5.1 // indirect
	github.com/urfave/cli v1.22.4
	golang.org/x/crypto v0.0.0-20200423211502-4bdfaf469ed5
	google.golang.org/protobuf v1.24.0
	k8s.io/code-generator v0.18.3
)
