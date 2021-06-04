module github.com/aperturerobotics/hydra

go 1.15

// aperture: use protobuf 1.3.x based fork for compatibility
replace (
	github.com/golang/protobuf => github.com/aperturerobotics/go-protobuf-1.3.x v0.0.0-20200726220404-fa7f51c52df0 // aperture-1.3.x
	github.com/lucas-clemente/quic-go => github.com/aperturerobotics/quic-go v0.7.1-0.20210518124640-25c39ec20d1d // aperture-protobuf-1.3.x
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.30.0
)

// aperture: use aperture forks
replace (
	github.com/ProtonMail/go-crypto => github.com/paralin/go-crypto v0.0.0-20210427232619-f5bd188194a5 // gopherjs-compat
	github.com/dolthub/go-mysql-server => github.com/paralin/go-mysql-server v0.10.1-0.20210603025017-7f4f011b68d5 // fixes
	github.com/genjidb/genji => github.com/paralin/genji v0.12.1-0.20210603025425-11ee02d7b08d // ext-engines
	github.com/multiformats/go-multihash => github.com/paralin/go-multihash v0.0.11-0.20200526102400-a989a5c6678b // gopherjs-compat
	github.com/nats-io/nats-server/v2 => github.com/aperturerobotics/bifrost-nats-server/v2 v2.1.8-0.20200831101324-59acc8fe7f74 // aperture-2.0
	github.com/nats-io/nats.go => github.com/aperturerobotics/bifrost-nats-client v1.10.1-0.20200831103200-24c3d0464e58 // aperture-2.0
)

require (
	github.com/Workiva/go-datastructures v1.0.53
	github.com/aperturerobotics/bifrost v0.0.0-20210530043826-59960619699d
	github.com/aperturerobotics/controllerbus v0.8.1-0.20210530041705-07db80f9adfe
	github.com/aperturerobotics/entitygraph v0.1.4-0.20210530040557-f19da9c2be6d
	github.com/aperturerobotics/timestamp v0.2.3
	github.com/blang/semver v3.5.1+incompatible
	github.com/cayleygraph/cayley v0.7.7-0.20210518204410-08381efb7f81
	github.com/cayleygraph/quad v1.2.4
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/dolthub/go-mysql-server v0.10.1-0.20210602232312-a3862060d72b
	github.com/dolthub/vitess v0.0.0-20210530214338-7755381e6501
	github.com/emirpasic/gods v1.12.0
	github.com/genjidb/genji v0.8.1-0.20201112071311-72319d2a2285
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/golang/protobuf v1.4.3
	github.com/golang/snappy v0.0.4-0.20210502035320-33fc3d5d8d99
	github.com/gomodule/redigo v1.8.4
	github.com/gopherjs/gopherjs v0.0.0-20210602210359-451d92adc31b
	github.com/hidal-go/hidalgo v0.0.0-20201109092204-05749a6d73df
	github.com/libp2p/go-libp2p-core v0.8.5
	github.com/libp2p/go-libp2p-crypto v0.1.0
	github.com/mr-tron/base58 v1.2.0
	github.com/paralin/go-indexeddb v0.0.0-20201108213622-b8aa4a40cb6e
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.10.0 // indirect
	github.com/restic/chunker v0.4.0
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.5
	github.com/vmihailenco/msgpack/v5 v5.3.2
	go.etcd.io/bbolt v1.3.5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/tools v0.1.3-0.20210602194553-384c3925727e // indirect
	gonum.org/v1/gonum v0.9.1-0.20210601193436-a683c830ae36
	google.golang.org/grpc v1.31.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gorm.io/gorm v1.21.10
)
