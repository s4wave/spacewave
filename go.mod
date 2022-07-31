module github.com/aperturerobotics/identity

go 1.18

require github.com/aperturerobotics/hydra v0.0.0-20220730225558-78c17e7b6422

// Note: The below is from the Hydra go.mod

require github.com/aperturerobotics/bifrost v0.4.2

// aperture: use ext-engines forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20220321114736-873b5e61a63c // aperture
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.11.1-0.20220603082843-e7608e49b7e5 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20220603082514-23299e614067 // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.13.1-0.20210906212411-d9723e75eaa0 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210703095932-8592b046e48a // ext-engines
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
	github.com/aperturerobotics/controllerbus v0.12.2
	github.com/aperturerobotics/entitygraph v0.2.2 // indirect
	github.com/aperturerobotics/starpc v0.9.0
)

// aperture: use compatibility forks
replace (
	github.com/libp2p/go-libp2p-core => github.com/paralin/go-libp2p-core v0.19.1-0.20220702014232-392fe0626689 // aperture
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.28.1-0.20220710071647-c608532b9a6e // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.8.2-0.20220322010420-77ab346a2cf8 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.27.2-0.20220609075637-a1d116b0035f // aperture
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.31-0.20220527065730-0e2a1370bccb // aperture
)

require (
	github.com/aperturerobotics/timestamp v0.6.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7-0.20220304214302-275a7428fb10
	github.com/cayleygraph/quad v1.2.4
	github.com/libp2p/go-libp2p-core v0.19.1
	github.com/pkg/errors v0.9.1
	github.com/planetscale/vtprotobuf v0.3.0
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.9.0
	google.golang.org/protobuf v1.28.1
	storj.io/drpc v0.0.30
)

require (
	github.com/btcsuite/btcd v0.22.1 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.0 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.0.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gobuffalo/envy v1.7.1 // indirect
	github.com/gobuffalo/logger v1.0.1 // indirect
	github.com/gobuffalo/packd v0.3.0 // indirect
	github.com/gobuffalo/packr/v2 v2.7.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ipfs/go-cid v0.2.0 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/joho/godotenv v1.3.0 // indirect
	github.com/klauspost/compress v1.15.6 // indirect
	github.com/klauspost/cpuid/v2 v2.0.14 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-libp2p v0.21.0 // indirect
	github.com/libp2p/go-mplex v0.7.1-0.20220702225122-8cbdf39b21f5 // indirect
	github.com/libp2p/go-openssl v0.0.7 // indirect
	github.com/lucas-clemente/quic-go v0.28.0 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.5 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.2 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.2 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.0-beta.1 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.0.4 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multiaddr v0.6.0 // indirect
	github.com/multiformats/go-multibase v0.1.1 // indirect
	github.com/multiformats/go-multicodec v0.5.0 // indirect
	github.com/multiformats/go-multihash v0.2.0 // indirect
	github.com/multiformats/go-varint v0.0.6 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/spf13/cobra v0.0.5 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/zeebo/blake3 v0.2.3 // indirect
	github.com/zeebo/errs v1.2.2 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4 // indirect
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/tools v0.1.12 // indirect
	gonum.org/v1/gonum v0.11.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	nhooyr.io/websocket v1.8.8-0.20210410000328-8dee580a7f74 // indirect
)
