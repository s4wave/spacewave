PROTOWRAP=hack/bin/protowrap
PROTOC_GEN_GO=hack/bin/protoc-gen-go
GOLIST=go list -f "{{ .Dir }}" -m

all:

vendor:
	export GO111MODULE=on; \
  go mod vendor

$(PROTOC_GEN_GO):
	export GO111MODULE=on; \
  cd ./hack; \
	go build -v \
		-o ./bin/protoc-gen-go \
		github.com/golang/protobuf/protoc-gen-go

$(PROTOWRAP):
	export GO111MODULE=on; \
  cd ./hack; \
	go build -v \
		-o ./bin/protowrap \
		github.com/square/goprotowrap/cmd/protowrap

gengo: $(PROTOWRAP) $(PROTOC_GEN_GO) vendor
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

