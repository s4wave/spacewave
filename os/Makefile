# Run these targets on the Linux build host, through the remote-build SOP.
OUT ?= /home/core/build-artifacts/spacewave-os
GOSTYLE ?= /home/core/gostyle-bin
PKGS = ./os/boot ./os/cmd/spacewave-init
export CGO_ENABLED = 0
export GOMAXPROCS = 4

.PHONY: build check test model
build:
	mkdir -p $(OUT)
	go build -p 4 -trimpath -ldflags='-s -w' -o $(OUT)/init ./os/cmd/spacewave-init
	file $(OUT)/init
	! readelf -l $(OUT)/init | grep -E 'INTERP|DYNAMIC'
	stat -c 'static executable bytes=%s' $(OUT)/init

check:
	$(GOSTYLE) $(PKGS)
	go vet -p 4 $(PKGS)

test:
	go test -p 4 -timeout=45s -run '^(TestRootReadOnly|TestPersistentRootReopen|TestRevisionLifecycle|TestDataVolumeReopen)$$' ./os/boot

model:
	bun node_modules/.bin/aptre-tla os/boot
