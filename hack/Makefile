GO_MOD_OUTDATED=bin/go-mod-outdated

export GO111MODULE=on
undefine GOARCH
undefine GOOS

$(GO_MOD_OUTDATED):
	go build -v \
		-o ./bin/go-mod-outdated \
		github.com/psampaz/go-mod-outdated

.PHONY: outdated
outdated: $(GO_MOD_OUTDATED)
	go list -mod=mod -u -m -json all | $(GO_MOD_OUTDATED) -update -direct
