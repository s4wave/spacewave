# Run these targets on the Linux build host, through the remote-build SOP.
OUT ?= /home/core/build-artifacts/spacewave-os
PKGS = ./os/boot ./os/cmd/spacewave-init
export CGO_ENABLED = 0
export GOMAXPROCS = 4

.PHONY: build check test
build:
	mkdir -p $(OUT)
	go build -p 4 -trimpath -ldflags='-s -w' -o $(OUT)/init ./os/cmd/spacewave-init
	file $(OUT)/init
	! readelf -l $(OUT)/init | grep -E 'INTERP|DYNAMIC'
	stat -c 'static executable bytes=%s' $(OUT)/init

check:
	/home/core/gostyle-bin $(PKGS)
	go vet -p 4 $(PKGS)

test:
	go test -p 4 -timeout=45s -run 'TestRootPinsRevision|TestMountHelperRejectsUnsafeArguments' ./os/boot
