module github.com/s4wave/spacewave/lint/quadscan

go 1.27.0

require (
	github.com/golangci/plugin-module-register v0.1.2
	github.com/s4wave/spacewave v0.57.30
	golang.org/x/tools v0.52.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/aperturerobotics/cayley v0.15.1-0.20261026172548-43cf8e6be6cb // indirect
	github.com/aperturerobotics/common v0.36.5 // indirect
	github.com/aperturerobotics/controllerbus v0.53.6-0.20261029175750-384f96d17d5a // indirect
	github.com/aperturerobotics/fastjson v0.1.2-0.20261024220211-b654d1d5b5aa // indirect
	github.com/aperturerobotics/json-iterator-lite v1.1.0 // indirect
	github.com/aperturerobotics/protobuf-go-lite v0.19.1-0.20261028131920-f644656fc4a7 // indirect
	github.com/aperturerobotics/util v1.34.10-0.20261024231407-c6b31c4e4ea6 // indirect
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/mr-tron/base58 v1.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	honnef.co/go/tools v0.8.2 // indirect
)

// Keep the custom linter host's Staticcheck IR compatible with x/tools.
tool honnef.co/go/tools/cmd/staticcheck
