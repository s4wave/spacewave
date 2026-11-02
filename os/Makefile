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
	go test -p 4 -timeout=45s -run '^(TestRootPinsRevision|TestPersistentRootReopen)$$' ./os/boot

model:
	bun node_modules/.bin/aptre-tla os/boot/Boot.cfg
	bun node_modules/.bin/aptre-tla os/boot/PersistentBoot.cfg
	mkdir -p $(OUT)/.tmp/direct-model
	cp os/boot/Boot.tla $(OUT)/.tmp/direct-model/Boot.tla
	cp os/boot/Boot-direct.cfg.refuted $(OUT)/.tmp/direct-model/Boot.cfg
	@if bun node_modules/.bin/aptre-tla $(OUT)/.tmp/direct-model/Boot.cfg >$(OUT)/.tmp/direct-model/trace.log 2>&1; then cat $(OUT)/.tmp/direct-model/trace.log; exit 1; fi
	grep -F 'EventuallyStopped' $(OUT)/.tmp/direct-model/trace.log
	grep -F 'StartExec: child = TRUE, execWait = TRUE, phase = "entering", serverAvailable = FALSE' $(OUT)/.tmp/direct-model/trace.log
	rm $(OUT)/.tmp/direct-model/Boot.tla $(OUT)/.tmp/direct-model/Boot.cfg
	mkdir -p $(OUT)/.tmp/persistent-model
	cp os/boot/PersistentBoot.tla $(OUT)/.tmp/persistent-model/PersistentBoot.tla
	@for mutant in head-first unretained early-close writable; do \
		cp os/boot/PersistentBoot-$$mutant.cfg.refuted $(OUT)/.tmp/persistent-model/PersistentBoot.cfg; \
		if bun node_modules/.bin/aptre-tla $(OUT)/.tmp/persistent-model/PersistentBoot.cfg >$(OUT)/.tmp/persistent-model/$$mutant.log 2>&1; then \
			cat $(OUT)/.tmp/persistent-model/$$mutant.log; exit 1; \
		fi; \
	done
	grep -F 'SavedRootHasBlocks' $(OUT)/.tmp/persistent-model/head-first.log
	grep -F 'SelectedRootRetained' $(OUT)/.tmp/persistent-model/unretained.log
	grep -F 'ClosedMeansUnused' $(OUT)/.tmp/persistent-model/early-close.log
	grep -F 'ServingPreservesStore' $(OUT)/.tmp/persistent-model/writable.log
	rm $(OUT)/.tmp/persistent-model/PersistentBoot.tla $(OUT)/.tmp/persistent-model/PersistentBoot.cfg
