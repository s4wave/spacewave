module github.com/aperturerobotics/forge

go 1.16

require (
	github.com/aperturerobotics/bifrost v0.1.2-0.20220322011412-e75f5260ed95
	github.com/aperturerobotics/controllerbus v0.9.1-0.20220322004716-ca57d2643bca
	github.com/aperturerobotics/entitygraph v0.2.0
	github.com/aperturerobotics/hydra v0.0.0-20220322011627-c5d553f7f326
	github.com/aperturerobotics/identity v0.0.0-20220321130854-1c7dd027eda6
	github.com/aperturerobotics/timestamp v0.4.0
)

// Copied from Hydra go.mod

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.11.1-0.20220315071359-d18204a140a5 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20220315035103-ee808c4b8def // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.13.1-0.20210906212411-d9723e75eaa0 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210703095932-8592b046e48a // ext-engines
)

// aperture: use compatibility forks
replace (
	github.com/bits-and-blooms/bloom/v3 => github.com/paralin/go-bloom/v3 v3.1.1-0.20220321113354-ddfde510cc94 // aperture
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20220321114736-873b5e61a63c // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.4.3-0.20211116083949-5904ad760e00 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.16-0.20210728072548-664b46444f01 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20210804024047-dc49ac2ea3b4 // gopherjs-compat
)

// Note: the below is from the Bifrost go.mod

// aperture: use compatibility forks
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/libp2p/go-libp2p-core => github.com/paralin/go-libp2p-core v0.14.1-0.20220321111733-8010b7b24680 // aperture
	github.com/libp2p/go-libp2p-tls => github.com/paralin/go-libp2p-tls v0.3.2-0.20220322010743-2af8fcae7b5b // js-compat
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.25.1-0.20220322005723-dee99cd12a43 // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	golang.org/x/crypto => github.com/aperturerobotics/golang-x-crypto v0.0.0-20220322011112-8d5764cfba1c // gopherjs-compat
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	nhooyr.io/websocket => github.com/paralin/nhooyr-websocket v1.8.8-0.20220321125022-7defdf942f07 // aperture
	storj.io/drpc => github.com/paralin/drpc v0.0.30-0.20220301023015-b1e9d6bd9478 // aperture
)

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7
	github.com/cayleygraph/quad v1.2.4
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-git/v5 v5.4.2
	github.com/golang/protobuf v1.5.2
	github.com/libp2p/go-libp2p-core v0.14.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.2-0.20220112234510-85981c045988
	github.com/urfave/cli v1.22.5
	github.com/valyala/fastjson v1.6.3
	storj.io/drpc v0.0.30
)
