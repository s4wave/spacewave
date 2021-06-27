module github.com/aperturerobotics/auth

go 1.16

// aperture: use aperture forks
replace (
	github.com/ProtonMail/go-crypto => github.com/paralin/go-crypto v0.0.0-20210427232619-f5bd188194a5 // gopherjs-compat
	github.com/bits-and-blooms/bitset => github.com/paralin/go-blooms-bitset v1.2.1-0.20210621003254-d10d8d6ab8b7 // aperture
	github.com/bits-and-blooms/bloom/v3 => github.com/paralin/go-bloom/v3 v3.0.2-0.20210621003511-7e4e43980591 // aperture
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.10.1-0.20210611012401-1e51e5b03b66 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20210611010940-f1489325f50b // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.12.1-0.20210603025425-11ee02d7b08d // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210605044355-486b076ae739 // ext-engines
)

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
	github.com/aperturerobotics/identity v0.0.0-20210429032019-b45e360ea44b
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/keybase/go-triplesec v0.0.0-20200218020411-6687d79e9f55
	github.com/kr/text v0.2.0 // indirect
	github.com/libp2p/go-libp2p-core v0.8.5
	github.com/manifoldco/promptui v0.8.0
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e // indirect
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
