module github.com/aperturerobotics/auth

go 1.18

require (
	github.com/aperturerobotics/hydra v0.0.0-20221224112816-d84fce724240 // indirect
	github.com/aperturerobotics/identity v0.0.0-20221224123704-e6d769b9505b
	github.com/keybase/go-triplesec v0.0.0-20221007201145-80ca9ce14c42
)

// Note: The below is from the Hydra go.mod

require github.com/aperturerobotics/bifrost v0.8.6-0.20221224094942-1209b6c2a855

// aperture: use ext-engines forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20220321114736-873b5e61a63c // aperture
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.12.1-0.20220917024939-dae88366f41d // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20220917020045-fdd5ada8e314 // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.13.1-0.20210906212411-d9723e75eaa0 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210703095932-8592b046e48a // ext-engines
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.0.0-20200820224411-1e751ff168ab
)

// aperture: use compatibility forks
replace (
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.4.3-0.20211116083949-5904ad760e00 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.16-0.20210728072548-664b46444f01 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20220323132038-01665499027f // aperture
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.20.4-0.20221224094810-f5a1b056d715
	github.com/aperturerobotics/entitygraph v0.3.2 // indirect
	github.com/aperturerobotics/starpc v0.16.0 // indirect
)

// aperture: use compatibility forks
replace (
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.28.2-0.20221208065852-9e95c24f3823 // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.8.2-0.20220322010420-77ab346a2cf8 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.28.2-0.20221202092004-7e5a6a8cf680 // aperture
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.31-0.20220527065730-0e2a1370bccb // aperture
)

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/libp2p/go-libp2p v0.24.1
	github.com/manifoldco/promptui v0.9.0
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.9.0
	github.com/urfave/cli/v2 v2.23.5
	golang.org/x/crypto v0.4.1-0.20221212223212-23edec0b383a
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/aperturerobotics/timestamp v0.6.0 // indirect
	github.com/aperturerobotics/util v0.0.0-20221219050225-3081033d019b // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/chzyer/readline v1.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/pprof v0.0.0-20221203041831-ce31453925ec // indirect
	github.com/ipfs/go-cid v0.3.2 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/klauspost/compress v1.15.12 // indirect
	github.com/klauspost/cpuid/v2 v2.2.1 // indirect
	github.com/libp2p/go-buffer-pool v0.1.1-0.20220919134021-a29bd39bcbb7 // indirect
	github.com/libp2p/go-openssl v0.1.1-0.20220921181522-00b60808a1ac // indirect
	github.com/libp2p/go-yamux/v4 v4.0.1-0.20220919134236-1c09f2ab3ec1 // indirect
	github.com/lucas-clemente/quic-go v0.31.1 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.3 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.1 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.8.0 // indirect
	github.com/multiformats/go-multibase v0.1.2-0.20220823162309-7160a7347ed1 // indirect
	github.com/multiformats/go-multicodec v0.7.1-0.20221017174837-a2baec7ca709 // indirect
	github.com/multiformats/go-multihash v0.2.2-0.20221030163302-608669da49b6 // indirect
	github.com/multiformats/go-multistream v0.3.3 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/onsi/ginkgo/v2 v2.5.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/zeebo/blake3 v0.2.3 // indirect
	github.com/zeebo/errs v1.2.2 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/exp v0.0.0-20221205204356-47842c84f3db // indirect
	golang.org/x/mod v0.7.0 // indirect
	golang.org/x/net v0.3.0 // indirect
	golang.org/x/sys v0.3.0 // indirect
	golang.org/x/tools v0.3.1-0.20221201230950-47a82463d369 // indirect
	gonum.org/v1/gonum v0.12.0 // indirect
	nhooyr.io/websocket v1.8.8-0.20221213223501-14fb98eba64e // indirect
	storj.io/drpc v0.0.30 // indirect
)
