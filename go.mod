module github.com/aperturerobotics/auth

go 1.25

require (
	github.com/aperturerobotics/abseil-cpp v0.0.0-20260131110040-4bb56e2f9017 // indirect
	github.com/aperturerobotics/bifrost v0.45.2-0.20260209015539-8e35acc2a1d3 // master
	github.com/aperturerobotics/common v0.30.1 // latest
	github.com/aperturerobotics/controllerbus v0.52.1 // latest
	github.com/aperturerobotics/entitygraph v0.11.0 // indirect
	github.com/aperturerobotics/identity v0.0.0-20260209041344-6c9d8dfb78a9 // master
	github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20251104042408-0c9eb8a3f726 // indirect
	github.com/aperturerobotics/protobuf v0.0.0-20260203024654-8201686529c4 // indirect
	github.com/aperturerobotics/protobuf-go-lite v0.12.1 // latest
	github.com/aperturerobotics/starpc v0.46.1 // indirect
	github.com/aperturerobotics/util v1.32.3 // indirect
)

require (
	github.com/keybase/go-triplesec v0.0.0-20231213205702-981541df982e
	github.com/libp2p/go-libp2p v0.47.0
	github.com/manifoldco/promptui v0.9.0 // latest
	github.com/mr-tron/base58 v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.9.4
	github.com/urfave/cli/v2 v2.27.5
	github.com/zeebo/blake3 v0.2.4
)

require (
	filippo.io/edwards25519 v1.1.1-0.20250211130249-04b037b40df0 // indirect
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/keybase/go-crypto v0.0.0-20200123153347-de78d2cb44f4 // indirect
	github.com/klauspost/compress v1.18.3 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.2 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.16.1 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/paralin/gonum-graph-simple v0.0.0-20240410084948-b970da5ebf33 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spaolacci/murmur3 v1.1.1-0.20190317074736-539464a789e9 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)

// Note: the below is from the identity go.mod

require github.com/aperturerobotics/hydra v0.0.0-20260131061755-2317d7bae7e5 // indirect; master

// Note: The below is from the Hydra go.mod

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/aperturerobotics/go-mysql-server v0.18.2-0.20240821042240-d51583de8ec0 // aperture
	github.com/dolthub/vitess => github.com/aperturerobotics/vitess v0.0.0-20240821040752-39ac045ae8fe // aperture
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.7.1-0.20230216081317-8a59f6dde100 // ext-engines
	xorm.io/xorm => github.com/paralin/go-xorm v1.3.3-0.20230216084813-0cd923e7ced6 // ext-engines
)

// aperture: use compatibility forks
replace (
	// https://github.com/dgraph-io/badger/pull/2048
	github.com/dgraph-io/badger/v4 => github.com/aperturerobotics/badger-go/v4 v4.0.0-20241029084129-c1a1dbed1aac // main
	github.com/hidal-go/hidalgo => github.com/aperturerobotics/hidalgo v0.3.1-0.20231111025334-8015549a1b51 // aperture
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.12.2-0.20220323132038-01665499027f // aperture
)

// Note: the below is from the Bifrost go.mod

// aperture: use compatibility forks
replace (
	github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master
	github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture
	github.com/libp2p/go-msgio => github.com/aperturerobotics/go-libp2p-msgio v0.0.0-20240511033615-1b69178aa5c8 // aperture
	github.com/multiformats/go-multiaddr => github.com/aperturerobotics/go-multiaddr v0.12.4-0.20240407071906-6f0354cc6755 // aperture
	github.com/multiformats/go-multihash => github.com/aperturerobotics/go-multihash v0.2.3 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
)

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/coder/websocket v1.8.14 // indirect
)
