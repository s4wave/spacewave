module github.com/s4wave/spacewave/lint/nonavigate

go 1.26.0

require (
	github.com/golangci/plugin-module-register v0.1.2
	golang.org/x/tools v0.50.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	honnef.co/go/tools v0.8.1 // indirect
)

// Keep the custom linter host's Staticcheck IR compatible with x/tools.
tool honnef.co/go/tools/cmd/staticcheck
