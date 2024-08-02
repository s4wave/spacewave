module github.com/aperturerobotics/identity

go 1.22

require github.com/aperturerobotics/hydra v0.0.0-20240802165027-eac0d36c0392 // master

require github.com/satori/go.uuid v1.2.0

// Note: The below is from the Hydra go.mod

require (
	github.com/aperturerobotics/bifrost v0.37.1 // master
	github.com/aperturerobotics/cayley v0.9.1 // latest
	github.com/aperturerobotics/json-iterator-lite v1.0.0 // indirect; latest
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/aperturerobotics/go-mysql-server v0.18.2-0.20240504092329-d5909fc5a93a // aperture
	github.com/dolthub/vitess => github.com/aperturerobotics/vitess v0.0.0-20240620014413-0cd132024ea5 // aperture
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
	xorm.io/xorm => github.com/paralin/go-xorm v1.3.3-0.20230216084813-0cd923e7ced6 // ext-engines
)

// aperture: use compatibility forks
replace (
	// https://github.com/dgraph-io/badger/pull/2048
	github.com/dgraph-io/badger/v4 => github.com/aperturerobotics/badger-go/v4 v4.0.0-20240504073313-17dd2ae7e207 // main
	// https://github.com/dgraph-io/ristretto/pull/375
	github.com/dgraph-io/ristretto => github.com/paralin/ristretto v0.1.2-0.20240221033725-e9838e36e9d8 // fix-wasm
	github.com/hidal-go/hidalgo => github.com/aperturerobotics/hidalgo v0.3.1-0.20231111025334-8015549a1b51 // aperture
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.12.2-0.20220323132038-01665499027f // aperture
)

require (
	github.com/Jeffail/gabs/v2 v2.7.0 // indirect
	github.com/paralin/gonum-graph-simple v0.0.0-20240410084948-b970da5ebf33 // indirect
	golang.org/x/sync v0.7.0 // indirect
)

// Note: the below is from the Bifrost go.mod

require (
	github.com/aperturerobotics/common v0.18.3 // latest
	github.com/aperturerobotics/controllerbus v0.47.3 // latest
	github.com/aperturerobotics/entitygraph v0.10.0 // indirect; latest
	github.com/aperturerobotics/protobuf-go-lite v0.6.5 // latest
	github.com/aperturerobotics/starpc v0.33.8 // latest
	github.com/aperturerobotics/util v1.25.7 // latest
)

// aperture: use compatibility forks
replace (
	github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master
	github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.33.1-0.20240511223728-e0b67c111765 // aperture
	github.com/libp2p/go-msgio => github.com/aperturerobotics/go-libp2p-msgio v0.0.0-20240511033615-1b69178aa5c8 // aperture
	github.com/multiformats/go-multiaddr => github.com/aperturerobotics/go-multiaddr v0.12.4-0.20240407071906-6f0354cc6755 // aperture
	github.com/multiformats/go-multihash => github.com/aperturerobotics/go-multihash v0.2.3 // aperture
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.0.0-20200820224411-1e751ff168ab // indirect: used by bifrost-nats-server
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20221228081037-b7c2df0c151f // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/nats-io/nkeys => github.com/nats-io/nkeys v0.3.0 // indirect: used by bifrost-nats-server
	github.com/quic-go/quic-go => github.com/aperturerobotics/quic-go v0.45.1-0.20240802054753-f83427ffc2c6 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
)

require (
	filippo.io/edwards25519 v1.1.1-0.20231210192602-a7dfd8e4e6b4 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/libp2p/go-libp2p v0.35.4
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.13.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/quic-go/quic-go v0.45.2 // indirect
	github.com/sirupsen/logrus v1.9.3
	github.com/zeebo/blake3 v0.2.3 // indirect
	golang.org/x/crypto v0.25.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	gonum.org/v1/gonum v0.15.0 // indirect
	nhooyr.io/websocket v1.8.11 // indirect; master
)

require github.com/blang/semver/v4 v4.0.0

require (
	github.com/cloudflare/circl v1.3.8 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/pprof v0.0.0-20240207164012-fb44976bdcd5 // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.2-0.20240322071716-53ef5820bd48 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/onsi/ginkgo/v2 v2.15.0 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/mod v0.19.0 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	golang.org/x/tools v0.23.0 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)
