PROTOWRAP=gobin -run github.com/square/goprotowrap/cmd/protowrap@master
GOLIST=go list -f "{{ .Dir }}" -m

all:

gengo: vendor/bin/gobin
	shopt -s globstar; \
	set -eo pipefail; \
	export GO111MODULE=on; \
	export PROJECT=$$(go list -m); \
	export PATH=$$(pwd)/vendor/bin:$${PATH}; \
	mkdir -p $$(pwd)/vendor/github.com/aperturerobotics; \
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


vendor/bin/gobin:
	mkdir -p vendor/bin
	go mod vendor
	export GO111MODULE=on ; \
	cd ./vendor ; touch go.mod ; \
	go mod tidy -v ; \
	go build -v \
		-o ./bin/protoc-gen-go \
		github.com/golang/protobuf/protoc-gen-go ; \
	go build -v \
		-o ./bin/gobin \
		github.com/myitcv/gobin

test:
	go test -v ./...
