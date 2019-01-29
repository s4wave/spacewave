gengo:
	shopt -s globstar; \
	protowrap -I $${GOPATH}/src \
		--go_out=plugins=grpc:$${GOPATH}/src \
		--proto_path $${GOPATH}/src \
		--print_structure \
		--only_specified_files \
		$$(pwd)/**/*.proto
	go build -i -v ./...

deps:
	go get -u -v github.com/golang/protobuf/protoc-gen-go
	go get -v github.com/square/goprotowrap/cmd/protowrap

reportcard:
	go get -v github.com/gojp/goreportcard/cmd/goreportcard-cli
	goreportcard-cli -v

test:
	go test -v ./...

