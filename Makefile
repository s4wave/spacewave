gengo:
	shopt -s globstar; \
	protowrap -I $${GOPATH}/src \
		--go_out=$${GOPATH}/src \
		--proto_path $${GOPATH}/src \
		--print_structure \
		--only_specified_files \
		$$(pwd)/**/*.proto
	go install -v ./...

deps:
	go get -u -v github.com/golang/protobuf/protoc-gen-go
	go get -v github.com/square/goprotowrap/cmd/protowrap

test:
	go test -v ./...

