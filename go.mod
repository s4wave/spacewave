module github.com/aperturerobotics/auth

go 1.21

require (
	github.com/aperturerobotics/identity v0.0.0-20231030184619-16d497d6b66b // master
	github.com/keybase/go-triplesec v0.0.0-20221220225315-06ddee08f3c2
	github.com/manifoldco/promptui v0.9.0
)

// Note: the below is from the identity go.mod

require github.com/aperturerobotics/hydra v0.0.0-20231030184105-4c0a71b2f606 // indirect; master

require github.com/satori/go.uuid v1.2.0

// Note: The below is from the Hydra go.mod

require github.com/aperturerobotics/bifrost v0.18.9 // master

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.15.1-0.20230424215448-944f16b19434 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20230423223447-1f5734a618e1 // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.14.1-0.20230213145718-23097a679f40 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
	xorm.io/xorm => github.com/paralin/go-xorm v1.3.3-0.20230216084813-0cd923e7ced6 // ext-engines
)

// aperture: use compatibility forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20230526013106-bcbeda7f50f0 // aperture
	github.com/cayleygraph/quad => github.com/aperturerobotics/cayley-quad v1.2.5-0.20230524232228-dc08772d0195 // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.6.2-0.20230322095819-b641fd8f849b // gopherjs-compat
	github.com/hidal-go/hidalgo => github.com/aperturerobotics/hidalgo v0.2.1-0.20230526002043-6e494c6ad96b // aperture
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.2.0 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.12.2-0.20220323132038-01665499027f // aperture
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/controllerbus v0.30.7 // latest
	github.com/aperturerobotics/entitygraph v0.4.0 // indirect
	github.com/aperturerobotics/starpc v0.21.2 // indirect; latest
)

// aperture: use compatibility forks
replace (
	github.com/multiformats/go-multiaddr => github.com/paralin/go-multiaddr v0.10.2-0.20230807174004-e1767541c061 // aperture
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.0.0-20200820224411-1e751ff168ab // indirect: used by bifrost-nats-server
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20221228081037-b7c2df0c151f // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/nats-io/nkeys => github.com/nats-io/nkeys v0.3.0 // indirect: used by bifrost-nats-server
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	github.com/quic-go/quic-go => github.com/aperturerobotics/quic-go v0.37.2-0.20230807175030-579e965e762f // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.1-0.20221224130652-ff61cbb763af // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.31.1-0.20231012212426-9cf9f0f94f47 // aperture
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.31-0.20220527065730-0e2a1370bccb // aperture
)

require (
	github.com/aperturerobotics/util v1.7.6 // indirect; master
	github.com/blang/semver v3.5.1+incompatible
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/libp2p/go-libp2p v0.32.0
	github.com/libp2p/go-yamux/v4 v4.0.1 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/multiformats/go-multiaddr v0.12.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/quic-go/quic-go v0.39.3 // indirect
	github.com/sirupsen/logrus v1.9.3
	github.com/zeebo/blake3 v0.2.3
	golang.org/x/crypto v0.14.0 // indirect
	gonum.org/v1/gonum v0.14.0 // indirect
	google.golang.org/protobuf v1.31.0
	nhooyr.io/websocket v1.8.10 // indirect
)

require github.com/urfave/cli/v2 v2.25.7

require (
	filippo.io/edwards25519 v1.0.1-0.20220803165937-8c58ed0e3550 // indirect
	github.com/aperturerobotics/timestamp v0.8.2 // indirect
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/pprof v0.0.0-20231023181126-ff6d637d2a7b // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multicodec v0.9.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/onsi/ginkgo/v2 v2.13.0 // indirect
	github.com/quic-go/qtls-go1-20 v0.3.4 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/mod v0.13.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)
