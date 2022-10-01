module plugin/sandbox

go 1.18

replace github.com/aperturerobotics/bldr => ../../

require github.com/aperturerobotics/starpc v0.10.7-0.20220928023314-05139d4c0dba // indirect

require (
	github.com/libp2p/go-mplex v0.7.1-0.20220825125536-a00a1352b54f // indirect
	golang.org/x/sync v0.0.0-20220819030929-7fc1605a5dde // indirect
)

replace (
	github.com/atotto/clipboard => github.com/paralin/atotto-clipboard v0.1.5-0.20220104232832-1bce292d51d0 // aperture
	github.com/charmbracelet/bubbletea => github.com/paralin/bubbletea v0.22.1-0.20220730231455-545b2354800a // aperture
	github.com/charmbracelet/lipgloss => github.com/paralin/lipgloss v0.4.1-0.20220101103150-467b03d84258 // aperture
	github.com/containerd/console => github.com/paralin/containerd-console v1.0.4-0.20220104234132-95e7aa4e3ecb // aperture
)

// The following is from the Forge go.mod.

// The following is from the Containers go.mod

replace (
	github.com/containers/image/v5 => github.com/paralin/containers-image/v5 v5.0.0-20220822072753-3116272a19fe // aperture
	github.com/containers/podman/v4 => github.com/paralin/podman/v4 v4.0.0-rc2.0.20220609081906-c641f9978e98 // aperture
)

require github.com/aperturerobotics/hydra v0.0.0-20220928020915-069b0c33d14d // indirect

// Note: the below is from the Hydra go.mod

require github.com/aperturerobotics/bifrost v0.6.2-0.20220928022749-43facb54fd8b // indirect

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
	github.com/aperturerobotics/controllerbus v0.14.5-0.20220930060516-890d1cd53f97 // indirect
	github.com/aperturerobotics/entitygraph v0.2.2 // indirect
)

// aperture: use compatibility forks
replace (
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.28.2-0.20220816034953-16dc6b89a8f8 // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.8.2-0.20220322010420-77ab346a2cf8 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.27.2-0.20220609075637-a1d116b0035f // aperture
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.31-0.20220527065730-0e2a1370bccb // aperture
)

require github.com/aperturerobotics/bldr v0.0.0-00010101000000-000000000000

require (
	github.com/Workiva/go-datastructures v1.0.53 // indirect
	github.com/aperturerobotics/timestamp v0.6.0 // indirect
	github.com/aperturerobotics/ts-proto-common-types v0.2.0 // indirect
	github.com/bits-and-blooms/bitset v1.3.0 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cayleygraph/cayley v0.7.7 // indirect
	github.com/cayleygraph/quad v1.2.4 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/dgraph-io/badger/v2 v2.2007.4 // indirect
	github.com/dgraph-io/ristretto v0.0.3-0.20200630154024-f66de99634de // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gobuffalo/logger v1.0.3 // indirect
	github.com/gobuffalo/packd v1.0.0 // indirect
	github.com/gobuffalo/packr/v2 v2.8.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gomodule/redigo v1.8.9 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/ipfs/go-cid v0.3.2 // indirect
	github.com/ipfs/go-log/v2 v2.5.1 // indirect
	github.com/karrick/godirwalk v1.16.1 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/klauspost/cpuid/v2 v2.1.1 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-libp2p v0.23.2 // indirect
	github.com/libp2p/go-openssl v0.1.0 // indirect
	github.com/lucas-clemente/quic-go v0.29.1 // indirect
	github.com/markbates/errx v1.1.0 // indirect
	github.com/markbates/oncer v1.0.0 // indirect
	github.com/markbates/safe v1.0.1 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.2 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.0 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multiaddr v0.7.0 // indirect
	github.com/multiformats/go-multibase v0.1.1 // indirect
	github.com/multiformats/go-multicodec v0.6.0 // indirect
	github.com/multiformats/go-multihash v0.2.1 // indirect
	github.com/multiformats/go-varint v0.0.6 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.19.0 // indirect
	github.com/paralin/go-indexeddb v1.0.1 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/restic/chunker v0.4.0 // indirect
	github.com/rogpeppe/go-internal v1.8.1 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/spf13/cobra v1.5.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/zeebo/assert v1.3.0 // indirect
	github.com/zeebo/blake3 v0.2.3 // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/crypto v0.0.0-20220926161630-eccd6366d1be // indirect
	golang.org/x/exp v0.0.0-20220916125017-b168a2c6b86b // indirect
	golang.org/x/mod v0.6.0-dev.0.20220906170120-8f535f745b87 // indirect
	golang.org/x/net v0.0.0-20220920183852-bf014ff85ad5 // indirect
	golang.org/x/sys v0.0.0-20220811171246-fbc7d0a398ab // indirect
	golang.org/x/term v0.0.0-20220526004731-065cf7ba2467 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.12 // indirect
	gonum.org/v1/gonum v0.12.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	nhooyr.io/websocket v1.8.8-0.20210410000328-8dee580a7f74 // indirect
)
