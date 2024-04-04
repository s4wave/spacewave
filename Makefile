# https://github.com/aperturerobotics/protobuf-project

SHELL:=bash
PROTOWRAP=hack/bin/protowrap
PROTOC_GEN_GO=hack/bin/protoc-gen-go
PROTOC_GEN_STARPC=hack/bin/protoc-gen-go-starpc
PROTOC_GEN_VTPROTO=hack/bin/protoc-gen-go-vtproto
GOIMPORTS=hack/bin/goimports
GOFUMPT=hack/bin/gofumpt
GOLANGCI_LINT=hack/bin/golangci-lint
GO_MOD_OUTDATED=hack/bin/go-mod-outdated
GOLIST=go list -f "{{ .Dir }}" -m

export GO111MODULE=on
undefine GOOS
undefine GOARCH

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

$(GOFUMPT):
	cd ./hack; \
	go build -v \
		-o ./bin/gofumpt \
		mvdan.cc/gofumpt

$(PROTOWRAP):
	cd ./hack; \
	go build -v \
		-o ./bin/protowrap \
		github.com/aperturerobotics/goprotowrap/cmd/protowrap

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

.PHONY: gengo
gengo: vendor $(GOIMPORTS) $(PROTOWRAP) $(PROTOC_GEN_GO) $(PROTOC_GEN_VTPROTO) $(PROTOC_GEN_STARPC)
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
		--go-vtproto_opt=features=marshal+unmarshal+size+equal+clone \
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
gents: vendor $(PROTOWRAP) node_modules
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
		--ts_proto_opt=useAbortSignal=true \
		--ts_proto_opt=useAsyncIterable=true \
		--ts_proto_opt=useDate=true \
		--proto_path $$(pwd)/vendor \
		--print_structure \
		--only_specified_files \
		$$(\
			git \
				ls-files "*.proto" |\
				xargs printf -- \
				"$$(pwd)/vendor/$${PROJECT}/%s "); \
	rm $$(pwd)/vendor/$${PROJECT} || true
	npm run format:js

.PHONY: genproto
genproto: gents gengo

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
	$(GOLANGCI_LINT) run --timeout=10m

.PHONY: fix
fix: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fix --timeout=10m

.PHONY: format
format: $(GOFUMPT) $(GOIMPORTS)
	$(GOIMPORTS) -w ./
	$(GOFUMPT) -w ./

.PHONY: test
test:
	go test -v ./...
