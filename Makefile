# https://github.com/aperturerobotics/protobuf-project

PROTOWRAP=hack/bin/protowrap
PROTOC_GEN_GO=hack/bin/protoc-gen-go
PROTOC_GEN_STARPC=hack/bin/protoc-gen-go-starpc
PROTOC_GEN_GO_DRPC=hack/bin/protoc-gen-go-drpc
PROTOC_GEN_VTPROTO=hack/bin/protoc-gen-go-vtproto
GOIMPORTS=hack/bin/goimports
GOLANGCI_LINT=hack/bin/golangci-lint
GO_MOD_OUTDATED=hack/bin/go-mod-outdated
GOLIST=go list -f "{{ .Dir }}" -m

export GO111MODULE=on
undefine GOARCH
undefine GOOS

all:

vendor:
	go mod vendor

$(PROTOC_GEN_GO):
	cd ./hack; \
	go build -v \
		-o ./bin/protoc-gen-go \
		google.golang.org/protobuf/cmd/protoc-gen-go

$(PROTOC_GEN_VTPROTO):
	cd ./hack; \
	go build -v \
		-o ./bin/protoc-gen-go-vtproto \
		github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto

$(PROTOC_GEN_GO_DRPC):
	cd ./hack; \
	go build -v \
		-o ./bin/protoc-gen-go-drpc \
		storj.io/drpc/cmd/protoc-gen-go-drpc

$(PROTOC_GEN_STARPC):
	cd ./hack; \
	go build -v \
		-o ./bin/protoc-gen-go-starpc \
		github.com/aperturerobotics/starpc/cmd/protoc-gen-go-starpc

$(GOIMPORTS):
	cd ./hack; \
	go build -v \
		-o ./bin/goimports \
		golang.org/x/tools/cmd/goimports

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

$(GO_MOD_OUTDATED):
	cd ./hack; \
	go build -v \
		-o ./bin/go-mod-outdated \
		github.com/psampaz/go-mod-outdated

# Add --go-grpc_out=$$(pwd)/vendor to use the GRPC protoc generator.
# .. and remove the "grpc" option from the vtprotobuf features list.

.PHONY: gengo
gengo: $(GOIMPORTS) $(PROTOWRAP) $(PROTOC_GEN_GO) $(PROTOC_GEN_VTPROTO) $(PROTOC_GEN_GO_DRPC) $(PROTOC_GEN_STARPC)
	go mod vendor
	shopt -s globstar; \
	set -eo pipefail; \
	export PROJECT=$$(go list -m); \
	export PATH=$$(pwd)/hack/bin:$${PATH}; \
	mkdir -p $$(pwd)/vendor/$$(dirname $${PROJECT}); \
	rm $$(pwd)/vendor/$${PROJECT} || true; \
	ln -s $$(pwd) $$(pwd)/vendor/$${PROJECT} ; \
	$(PROTOWRAP) \
		-I $$(pwd)/vendor \
		--go_out=$$(pwd)/vendor \
		--go-vtproto_out=$$(pwd)/vendor \
		--go-vtproto_opt=features=marshal+unmarshal+size+equal \
		--go-drpc_out=$$(pwd)/vendor \
		--go-drpc_opt=json=false \
		--go-drpc_opt=protolib=github.com/golang/protobuf/proto \
		--go-drpc_opt=protolib=github.com/planetscale/vtprotobuf/codec/drpc \
		--go-starpc_out=$$(pwd)/vendor \
		--proto_path $$(pwd)/vendor \
		--print_structure \
		--only_specified_files \
		$$(\
			git \
				ls-files "*.proto" |\
				xargs printf -- \
				"$$(pwd)/vendor/$${PROJECT}/%s "); \
	rm $$(pwd)/vendor/$${PROJECT} || true
	$(GOIMPORTS) -w ./

node_modules:
	yarn install

.PHONY: gents
gents: $(PROTOWRAP) node_modules
	go mod vendor
	shopt -s globstar; \
	set -eo pipefail; \
	export PROJECT=$$(go list -m); \
	export PATH=$$(pwd)/hack/bin:$${PATH}; \
	mkdir -p $$(pwd)/vendor/$$(dirname $${PROJECT}); \
	rm $$(pwd)/vendor/$${PROJECT} || true; \
	ln -s $$(pwd) $$(pwd)/vendor/$${PROJECT} ; \
	$(PROTOWRAP) \
		-I $$(pwd)/vendor \
		--plugin=./node_modules/.bin/protoc-gen-ts_proto \
		--ts_proto_out=$$(pwd)/vendor \
		--ts_proto_opt=esModuleInterop=true \
		--ts_proto_opt=fileSuffix=.pb \
		--ts_proto_opt=importSuffix=.js \
		--ts_proto_opt=forceLong=long \
		--ts_proto_opt=oneof=unions \
		--ts_proto_opt=outputServices=default,outputServices=generic-definitions \
		--ts_proto_opt=useDate=true \
		--ts_proto_opt=useAsyncIterable=true \
		--proto_path $$(pwd)/vendor \
		--print_structure \
		--only_specified_files \
		$$(\
			git \
				ls-files "*.proto" |\
				xargs printf -- \
				"$$(pwd)/vendor/$${PROJECT}/%s "); \
	go mod vendor

.PHONY: genproto
genproto: gengo gents

.PHONY: gen
gen: genproto

.PHONY: outdated
outdated: $(GO_MOD_OUTDATED)
	go list -mod=mod -u -m -json all | $(GO_MOD_OUTDATED) -update -direct

.PHONY: list
list: $(GO_MOD_OUTDATED)
	go list -mod=mod -u -m -json all | $(GO_MOD_OUTDATED)

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

.PHONY: fix
fix: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fix

.PHONY: test
test:
	go test -v ./...
