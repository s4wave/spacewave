module github.com/aperturerobotics/forge

go 1.16

require (
	github.com/aperturerobotics/bifrost v0.1.1-0.20220321113118-76dcc5645519
	github.com/aperturerobotics/controllerbus v0.8.9-0.20220302112603-00cd479c3d0a
	github.com/aperturerobotics/entitygraph v0.1.4-0.20210530040557-f19da9c2be6d
	github.com/aperturerobotics/hydra v0.0.0-20220321115133-d86f1dc6678b
	github.com/aperturerobotics/identity v0.0.0-20220301040115-b7aa1158c7ad
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
	// github.com/cayleygraph/quad => github.com/paralin/cayley-quad v1.2.5-0.20211209073857-a28a5348625f // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.4.3-0.20211116083949-5904ad760e00 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/marten-seemann/qtls-go1-16 => github.com/paralin/qtls-go1-16 v0.1.5-0.20210728071944-419a2c247411 // gopherjs-compat
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.16-0.20210728072548-664b46444f01 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20210804024047-dc49ac2ea3b4 // gopherjs-compat
	github.com/sirupsen/logrus => github.com/paralin/logrus v1.8.2-0.20210804014116-ae269fb01c6c // gopherjs-compat
	golang.org/x/crypto => github.com/aperturerobotics/golang-x-crypto v0.0.0-20220321111526-87c0d0398f72 // gopherjs-compat
)

// Note: the below is from the Bifrost go.mod

// aperture: use compatibility forks
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/libp2p/go-libp2p-core => github.com/paralin/go-libp2p-core v0.14.1-0.20220321111733-8010b7b24680 // aperture
	github.com/libp2p/go-libp2p-tls => github.com/paralin/go-libp2p-tls v0.3.2-0.20220321112951-db0cab39ed18 // js-compat
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.23.1-0.20220321112440-8295926e98d6 // aperture
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
	github.com/paralin/kcp-go-lite => github.com/paralin/kcp-go-lite v1.0.2-0.20210907043027-271505668bd0 // aperture
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	storj.io/drpc => github.com/paralin/drpc v0.0.30-0.20220301023015-b1e9d6bd9478 // aperture
)

require (
	github.com/Jeffail/gabs v1.4.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7-0.20220304214302-275a7428fb10
	github.com/cayleygraph/quad v1.2.4
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-git/v5 v5.4.3-0.20220224134545-c785af3f4559
	github.com/golang/protobuf v1.5.2
	github.com/libp2p/go-libp2p-core v0.14.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	github.com/valyala/fastjson v1.6.3
	storj.io/drpc v0.0.30
)
