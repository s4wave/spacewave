SHELL := /bin/bash
export GO111MODULE=on
GOLIST=go list -f "{{ .Dir }}" -m

GOLANGCI_LINT=hack/bin/golangci-lint
PROTOC_GEN_GO=hack/bin/protoc-gen-go
PROTOWRAP=hack/bin/protowrap

all:

vendor:
	go mod vendor

$(PROTOC_GEN_GO):
	cd ./hack; \
	go build -v \
		-o ./bin/protoc-gen-go \
		github.com/golang/protobuf/protoc-gen-go

$(PROTOWRAP):
	cd ./hack; \
	go build -v \
		-o ./bin/protowrap \
		github.com/square/goprotowrap/cmd/protowrap

$(GOLANGCI_LINT):
	cd ./hack; \
	go build -v \
		-o ./bin/golangci-lint \
		github.com/golangci/golangci-lint/cmd/golangci-lint

genproto: $(PROTOWRAP) $(PROTOC_GEN_GO) vendor
	shopt -s globstar; \
	set -eo pipefail; \
	export GO111MODULE=on; \
	export PROJECT=$$(go list -m); \
	export PATH=$$(pwd)/hack/bin:$${PATH}; \
	mkdir -p $$(pwd)/vendor/$$(dirname $${PROJECT}); \
	rm $$(pwd)/vendor/$${PROJECT} || true; \
	ln -s $$(pwd) $$(pwd)/vendor/$${PROJECT} ; \
	$(PROTOWRAP) \
		-I $$(pwd)/vendor \
		--go_out=plugins=grpc:$$(pwd)/vendor \
		--proto_path $$(pwd)/vendor \
		--print_structure \
		--only_specified_files \
		$$(\
			git \
				ls-files "*.proto" |\
				xargs printf -- \
				"$$(pwd)/vendor/$${PROJECT}/%s ")

gengo: genproto

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

test:
	go test -v ./...
