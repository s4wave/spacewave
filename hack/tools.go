package hack

import (
	// _ imports protowrap
	_ "github.com/aperturerobotics/goprotowrap/cmd/protowrap"
	// _ imports protoc-gen-go
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	// _ imports protoc-gen-go-vtproto
	_ "github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto"
	// _ imports golangci-lint
	_ "github.com/golangci/golangci-lint/pkg/golinters"
	// _ imports golangci-lint commands
	_ "github.com/golangci/golangci-lint/pkg/commands"
	// _ imports go-mod-outdated
	_ "github.com/psampaz/go-mod-outdated"
	// _ imports protoc-gen-starpc
	_ "github.com/aperturerobotics/starpc/cmd/protoc-gen-go-starpc"
	// _ imports esbuild
	_ "github.com/evanw/esbuild/cmd/esbuild"
)
