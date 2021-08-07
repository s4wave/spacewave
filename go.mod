module github.com/aperturerobotics/bldr

go 1.16

// JS / Browser-specific replacements

replace (
	github.com/dgraph-io/badger/v2 => github.com/paralin/badger/v2 v2.0.1-rc1.0.20200609030847-c83b793e278e // fix-32bit
	github.com/rgraphql/magellan => github.com/rgraphql/magellan v0.6.3-0.20200830093112-c88cc1f643b2 // master
	github.com/rgraphql/rgraphql => github.com/rgraphql/rgraphql v1.0.8 // master
)

// Copied from hydra go.mod

// aperture: use js-compat forks
replace (
	github.com/cayleygraph/cayley => github.com/aperturerobotics/cayley v0.7.7-0.20210804025450-76a92a481ea5 // aperture
	github.com/go-git/go-git/v5 => github.com/paralin/go-git/v5 v5.3.1-0.20210804011724-d84485be5d08 // gopherjs-compat
	github.com/json-iterator/go => github.com/paralin/json-iterator-go v1.1.8-0.20191007015249-d1055a931522 // js-compat
	github.com/libp2p/go-libp2p-tls => github.com/paralin/go-libp2p-tls v0.1.4-0.20210728062949-a42c760a733f // js-compat
	github.com/marten-seemann/qtls-go1-16 => github.com/paralin/qtls-go1-16 v0.1.5-0.20210728071944-419a2c247411 // gopherjs-compat
	github.com/prometheus/client_golang => github.com/paralin/prometheus_client_golang v1.10.1-0.20210804024047-dc49ac2ea3b4 // gopherjs-compat
	github.com/sirupsen/logrus => github.com/paralin/logrus v1.8.2-0.20210804014116-ae269fb01c6c // gopherjs-compat
	google.golang.org/grpc => github.com/paralin/grpc-go v1.30.1-0.20210804030014-1587a7c16b66 // aperture
)

// aperture: use aperture forks
replace (
	github.com/bits-and-blooms/bitset => github.com/paralin/go-blooms-bitset v1.2.1-0.20210621003254-d10d8d6ab8b7 // aperture
	github.com/bits-and-blooms/bloom/v3 => github.com/paralin/go-bloom/v3 v3.0.2-0.20210621003511-7e4e43980591 // aperture
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

// aperture: use ext-engines forks
replace (
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.10.1-0.20210715210115-22d267bf1416 // ext-engines
	github.com/dolthub/vitess => github.com/paralin/vitess v0.0.0-20210611010940-f1489325f50b // ext-engines
	github.com/genjidb/genji => github.com/paralin/genji v0.12.1-0.20210715210024-97123bb291e7 // ext-engines
	github.com/go-sql-driver/mysql => github.com/paralin/go-mysql-driver v1.6.1-0.20210605044355-486b076ae739 // ext-engines
)

// aperture: use forks for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.22.1-0.20210728081144-c7bd4637cac2 // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
// google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

require (
	github.com/Microsoft/go-winio v0.4.16
	github.com/aperturerobotics/auth v0.0.0-20210703095553-e512075b9cbf
	github.com/aperturerobotics/bifrost v0.0.0-20210804000255-0a27eb950f05
	github.com/aperturerobotics/controllerbus v0.8.4-0.20210729091933-eb89d362c5c2
	github.com/aperturerobotics/hydra v0.0.0-20210804005056-95d36cde85d0
	github.com/evanw/esbuild v0.12.17
	github.com/golang/protobuf v1.5.2
	github.com/gopherjs/gopherjs v0.0.0-20210803090616-8f023c250c89
	github.com/manifoldco/promptui v0.8.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
)
