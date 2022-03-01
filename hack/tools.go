package hack

import (
	// _ imports protowrap
	_ "github.com/square/goprotowrap"
	// _ imports protoc-gen-go
	_ "github.com/golang/protobuf/proto"
	// _ imports golangci-lint
	_ "github.com/golangci/golangci-lint/pkg/golinters"
	// _ imports drpc
	_ "storj.io/drpc/cmd/protoc-gen-go-drpc"
	// _ imports golangci-lint commands
	_ "github.com/golangci/golangci-lint/pkg/commands"
)
